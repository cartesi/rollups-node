// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

func (s *Service) findClaimAcceptedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	prevEpoch *model.Epoch,
	currEpoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	*iconsensus.IConsensusClaimAccepted,
	*iconsensus.IConsensusClaimAccepted,
	error,
) {
	err := checkEpochSequenceConstraint(prevEpoch, currEpoch)
	if err != nil {
		err = s.setApplicationCorrupted(
			ctx,
			app,
			"%v. epoch: %v (%v).",
			err,
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, nil, err
	}

	ic, prevClaimAcceptanceEvent, currClaimAcceptanceEvent, err :=
		s.blockchain.findClaimAcceptedEventAndSucc(ctx, app, prevEpoch, fromBlock, toBlock)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("finding claim accepted event for epoch %d (%d): %w", prevEpoch.Index, prevEpoch.VirtualIndex, err)
	}

	if prevClaimAcceptanceEvent == nil {
		err = s.setApplicationCorrupted(
			ctx,
			app,
			"application has an invalid epoch: %v (%v), missing claim acceptance event.",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, nil, err
	}
	matches, ok := claimAcceptedEventMatches(app, prevEpoch, prevClaimAcceptanceEvent)
	if !ok {
		err = s.markMatcherPrecondFailure(app, prevEpoch, "findClaimAcceptedEventAndSucc(prev)")
		return nil, nil, nil, err
	}
	if !matches {
		err = s.setApplicationDiverged(
			ctx,
			app,
			"application has an invalid epoch: %v (%v). event does not match: %v",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
			prevClaimAcceptanceEvent.Raw.TxHash,
		)
		return nil, nil, nil, err
	}
	return ic, prevClaimAcceptanceEvent, currClaimAcceptanceEvent, nil
}

// acceptClaimsAndUpdateDatabase looks for ClaimAccepted events on chain and
// moves local epochs to CLAIM_ACCEPTED.
//
// Tick normally passes CLAIM_STAGED epochs here. Other reconciliation paths
// may move CLAIM_SUBMITTED or CLAIM_COMPUTED directly to CLAIM_ACCEPTED, and
// UpdateEpochWithAcceptedClaim accepts those source states.
//
// It returns the number of successful state changes and any errors.
func (s *Service) acceptClaimsAndUpdateDatabase(
	acceptedEpochs map[int64]*model.Epoch,
	stagedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, []error) {
	transitions := 0
	errs := []error{}

	for key, currEpoch := range stagedEpochs {
		result := s.processAcceptedClaimEvent(stagedClaimWork{
			app:       apps[key],
			prevEpoch: acceptedEpochs[key],
			epoch:     currEpoch,
		}, defaultBlockNumber)
		transitions += result.progress
		if result.err != nil {
			errs = append(errs, result.err)
		}
		if result.drop {
			delete(stagedEpochs, key)
		}
	}
	return transitions, errs
}

func (s *Service) processAcceptedClaimEvent(
	work stagedClaimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch
	prevEpoch := work.prevEpoch

	if err := s.checkConsensusForAddressChange(app, defaultBlockNumber); err != nil {
		return claimDropped(err)
	}

	var currEvent *iconsensus.IConsensusClaimAccepted
	var err error
	if prevEpoch != nil {
		_, _, currEvent, err = s.findClaimAcceptedEventAndSucc(
			s.Context, app, prevEpoch, currEpoch, prevEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
		)
	} else {
		_, currEvent, _, err = s.blockchain.findClaimAcceptedEventAndSucc(
			s.Context, app, currEpoch, currEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
		)
	}
	if err != nil {
		return claimDropped(err)
	}

	if currEvent == nil {
		return claimNoProgress()
	}

	s.Logger.Debug("Found ClaimAccepted Event",
		"app", currEvent.AppContract,
		"outputs_merkle_root", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
		"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
	)
	matches, ok := claimAcceptedEventMatches(app, currEpoch, currEvent)
	if !ok {
		return claimDropped(s.markMatcherPrecondFailure(app, currEpoch, "acceptClaimsAndUpdateDatabase"))
	}
	if !matches {
		return claimDropped(s.markAcceptedDivergence(app, currEpoch, currEvent, "acceptClaimsAndUpdateDatabase"))
	}
	s.Logger.Debug("Updating claim status to accepted",
		"app", app.IApplicationAddress,
		"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
		"last_block", currEpoch.LastBlock,
	)
	txHash := currEvent.Raw.TxHash
	err = s.repository.UpdateEpochWithAcceptedClaim(s.Context, currEpoch.ApplicationID, currEpoch.Index, &txHash)
	if err != nil {
		return claimDropped(err)
	}
	s.dropAcceptInFlight(app.ID)
	s.dropAcceptAttempt(acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index})
	s.Logger.Info("Claim accepted",
		"app", currEvent.AppContract,
		"event_block_number", currEvent.Raw.BlockNumber,
		"outputs_merkle_root", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
		"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
		"tx", txHash,
	)
	return claimProgressed(1)
}

