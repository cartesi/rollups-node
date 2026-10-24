// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/common"
)

func (s *Service) classifyClaimSubmittedEvents(
	ctx context.Context,
	app *model.Application,
	epoch *model.Epoch,
	events []*iconsensus.IConsensusClaimSubmitted,
	site string,
) (
	*iconsensus.IConsensusClaimSubmitted,
	bool,
	error,
) {
	for _, event := range events {
		if !claimSubmittedEventMatchesEpoch(app, epoch, event) {
			continue
		}
		s.Logger.Debug("Found ClaimSubmitted Event",
			"app", event.AppContract,
			"outputs_merkle_root", fmt.Sprintf("%x", event.OutputsMerkleRoot),
			"last_block", event.LastProcessedBlockNumber.Uint64(),
		)
		matches, ok := claimSubmittedEventMatches(app, epoch, event)
		if !ok {
			return nil, true, s.markMatcherPrecondFailure(ctx, app, epoch, site)
		}
		if matches {
			if !s.shouldRecordMatchingClaimSubmitted(app, epoch, event) {
				continue
			}
			return event, false, nil
		}

		if s.shouldIgnoreQuorumSubmittedMismatch(app, epoch, event, site) {
			continue
		}
		return nil, true, s.markSubmittedDivergence(ctx, app, epoch, event, site)
	}
	return nil, false, nil
}

func (s *Service) shouldIgnoreQuorumSubmittedMismatch(
	app *model.Application,
	epoch *model.Epoch,
	event *iconsensus.IConsensusClaimSubmitted,
	site string,
) bool {
	if app.ConsensusType != model.Consensus_Quorum {
		return false
	}

	submitter, ok := s.blockchain.claimSubmitterAddress()
	if ok && event.Submitter != (common.Address{}) && event.Submitter == submitter {
		return false
	}

	ourOutputsMerkleRoot := common.Hash{}
	if epoch.TxBufferDataBlock != nil {
		ourOutputsMerkleRoot = *epoch.TxBufferDataBlock
	}
	outputsMatch := common.Hash(event.OutputsMerkleRoot) == ourOutputsMerkleRoot

	level := s.Logger.Info
	if outputsMatch {
		level = s.Logger.Warn
	}
	level("Quorum: observed non-matching ClaimSubmitted from a non-local or unknown submitter; continuing",
		"site", site,
		"app", app.IApplicationAddress,
		"event_submitter", event.Submitter,
		"our_submitter", submitter,
		"has_our_submitter", ok,
		"event_outputs", fmt.Sprintf("%x", event.OutputsMerkleRoot),
		"our_outputs", ourOutputsMerkleRoot.Hex(),
		"event_machine", fmt.Sprintf("%x", event.MachineMerkleRoot),
		"our_machine", hashToHex(epoch.MachineHash),
		"last_block", epoch.LastBlock,
	)
	return true
}

func (s *Service) shouldRecordMatchingClaimSubmitted(
	app *model.Application,
	epoch *model.Epoch,
	event *iconsensus.IConsensusClaimSubmitted,
) bool {
	if app.ConsensusType != model.Consensus_Quorum || !s.submissionEnabled {
		return true
	}
	submitter, ok := s.blockchain.claimSubmitterAddress()
	if !ok || event.Submitter == (common.Address{}) || event.Submitter == submitter {
		return true
	}
	s.Logger.Info("Quorum: observed matching ClaimSubmitted from another validator; submitting local vote",
		"app", app.IApplicationAddress,
		"event_submitter", event.Submitter,
		"our_submitter", submitter,
		"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
		"last_block", epoch.LastBlock,
	)
	return false
}

// submitClaimsAndUpdateDatabase moves epochs from CLAIM_COMPUTED toward
// CLAIM_SUBMITTED. It returns the number of successful state changes and any
// errors.
func (s *Service) submitClaimsAndUpdateDatabase(
	ctx context.Context,
	computedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, error) {
	return s.observeClaims(ctx, computedEpochs, apps, defaultBlockNumber, s.processComputedClaim)
}

