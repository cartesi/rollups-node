// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
)

// acceptClaimsAndUpdateDatabase looks for ClaimAccepted events on chain and
// moves local epochs to CLAIM_ACCEPTED.
//
// Tick normally passes CLAIM_STAGED epochs here. Other reconciliation paths
// may move CLAIM_SUBMITTED or CLAIM_COMPUTED directly to CLAIM_ACCEPTED, and
// UpdateEpochWithAcceptedClaim accepts those source states.
//
// It returns the number of successful state changes and any errors.
func (s *Service) acceptClaimsAndUpdateDatabase(
	ctx context.Context,
	stagedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, error) {
	return s.observeClaims(ctx, stagedEpochs, apps, defaultBlockNumber, s.processAcceptedClaimEvent)
}

func (s *Service) processAcceptedClaimEvent(
	ctx context.Context,
	work claimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch

	if err := requireObservedStaging(currEpoch, defaultBlockNumber.Uint64()); err != nil {
		return claimRetryLater(err)
	}

	if err := s.checkConsensusForAddressChange(ctx, app, defaultBlockNumber); err != nil {
		return claimDropped(err)
	}

	_, currEvent, _, err := s.blockchain.findClaimAcceptedEventAndSucc(
		ctx, app, currEpoch, currEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
	)
	if err != nil {
		return claimDropped(err)
	}

	if currEvent == nil || !claimAcceptedEventMatchesEpoch(app, currEpoch, currEvent) {
		return claimNoProgress()
	}

	s.Logger.Debug("Found ClaimAccepted Event",
		"app", currEvent.AppContract,
		"outputs_merkle_root", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
		"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
	)
	matches, ok := claimAcceptedEventMatches(app, currEpoch, currEvent)
	if !ok {
		return claimDropped(s.markMatcherPrecondFailure(ctx, app, currEpoch, "acceptClaimsAndUpdateDatabase"))
	}
	if !matches {
		return claimDropped(s.markAcceptedDivergence(ctx, app, currEpoch, currEvent, "acceptClaimsAndUpdateDatabase"))
	}
	if s.defaultBlock.Provisional() {
		claim, err := s.blockchain.getClaimStatus(ctx, app, currEpoch, defaultBlockNumber)
		if err != nil {
			return claimRetryLater(err)
		}
		if claim.Status != claimStatusAccepted || claim.StagedOutputsMerkleRoot != currEvent.OutputsMerkleRoot {
			return claimRetryLater(txsender.ErrInconclusive)
		}
		if err := s.refreshStagedClaim(ctx, app, currEpoch, claim, defaultBlockNumber.Uint64()); err != nil {
			return claimRetryLater(err)
		}
	}
	s.Logger.Debug("Updating claim status to accepted",
		"app", app.IApplicationAddress,
		"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
		"last_block", currEpoch.LastBlock,
	)
	txHash := currEvent.Raw.TxHash
	err = s.repository.UpdateEpochWithAcceptedClaim(ctx, currEpoch.ApplicationID, currEpoch.Index, &txHash)
	if err != nil {
		return claimDropped(err)
	}
	s.Logger.Info("Claim accepted",
		"app", currEvent.AppContract,
		"event_block_number", currEvent.Raw.BlockNumber,
		"outputs_merkle_root", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
		"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
		"tx", txHash,
	)
	return claimWorkCompleted(1)
}

// observeStagedClaims checks committed staging facts against the configured view.
// The publisher collects ready accept
// candidates. Reader mode observes without constructing publication work.
func (s *Service) observeStagedClaims(
	ctx context.Context,
	stagedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, error) {
	return s.observeClaims(ctx, stagedEpochs, apps, defaultBlockNumber, s.processStagedClaim)
}

func (s *Service) processStagedClaim(
	ctx context.Context,
	work claimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch

	currentBlock, result, done := s.stagedClaimReadyForAccept(ctx, app, currEpoch, defaultBlockNumber)
	if done {
		return result
	}

	// Read the claim state before sending acceptClaim. Use the same block
	// number for all reads in this tick.
	claim, err := s.blockchain.getClaimStatus(ctx, app, currEpoch, defaultBlockNumber)
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
		return s.terminalizeForeclosedStagedClaim(ctx, app, currEpoch, currentBlock)
	}
	if result, done := s.handlePreAcceptClaimStatus(ctx, app, currEpoch, claim, currentBlock); done {
		return result
	}
	return claimNoProgress()
}