// acceptStagedClaimsAndIssueAcceptTx checks CLAIM_STAGED epochs.
//
// If the staging period has passed and submit mode is enabled, it sends an
// acceptClaim transaction. Before sending, it calls getClaim(). Another
// validator may have accepted the same claim first. If that already happened,
// we only update the local DB to CLAIM_ACCEPTED and do not send our own
// transaction.
//
// In reader mode (submissionEnabled=false), this function does not send
// transactions; the later ClaimAccepted event scan performs the DB update.
func (s *Service) acceptStagedClaimsAndIssueAcceptTx(
	stagedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, []error) {
	transitions := 0
	errs := []error{}

	for key, currEpoch := range stagedEpochs {
		result := s.processStagedClaim(stagedClaimWork{
			app:   apps[key],
			epoch: currEpoch,
		}, defaultBlockNumber)
		transitions += result.progress
		if result.err != nil {
			errs = append(errs, result.err)
		}
		if result.drop {
			delete(stagedEpochs, key)
		}
	}
	return transitions, errs
}

func (s *Service) processStagedClaim(
	work stagedClaimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch

	currentBlock, result, done := s.stagedClaimReadyForAccept(app, currEpoch, defaultBlockNumber)
	if done {
		return result
	}

	// Read the claim state before sending acceptClaim. Use the same block
	// number for all reads in this tick.
	claim, err := s.blockchain.getClaimStatus(s.Context, app, currEpoch, defaultBlockNumber)
	if err != nil {
		return claimRetryLater(fmt.Errorf("getClaim before acceptClaim (app=%v, epoch=%d): %w",
			app.IApplicationAddress, currEpoch.Index, err))
	}
	// A foreclosed application has no remaining on-chain path for a claim that
	// is not already ACCEPTED, so terminalize it to CLAIM_FORECLOSED instead of
	// running the generic accept/divergence handling (which would otherwise mark
	// the app FAILED on an UNSTAGED reading and leave the drain behind a
	// re-enable loop).
	if app.ForecloseBlock != 0 && claim.Status != claimStatusAccepted {
		return s.terminalizeForeclosedStagedClaim(app, currEpoch)
	}
	if result, done := s.handlePreAcceptClaimStatus(app, currEpoch, claim, currentBlock); done {
		return result
	}
	return s.broadcastAcceptClaimOrReconcileRevert(app, currEpoch, defaultBlockNumber)
}