func (s *Service) processComputedClaim(
	ctx context.Context,
	work claimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch
	if err := requireObservedStaging(currEpoch, defaultBlockNumber.Uint64()); err != nil {
		return claimRetryLater(err)
	}

	// Stop if the consensus contract address changed on chain.
	if err := s.checkConsensusForAddressChange(ctx, app, defaultBlockNumber); err != nil {
		return claimDropped(err)
	}

	if result, done := s.reconcileComputedAcceptedEvent(ctx, work, defaultBlockNumber); done {
		return result
	}

	_, submittedEvents, result, done := s.findSubmittedEventsForComputedClaim(ctx, work, defaultBlockNumber)
	if done {
		return result
	}

	currEvent, shouldDrop, err := s.classifyClaimSubmittedEvents(
		ctx, app, currEpoch, submittedEvents, "submitClaimsAndUpdateDatabase(ClaimSubmitted)")
	if shouldDrop {
		return claimDropped(err)
	}
	if err != nil {
		return claimRetryLater(err)
	}
	if currEvent != nil {
		return s.recordSubmittedEvent(ctx, app, currEpoch, currEvent)
	}

	if s.submissionEnabled || app.ForecloseBlock != 0 {
		// Before sending a transaction, read the claim state from the chain.
		// The claim identity is (app, last processed block number,
		// machineMerkleRoot). The chain may already know this claim as STAGED
		// or ACCEPTED after a restart, or when another party acted first.
		//
		// This read also runs for foreclosed apps. A claim accepted before
		// foreclosure must still be copied into the DB. Only the new submitClaim
		// transaction is skipped for foreclosed apps.
		if reconciled, err := s.reconcileBeforeSubmit(ctx, app, currEpoch, defaultBlockNumber); reconciled {
			if err != nil {
				return claimDropped(err)
			}
			return claimWorkCompleted(1)
		} else if err != nil {
			return claimDropped(err)
		}

		// Foreclosed apps cannot submit new claims. The getClaim call above
		// already showed that this claim is not STAGED or ACCEPTED, so it has
		// no remaining on-chain path.
		if app.ForecloseBlock != 0 {
			if ferr := s.forecloseClaim(ctx, app, currEpoch, defaultBlockNumber.Uint64(), "submitClaimsAndUpdateDatabase"); ferr != nil {
				return claimDropped(ferr)
			}
			return claimWorkCompleted(1)
		}
	}

	if s.submissionEnabled {
		return s.planComputedClaim(ctx, app, currEpoch, defaultBlockNumber)
	}
	return claimNoProgress()
}

func (s *Service) reconcileComputedAcceptedEvent(
	ctx context.Context,
	work claimWork,
	defaultBlockNumber *big.Int,
) (claimStepResult, bool) {
	app := work.app
	currEpoch := work.epoch

	// Look for ClaimAccepted events for this epoch.
	//
	// If the event matches our machine root, we are just catching up from
	// chain state and can mark the epoch ACCEPTED locally.
	//
	// If the event has a different machine root, another claim was accepted and
	// our claim cannot win. Mark the app DIVERGED with the consensus-specific
	// acceptance-divergence reason. Quorum can legitimately reach this path from
	// CLAIM_COMPUTED when another validator's vote wins before ours is staged.
	// Authority usually finds the mismatch earlier, during submission, but this
	// path handles both consensus types.
	acceptScanFrom := currEpoch.LastBlock + 1
	_, foreignAccepted, _, err := s.blockchain.findClaimAcceptedEventAndSucc(
		ctx, app, currEpoch, acceptScanFrom, defaultBlockNumber.Uint64(),
	)
	if err != nil {
		return claimDropped(fmt.Errorf(
			"scanning ClaimAccepted for computed epoch %d (%d): %w",
			currEpoch.Index, currEpoch.VirtualIndex, err)), true
	}
	if foreignAccepted == nil || !claimAcceptedEventMatchesEpoch(app, currEpoch, foreignAccepted) {
		return claimNoProgress(), false
	}
	matches, ok := claimAcceptedEventMatches(app, currEpoch, foreignAccepted)
	if !ok {
		return claimDropped(s.markMatcherPrecondFailure(ctx, app, currEpoch, "submitClaimsAndUpdateDatabase(ClaimAccepted)")), true
	}
	if !matches {
		return claimDropped(s.markAcceptedDivergence(ctx, app, currEpoch, foreignAccepted, "submitClaimsAndUpdateDatabase")), true
	}
	acceptedTxHash := foreignAccepted.Raw.TxHash
	if err := s.repository.UpdateEpochWithAcceptedClaim(
		ctx, currEpoch.ApplicationID, currEpoch.Index, &acceptedTxHash); err != nil {
		return claimDropped(fmt.Errorf(
			"reconciling COMPUTED→ACCEPTED for epoch %d (%d): %w",
			currEpoch.Index, currEpoch.VirtualIndex, err)), true
	}
	s.Logger.Info("ClaimAccepted observed for computed epoch (deep catch-up; reconciled)",
		"app", app.IApplicationAddress,
		"epoch_index", currEpoch.Index,
		"tx", foreignAccepted.Raw.TxHash,
		"last_block", currEpoch.LastBlock,
	)
	return claimWorkCompleted(1), true
}

