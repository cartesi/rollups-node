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

func (s *Service) findClaimSubmittedEventAndSucc(
	ctx context.Context,
	app *model.Application,
	prevEpoch *model.Epoch,
	currEpoch *model.Epoch,
	fromBlock uint64,
	toBlock uint64,
) (
	*iconsensus.IConsensus,
	[]*iconsensus.IConsensusClaimSubmitted,
	error,
) {
	err := checkEpochSequenceConstraint(prevEpoch, currEpoch)
	if err != nil {
		err = s.setApplicationCorrupted(
			s.Context,
			app,
			"%v. epoch: %v (%v).",
			err,
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, err
	}

	ic, events, err :=
		s.blockchain.findClaimSubmittedEventAndSucc(ctx, app, prevEpoch, fromBlock, toBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("finding claim submitted event for epoch %d (%d): %w", prevEpoch.Index, prevEpoch.VirtualIndex, err)
	}

	var prevClaimSubmissionEvent *iconsensus.IConsensusClaimSubmitted
	for _, event := range events {
		if claimSubmittedEventMatchesEpoch(app, prevEpoch, event) {
			matches, ok := claimSubmittedEventMatches(app, prevEpoch, event)
			if !ok {
				err = s.markMatcherPrecondFailure(app, prevEpoch, "findClaimSubmittedEventAndSucc(prev)")
				return nil, nil, err
			}
			if matches {
				prevClaimSubmissionEvent = event
				break
			}
			if s.shouldIgnoreQuorumSubmittedMismatch(app, prevEpoch, event, "findClaimSubmittedEventAndSucc(prev)") {
				continue
			}
			err = s.setApplicationDiverged(
				s.Context,
				app,
				"application has an invalid epoch: %v (%v), missing claim submitted event (%v).",
				prevEpoch.Index,
				prevEpoch.VirtualIndex,
				event.Raw.TxHash,
			)
			return nil, nil, err
		}
	}
	if prevClaimSubmissionEvent == nil {
		err = s.setApplicationCorrupted(
			s.Context,
			app,
			"application has an invalid epoch: %v (%v). No claim submission event to match.",
			prevEpoch.Index,
			prevEpoch.VirtualIndex,
		)
		return nil, nil, err
	}

	return ic, events, nil
}

func (s *Service) classifyClaimSubmittedEvents(
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
			return nil, true, s.markMatcherPrecondFailure(app, epoch, site)
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
		return nil, true, s.markSubmittedDivergence(app, epoch, event, site)
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
	if epoch.OutputsMerkleRoot != nil {
		ourOutputsMerkleRoot = *epoch.OutputsMerkleRoot
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
		"outputs_merkle_root", hashToHex(epoch.OutputsMerkleRoot),
		"last_block", epoch.LastBlock,
	)
	return false
}

// submitClaimsAndUpdateDatabase moves epochs from CLAIM_COMPUTED toward
// CLAIM_SUBMITTED. It returns the number of successful state changes and any
// errors.
func (s *Service) submitClaimsAndUpdateDatabase(
	acceptedOrSubmittedEpochs map[int64]*model.Epoch,
	computedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, []error) {
	confirmed, err := s.checkClaimsInFlight(computedEpochs, apps, defaultBlockNumber)
	if err != nil {
		return confirmed, []error{err}
	}

	transitions := confirmed
	errs := []error{}
	for key, currEpoch := range computedEpochs {
		result := s.processComputedClaim(computedClaimWork{
			app:       apps[key],
			prevEpoch: acceptedOrSubmittedEpochs[key],
			epoch:     currEpoch,
		}, defaultBlockNumber)
		transitions += result.progress
		if result.err != nil {
			errs = append(errs, result.err)
		}
		if result.drop {
			delete(computedEpochs, key)
		}
	}
	return transitions, errs
}

func (s *Service) processComputedClaim(
	work computedClaimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch
	prevEpoch := work.prevEpoch
	appID := app.ID

	if s.hasClaimInFlight(appID) {
		return claimNoProgress()
	}

	// Stop if the consensus contract address changed on chain.
	if err := s.checkConsensusForAddressChange(app, defaultBlockNumber); err != nil {
		return claimDropped(err)
	}

	if result, done := s.reconcileComputedAcceptedEvent(work, defaultBlockNumber); done {
		return result
	}

	ic, submittedEvents, result, done := s.findSubmittedEventsForComputedClaim(work, defaultBlockNumber)
	if done {
		return result
	}

	currEvent, shouldDrop, err := s.classifyClaimSubmittedEvents(
		app, currEpoch, submittedEvents, "submitClaimsAndUpdateDatabase(ClaimSubmitted)")
	if shouldDrop {
		return claimDropped(err)
	}
	if err != nil {
		return claimRetryLater(err)
	}
	if currEvent != nil {
		return s.recordSubmittedEvent(app, currEpoch, currEvent)
	}

	if prevEpoch != nil && prevEpoch.Status != model.EpochStatus_ClaimAccepted {
		s.Logger.Debug("Waiting previous claim to be accepted before submitting new one. Previous:",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(prevEpoch.OutputsMerkleRoot),
			"last_block", prevEpoch.LastBlock,
		)
		return claimNoProgress()
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
		if reconciled, err := s.reconcileBeforeSubmit(app, currEpoch, defaultBlockNumber); reconciled {
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
			if ferr := s.forecloseClaim(app, currEpoch, "submitClaimsAndUpdateDatabase"); ferr != nil {
				return claimDropped(ferr)
			}
			return claimWorkCompleted(1)
		}
	}

	if s.submissionEnabled {
		return s.broadcastComputedClaim(ic, app, currEpoch, defaultBlockNumber)
	}
	return claimNoProgress()
}

func (s *Service) reconcileComputedAcceptedEvent(
	work computedClaimWork,
	defaultBlockNumber *big.Int,
) (claimStepResult, bool) {
	app := work.app
	currEpoch := work.epoch
	prevEpoch := work.prevEpoch

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
	if prevEpoch != nil {
		acceptScanFrom = prevEpoch.LastBlock + 1
	}
	_, foreignAccepted, _, err := s.blockchain.findClaimAcceptedEventAndSucc(
		s.Context, app, currEpoch, acceptScanFrom, defaultBlockNumber.Uint64(),
	)
	if err != nil {
		return claimDropped(fmt.Errorf(
			"scanning ClaimAccepted for computed epoch %d (%d): %w",
			currEpoch.Index, currEpoch.VirtualIndex, err)), true
	}
	if foreignAccepted == nil {
		return claimNoProgress(), false
	}
	matches, ok := claimAcceptedEventMatches(app, currEpoch, foreignAccepted)
	if !ok {
		return claimDropped(s.markMatcherPrecondFailure(app, currEpoch, "submitClaimsAndUpdateDatabase(ClaimAccepted)")), true
	}
	if !matches {
		return claimDropped(s.markAcceptedDivergence(app, currEpoch, foreignAccepted, "submitClaimsAndUpdateDatabase")), true
	}
	acceptedTxHash := foreignAccepted.Raw.TxHash
	if err := s.repository.UpdateEpochWithAcceptedClaim(
		s.Context, currEpoch.ApplicationID, currEpoch.Index, &acceptedTxHash); err != nil {
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
	work computedClaimWork,
	defaultBlockNumber *big.Int,
) (*iconsensus.IConsensus, []*iconsensus.IConsensusClaimSubmitted, claimStepResult, bool) {
	app := work.app
	currEpoch := work.epoch
	prevEpoch := work.prevEpoch

	var ic *iconsensus.IConsensus
	var submittedEvents []*iconsensus.IConsensusClaimSubmitted
	var err error
	if prevEpoch != nil {
		ic, submittedEvents, err = s.findClaimSubmittedEventAndSucc(
			s.Context, app, prevEpoch, currEpoch, prevEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
		)
	} else {
		ic, submittedEvents, err = s.blockchain.findClaimSubmittedEventAndSucc(
			s.Context, app, currEpoch, currEpoch.LastBlock+1, defaultBlockNumber.Uint64(),
		)
	}
	if err != nil {
		return nil, nil, claimDropped(err), true
	}
	return ic, submittedEvents, claimNoProgress(), false
}

func (s *Service) recordSubmittedEvent(
	app *model.Application,
	currEpoch *model.Epoch,
	currEvent *iconsensus.IConsensusClaimSubmitted,
) claimStepResult {
	s.Logger.Debug("Updating claim status to submitted",
		"app", app.IApplicationAddress,
		"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
		"last_block", currEpoch.LastBlock,
	)
	txHash := currEvent.Raw.TxHash
	err := s.repository.UpdateEpochWithSubmittedClaim(
		s.Context,
		currEpoch.ApplicationID,
		currEpoch.Index,
		txHash,
	)
	if err != nil {
		return claimDropped(err)
	}
	s.dropClaimInFlight(app.ID)
	s.Logger.Info("Claim previously submitted",
		"app", app.IApplicationAddress,
		"event_block_number", currEvent.Raw.BlockNumber,
		"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
		"last_block", currEpoch.LastBlock,
	)
	return claimProgressed(1)
}

func (s *Service) broadcastComputedClaim(
	ic *iconsensus.IConsensus,
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) claimStepResult {
	s.Logger.Debug("Submitting claim to blockchain",
		"app", app.IApplicationAddress,
		"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
		"last_block", currEpoch.LastBlock,
	)
	txHash, err := s.blockchain.submitClaimToBlockchain(ic, app, currEpoch)
	if err != nil {
		switch outcome, stateErr := s.handleSubmitClaimRevert(err, app, currEpoch); outcome {
		case submitClaimAlreadyOnChain:
			return claimNoProgress()
		case submitClaimRetryLater:
			// Keep currEpoch in computedEpochs so the next tick retries.
			return claimNoProgress()
		case submitClaimAppHalted:
			return claimDropped(stateErr)
		case submitClaimUnknown:
			return claimDropped(err)
		default:
			// A new submitClaimRevertOutcome was added, but this switch was not
			// updated. Return the error so the bug is visible in logs. The epoch
			// stays in computedEpochs, so a later tick can try again.
			s.Logger.Error("unhandled submitClaimRevertOutcome; treating as retry-later",
				"outcome", outcome,
				"app", app.IApplicationAddress,
				"epoch_index", currEpoch.Index,
				"error", err)
			return claimRetryLater(fmt.Errorf("unhandled submitClaimRevertOutcome %d: %w", outcome, err))
		}
	}
	s.putClaimInFlight(app.ID, inFlightTx{
		txHash:         txHash,
		firstSeenBlock: defaultBlockNumber.Uint64(),
	})
	return claimProgressed(1)
}

// reconcileBeforeSubmit calls getClaim before submitClaim.
//
// If the chain already has this claim as STAGED or ACCEPTED, update the DB and
// do not send another transaction. Returns (reconciled, err):
//   - (true, nil): the DB was updated to STAGED or ACCEPTED.
//   - (false, nil): the chain says UNSTAGED; caller may submit.
//   - (_, err): an error occurred; caller should drop this work item.
//
// All chain reads in one tick use the same finalized block number.
func (s *Service) reconcileBeforeSubmit(
	app *model.Application,
	currEpoch *model.Epoch,
	defaultBlockNumber *big.Int,
) (bool, error) {
	claim, err := s.blockchain.getClaimStatus(s.Context, app, currEpoch, defaultBlockNumber)
	if err != nil {
		return false, fmt.Errorf("pre-submit getClaim (app=%v, epoch=%d): %w",
			app.IApplicationAddress, currEpoch.Index, err)
	}
	switch claim.Status {
	case claimStatusAccepted:
		if err := s.updateEpochAcceptedFromClaimStatus(app, currEpoch, claim, "reconcileBeforeSubmit"); err != nil {
			return false, fmt.Errorf("reconciling epoch %d (%d) to ACCEPTED: %w",
				currEpoch.Index, currEpoch.VirtualIndex, err)
		}
		s.Logger.Info("Claim already accepted on chain (reconciled pre-submit)",
			"app", app.IApplicationAddress,
			"epoch_index", currEpoch.Index,
			"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
			"last_block", currEpoch.LastBlock,
		)
		return true, nil
	case claimStatusStaged:
		stagingBlock, err := s.updateEpochStagedFromClaimStatus(app, currEpoch, claim, "reconcileBeforeSubmit")
		if err != nil {
			return false, fmt.Errorf("reconciling epoch %d (%d) to STAGED: %w",
				currEpoch.Index, currEpoch.VirtualIndex, err)
		}
		s.Logger.Info("Claim already staged on chain (reconciled pre-submit)",
			"app", app.IApplicationAddress,
			"epoch_index", currEpoch.Index,
			"outputs_merkle_root", hashToHex(currEpoch.OutputsMerkleRoot),
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