func (s *Service) stagedClaimReadyForAccept(
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) (uint64, claimStepResult, bool) {
	// We already sent an acceptClaim transaction and are waiting for it.
	if s.hasAcceptInFlight(app.ID) {
		return 0, claimNoProgress(), true
	}

	if err := s.checkConsensusForAddressChange(app, defaultBlockNumber); err != nil {
		return 0, claimRetryLater(err), true
	}

	if currEpoch.StagedAtBlock == nil {
		// Invariant: CLAIM_STAGED rows must have staged_at_block. The database
		// CHECK should stop this from happening.
		err := s.setApplicationCorrupted(s.Context, app,
			"epoch %d (%d) is CLAIM_STAGED but staged_at_block is nil",
			currEpoch.Index, currEpoch.VirtualIndex)
		return 0, claimRetryLater(err), true
	}

	currentBlock := defaultBlockNumber.Uint64()
	if app.ForecloseBlock != 0 {
		return currentBlock, claimNoProgress(), false
	}
	// The staging period has not passed yet. Try again in a later tick.
	if currentBlock < *currEpoch.StagedAtBlock {
		return currentBlock, claimNoProgress(), true
	}
	if currentBlock-*currEpoch.StagedAtBlock < app.ClaimStagingPeriod {
		return currentBlock, claimNoProgress(), true
	}
	if !s.submissionEnabled {
		// Reader mode does not send transactions. Wait until another party
		// sends acceptClaim and we observe ClaimAccepted.
		return currentBlock, claimNoProgress(), true
	}
	return currentBlock, claimNoProgress(), false
}

func (s *Service) handlePreAcceptClaimStatus(
	app *model.Application,
	currEpoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	currentBlock uint64,
) (claimStepResult, bool) {
	switch claim.Status {
	case claimStatusAccepted: // Another party accepted first; update our DB.
		err := s.updateEpochAcceptedFromClaimStatus(app, currEpoch, claim, "acceptStagedClaimsAndIssueAcceptTx")
		if err != nil {
			return claimRetryLater(err), true
		}
		s.dropAcceptAttempt(acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index})
		s.Logger.Info("Claim accepted (front-run; observed via getClaim)",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
			"last_block", currEpoch.LastBlock,
		)
		return claimProgressed(1), true
	case claimStatusUnstaged:
		// Invariant: at the finalized block, a local CLAIM_STAGED row must
		// match a STAGED or ACCEPTED claim on chain. If the chain says UNSTAGED,
		// the node is probably reading the wrong chain, an old block, or stale
		// node_config. Mark the app FAILED so the operator sees the problem.
		if ferr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"getClaim returned UNSTAGED for epoch %d (%d) recorded as CLAIM_STAGED at block %d; "+
				"current block %d. Likely a misconfigured default block or stale node_config — "+
				"verify CARTESI_BLOCKCHAIN_DEFAULT_BLOCK is 'finalized' or 'safe' and that the "+
				"node_config row matches before re-enabling.",
			currEpoch.Index, currEpoch.VirtualIndex,
			*currEpoch.StagedAtBlock, currentBlock); ferr != nil {
			return claimRetryLater(fmt.Errorf("marking app FAILED on UNSTAGED pre-accept getClaim: %w", ferr)), true
		}
		return claimNoProgress(), true
	case claimStatusStaged:
		// Defense-in-depth invariant check. The chain says our claim is still
		// STAGED, so its outputs root must match our local epoch. If it does
		// not match, this node and the chain disagree about the claim data.
		if vErr := s.verifyClaimOutputsMatch(app, currEpoch, claim, "acceptStagedClaimsAndIssueAcceptTx"); vErr != nil {
			return claimRetryLater(vErr), true
		}
		return claimNoProgress(), false
	default:
		// getClaim returned a ClaimStatus this node does not model (the enum is
		// 0/1/2). This is not a transient hiccup: the value came from a typed
		// view call, so the IConsensus contract is most likely a newer version
		// than this node. Mark the app FAILED (recoverable) so an operator sees
		// it, rather than skipping silently every tick forever.
		if ferr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"getClaim returned unmodeled ClaimStatus %d for epoch %d (%d) — this node models "+
				"only 0/1/2. The IConsensus contract may be newer than this node supports; "+
				"upgrade the node or verify the contract before re-enabling.",
			claim.Status, currEpoch.Index, currEpoch.VirtualIndex); ferr != nil {
			return claimRetryLater(fmt.Errorf(
				"marking app FAILED on unmodeled ClaimStatus %d: %w", claim.Status, ferr)), true
		}
		return claimNoProgress(), true
	}
}