func (s *Service) stagedClaimReadyForAccept(
	ctx context.Context,
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) (uint64, claimStepResult, bool) {
	if err := s.checkConsensusForAddressChange(ctx, app, defaultBlockNumber); err != nil {
		return 0, claimRetryLater(err), true
	}

	if currEpoch.StagedAtBlock == nil {
		// Invariant: CLAIM_STAGED rows must have staged_at_block. The database
		// CHECK should stop this from happening.
		err := s.setApplicationCorrupted(ctx, app,
			"epoch %d (%d) is CLAIM_STAGED but staged_at_block is nil",
			currEpoch.Index, currEpoch.VirtualIndex)
		return 0, claimRetryLater(err), true
	}

	currentBlock := defaultBlockNumber.Uint64()
	if err := requireObservedStaging(currEpoch, currentBlock); err != nil {
		return currentBlock, claimRetryLater(err), true
	}
	if app.ForecloseBlock != 0 {
		return currentBlock, claimNoProgress(), false
	}
	// The staging period has not passed yet. Try again in a later tick.
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
	ctx context.Context,
	app *model.Application,
	currEpoch *model.Epoch,
	claim iconsensus.IConsensusClaim,
	currentBlock uint64,
) (claimStepResult, bool) {
	switch claim.Status {
	case claimStatusAccepted: // Another party accepted first; update our DB.
		if s.defaultBlock.Provisional() {
			if err := s.verifyClaimOutputsMatch(ctx, app, currEpoch, claim, "observeStagedClaims"); err != nil {
				return claimRetryLater(err), true
			}
			if err := s.refreshStagedClaim(ctx, app, currEpoch, claim, currentBlock); err != nil {
				return claimRetryLater(err), true
			}
		}
		err := s.updateEpochAcceptedFromClaimStatus(ctx, app, currEpoch, claim, "observeStagedClaims")
		if err != nil {
			return claimRetryLater(err), true
		}
		s.Logger.Info("Claim accepted (front-run; observed via getClaim)",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
			"last_block", currEpoch.LastBlock,
		)
		return claimWorkCompleted(1), true
	case claimStatusUnstaged:
		if s.defaultBlock.Provisional() {
			return claimRetryLater(fmt.Errorf("%w: epoch %d staging is absent at provisional block %d",
				txsender.ErrInconclusive, currEpoch.Index, currentBlock)), true
		}
		// A finalized view at or above the stored staging block contradicts
		// this fact. Keep the recoverable diagnosis for operator repair.
		if ferr := appstatus.SetFailedf(ctx, s.Logger, s.repository, app,
			"getClaim returned UNSTAGED for epoch %d (%d) recorded as CLAIM_STAGED at block %d; "+
				"current block %d. Check chain configuration and provisional-state reorgs; "+
				"rollback is unsupported. Repair stored state before re-enabling.",
			currEpoch.Index, currEpoch.VirtualIndex,
			*currEpoch.StagedAtBlock, currentBlock); ferr != nil {
			return claimRetryLater(fmt.Errorf("marking app FAILED on UNSTAGED pre-accept getClaim: %w", ferr)), true
		}
		return claimNoProgress(), true
	case claimStatusStaged:
		// Defense-in-depth invariant check. The chain says our claim is still
		// STAGED, so its outputs root must match our local epoch. If it does
		// not match, this node and the chain disagree about the claim data.
		if vErr := s.verifyClaimOutputsMatch(ctx, app, currEpoch, claim, "observeStagedClaims"); vErr != nil {
			return claimRetryLater(vErr), true
		}
		if s.defaultBlock.Provisional() {
			if err := s.refreshStagedClaim(ctx, app, currEpoch, claim, currentBlock); err != nil {
				return claimRetryLater(err), true
			}
		}
		return claimNoProgress(), false
	default:
		// getClaim returned a ClaimStatus this node does not model (the enum is
		// 0/1/2). This is not a transient hiccup: the value came from a typed
		// view call, so the IConsensus contract is most likely a newer version
		// than this node. Mark the app FAILED (recoverable) so an operator sees
		// it, rather than skipping silently every tick forever.
		if ferr := appstatus.SetFailedf(ctx, s.Logger, s.repository, app,
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
	ctx context.Context,
	app *model.Application,
	currEpoch *model.Epoch,
	observedBlock uint64,
) claimStepResult {
	if ferr := s.forecloseClaim(ctx, app, currEpoch, observedBlock, "observeStagedClaims"); ferr != nil {
		return claimRetryLater(ferr)
	}
	return claimWorkCompleted(1)
}