func (s *Service) findSubmittedEventsForComputedClaim(
	ctx context.Context,
	work claimWork,
	defaultBlockNumber *big.Int,
) (*iconsensus.IConsensus, []*iconsensus.IConsensusClaimSubmitted, claimStepResult, bool) {
	app := work.app
	currEpoch := work.epoch
	ic, submittedEvents, err := s.blockchain.findClaimSubmittedEventAndSucc(
		ctx, app, currEpoch, currEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
	)
	if err != nil {
		return nil, nil, claimDropped(err), true
	}
	return ic, submittedEvents, claimNoProgress(), false
}

func (s *Service) recordSubmittedEvent(
	ctx context.Context,
	app *model.Application,
	currEpoch *model.Epoch,
	currEvent *iconsensus.IConsensusClaimSubmitted,
) claimStepResult {
	s.Logger.Debug("Updating claim status to submitted",
		"app", app.IApplicationAddress,
		"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
		"last_block", currEpoch.LastBlock,
	)
	txHash := currEvent.Raw.TxHash
	err := s.repository.UpdateEpochWithSubmittedClaim(
		ctx,
		currEpoch.ApplicationID,
		currEpoch.Index,
		txHash,
	)
	if err != nil {
		return claimDropped(err)
	}
	s.Logger.Info("Claim previously submitted",
		"app", app.IApplicationAddress,
		"event_block_number", currEvent.Raw.BlockNumber,
		"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
		"last_block", currEpoch.LastBlock,
	)
	return claimProgressed(1)
}

func (s *Service) planComputedClaim(
	ctx context.Context,
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) claimStepResult {
	// This diagnoses persisted proof integrity; publication has its own coherent
	// epoch eligibility check. A later execution stop must not hide a DC finding.
	status := app.Status
	if !app.Enabled || app.ForecloseBlock != 0 ||
		(status != model.ApplicationStatus_OK && !status.IsExecutionTerminal() && status != model.ApplicationStatus_InvalidOutputsRoot) {
		return claimNoProgress()
	}
	_, err := currEpoch.StateProof()
	if err != nil {
		stateErr := s.setApplicationCorrupted(
			ctx,
			app,
			"cannot submit claim for epoch %d (%d), last_block %d: persisted machine state proof is incomplete: %v",
			currEpoch.Index,
			currEpoch.VirtualIndex,
			currEpoch.LastBlock,
			err,
		)
		return claimDropped(stateErr)
	}
	return claimNoProgress()
}

// reconcileBeforeSubmit calls getClaim before submitClaim.
//
// If the chain already has this claim as STAGED or ACCEPTED, update the DB and
// do not send another transaction. Returns (reconciled, err):
//   - (true, nil): the DB was updated to STAGED or ACCEPTED.
//   - (false, nil): the chain says UNSTAGED; caller may submit.
//   - (_, err): an error occurred; caller should drop this work item.
//
// All chain reads in one tick use the same configured observation block.
func (s *Service) reconcileBeforeSubmit(
	ctx context.Context,
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) (bool, error) {
	claim, err := s.blockchain.getClaimStatus(ctx, app, currEpoch, defaultBlockNumber)
	if err != nil {
		return false, fmt.Errorf("pre-submit getClaim (app=%v, epoch=%d): %w",
			app.IApplicationAddress, currEpoch.Index, err)
	}
	switch claim.Status {
	case claimStatusAccepted:
		if err := s.updateEpochAcceptedFromClaimStatus(ctx, app, currEpoch, claim, "reconcileBeforeSubmit"); err != nil {
			return false, fmt.Errorf("reconciling epoch %d (%d) to ACCEPTED: %w",
				currEpoch.Index, currEpoch.VirtualIndex, err)
		}
		s.Logger.Info("Claim already accepted on chain (reconciled pre-submit)",
			"app", app.IApplicationAddress,
			"epoch_index", currEpoch.Index,
			"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
			"last_block", currEpoch.LastBlock,
		)
		return true, nil
	case claimStatusStaged:
		stagingBlock, err := s.updateEpochStagedFromClaimStatus(ctx, app, currEpoch, claim, "reconcileBeforeSubmit")
		if err != nil {
			return false, fmt.Errorf("reconciling epoch %d (%d) to STAGED: %w",
				currEpoch.Index, currEpoch.VirtualIndex, err)
		}
		s.Logger.Info("Claim already staged on chain (reconciled pre-submit)",
			"app", app.IApplicationAddress,
			"epoch_index", currEpoch.Index,
			"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
			"last_block", currEpoch.LastBlock,
			"staged_at_block", stagingBlock,
		)
		return true, nil
	case claimStatusUnstaged:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected ClaimStatus %d from getClaim for app=%v epoch=%d",
			claim.Status, app.IApplicationAddress, currEpoch.Index)
	}
}