func (s *Service) terminalizeForeclosedStagedClaim(
	app *model.Application,
	currEpoch *model.Epoch,
) claimStepResult {
	if ferr := s.forecloseClaim(app, currEpoch, "acceptStagedClaimsAndIssueAcceptTx"); ferr != nil {
		return claimRetryLater(ferr)
	}
	s.dropAcceptAttempt(acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index})
	return claimWorkCompleted(1)
}

func (s *Service) broadcastAcceptClaimOrReconcileRevert(
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) claimStepResult {
	// Stop after too many failed acceptClaim attempts. This prevents the node
	// from spending gas forever on one epoch. Each (app, epoch) has its own
	// attempt counter.
	attemptKey := acceptAttemptKey{currEpoch.ApplicationID, currEpoch.Index}
	attempts := s.incrementAcceptAttempt(attemptKey)
	if attempts > s.maxAcceptAttempts {
		var err error
		if ferr := appstatus.SetFailedf(s.Context, s.Logger, s.repository, app,
			"acceptClaim has failed %d consecutive times for epoch %d (%d); "+
				"inspect logs and the chain state, then re-enable. "+
				"Common causes: gas estimation issues, signer not authorised, "+
				"nonce gaps, or a fork inconsistent with the configured RPC.",
			attempts, currEpoch.Index, currEpoch.VirtualIndex); ferr != nil {
			err = fmt.Errorf("marking app FAILED after %d accept attempts: %w",
				attempts, ferr)
		}
		s.dropAcceptAttempt(attemptKey)
		return claimRetryLater(err)
	}

	txCtx, cancel := context.WithTimeout(s.Context, s.submissionTimeout)
	defer cancel()
	txHash, err := s.blockchain.acceptClaimOnBlockchain(txCtx, app, currEpoch)
	if err != nil {
		outcome, stateErr := s.handleAcceptClaimRevert(err, app, currEpoch)
		switch outcome {
		case acceptClaimRetryLater:
			return claimNoProgress()
		case acceptClaimAppHalted:
			s.dropAcceptAttempt(attemptKey)
			return claimRetryLater(stateErr)
		case acceptClaimReconciledAccepted:
			claim, gerr := s.blockchain.getClaimStatus(s.Context, app, currEpoch, defaultBlockNumber)
			if gerr != nil {
				return claimRetryLater(fmt.Errorf("getClaim after acceptClaim front-run revert (app=%v, epoch=%d): %w",
					app.IApplicationAddress, currEpoch.Index, gerr))
			}
			if claim.Status != claimStatusAccepted {
				s.Logger.Warn("acceptClaim reverted as accepted, but pinned getClaim has not caught up",
					"app", app.IApplicationAddress,
					"epoch_index", currEpoch.Index,
					"claim_status", claim.Status,
				)
				s.dropAcceptAttempt(attemptKey)
				return claimNoProgress()
			}
			err = s.updateEpochAcceptedFromClaimStatus(app, currEpoch, claim, "acceptClaimReconciledAccepted")
			if err != nil {
				return claimRetryLater(err)
			}
			s.dropAcceptAttempt(attemptKey)
			return claimProgressed(1)
		case acceptClaimUnknown:
			return claimRetryLater(err)
		default:
			// A new acceptClaimRevertOutcome was added, but this switch was not
			// updated. Return the error so the bug is visible in logs. The
			// normal attempt counter still limits retries.
			s.Logger.Error("unhandled acceptClaimRevertOutcome; surfacing as error",
				"outcome", outcome,
				"app", app.IApplicationAddress,
				"epoch_index", currEpoch.Index,
				"error", err)
			return claimRetryLater(fmt.Errorf("unhandled acceptClaimRevertOutcome %d: %w", outcome, err))
		}
	}
	s.putAcceptInFlight(app.ID, inFlightTx{
		txHash:         txHash,
		firstSeenBlock: defaultBlockNumber.Uint64(),
	})
	return claimNoProgress()
}
