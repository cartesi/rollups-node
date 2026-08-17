// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"

	"github.com/ethereum/go-ethereum/core/types"
)

// stageReceiptOutcome describes what tryStageFromReceipt found in a submit
// transaction receipt. The caller uses it to choose the next local DB update.
type stageReceiptOutcome int

const (
	// stageReceiptNoMatch means the receipt has no ClaimStaged event for our
	// epoch. This is normal for Quorum votes that are not the deciding vote.
	// The caller should only move COMPUTED -> SUBMITTED.
	stageReceiptNoMatch stageReceiptOutcome = iota

	// stageReceiptStaged means the receipt has a matching ClaimStaged event.
	// The DB was moved COMPUTED -> SUBMITTED -> STAGED in one transaction.
	stageReceiptStaged

	// stageReceiptDivergent means the receipt has ClaimStaged for our app and
	// epoch, but the outputs or machine root are different. The app was set to
	// DIVERGED with a reason that names this claim stage.
	stageReceiptDivergent

	// stageReceiptPrecondFailure means local data was missing, so we could not
	// compare the event with our epoch. The app was set to CORRUPTED, which is
	// terminal: the missing outputs_merkle_root / machine_hash cannot be
	// reconstructed by restarting, so the app is not re-enableable.
	stageReceiptPrecondFailure

	// stageReceiptDBPending means the receipt matched, but the DB write failed.
	// Do not fall back to only marking the epoch SUBMITTED. That would hide the
	// STAGED event already seen in this receipt, and a DB outage could make the
	// next staging scan miss the same signal again. Keep the in-flight
	// transaction and retry the same receipt on the next tick.
	stageReceiptDBPending
)

// tryStageFromReceipt searches a transaction receipt for a ClaimStaged event
// that matches the given epoch. In v3 contracts:
//   - Authority's submitClaim ALWAYS emits ClaimSubmitted + ClaimStaged in
//     the same transaction (Authority.sol:35-66).
//   - Quorum's submitClaim emits ClaimStaged in the same transaction only when
//     submission is the deciding vote (Quorum.sol:116-123).
//
// In both cases this function records COMPUTED -> SUBMITTED -> STAGED with one
// DB transaction. A crash cannot leave only part of that state change written.
//
// Note: in v3 the contract NEVER emits ClaimAccepted in the same tx as
// ClaimSubmitted, regardless of claimStagingPeriod. The acceptClaim path is
// always a separate transaction. Code that tries to accept from the submit
// receipt is using the wrong contract model.
func (s *Service) tryStageFromReceipt(
	receipt *types.Receipt,
	app *model.Application,
	epoch *model.Epoch,
) (stageReceiptOutcome, error) {
	ic, err := iconsensus.NewIConsensus(app.IConsensusAddress, nil)
	if err != nil {
		s.Logger.Warn("staging fast-path: failed to create ABI binding",
			"app", app.IApplicationAddress, "error", err)
		return stageReceiptNoMatch, nil
	}
	for _, log := range receipt.Logs {
		// Only use logs from the consensus contract. A different contract could
		// emit a log with the same topic hash. The ABI parser checks the topic,
		// not the address, so we check the address here first.
		if log.Address != app.IConsensusAddress {
			continue
		}
		event, err := ic.ParseClaimStaged(*log)
		if err != nil {
			continue // not a ClaimStaged event
		}
		if !claimStagedEventMatchesEpoch(app, epoch, event) {
			continue
		}
		matches, ok := claimStagedEventMatches(app, epoch, event)
		if !ok {
			pErr := s.markMatcherPrecondFailure(app, epoch, "tryStageFromReceipt")
			return stageReceiptPrecondFailure, pErr
		}
		if !matches {
			// Same app and epoch, but different outputs or machine root. For
			// Authority, the owner produced a different state. For Quorum, this
			// receipt should be from our own transaction, so seeing another
			// state here is also a fault. Mark the app DIVERGED and return a
			// special result so the caller does not log success or fall back to
			// the plain SUBMITTED update.
			divErr := s.markStagingDivergence(app, epoch, event, "tryStageFromReceipt")
			return stageReceiptDivergent, divErr
		}
		err = s.repository.UpdateEpochThroughStaging(
			s.Context, epoch.ApplicationID, epoch.Index,
			receipt.TxHash, log.BlockNumber)
		if err != nil {
			return stageReceiptDBPending, fmt.Errorf(
				"UpdateEpochThroughStaging (app=%s, epoch=%d): %w",
				app.IApplicationAddress, epoch.Index, err)
		}
		s.Logger.Info("Claim staged (fast path)",
			"app", app.IApplicationAddress,
			"epoch_index", epoch.Index,
			"outputs_merkle_root", hashToHex(epoch.TxBufferDataBlock),
			"last_block", epoch.LastBlock,
			"staged_at_block", log.BlockNumber,
			"tx", receipt.TxHash)
		return stageReceiptStaged, nil
	}
	// No matching ClaimStaged event in this receipt. This is normal for Quorum
	// votes that are not the deciding vote. A later staging scan will find the
	// ClaimStaged event when it exists.
	return stageReceiptNoMatch, nil
}

// stageClaimsAndUpdateDatabase looks for ClaimStaged events on chain and moves
// local epochs from CLAIM_SUBMITTED to CLAIM_STAGED.
//
// If the event is for our epoch but has a different machine root, the app is
// set to DIVERGED. The reason tells the guardian to call foreclose() before
// the staging period ends.
func (s *Service) stageClaimsAndUpdateDatabase(
	acceptedEpochs map[int64]*model.Epoch,
	submittedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	defaultBlockNumber *big.Int,
) (int, []error) {
	transitions := 0
	errs := []error{}

	for key, currEpoch := range submittedEpochs {
		result := s.processSubmittedClaim(submittedClaimWork{
			app:       apps[key],
			prevEpoch: acceptedEpochs[key],
			epoch:     currEpoch,
		}, defaultBlockNumber)
		transitions += result.progress
		if result.err != nil {
			errs = append(errs, result.err)
		}
		if result.drop {
			delete(submittedEpochs, key)
		}
	}
	return transitions, errs
}

func (s *Service) processSubmittedClaim(
	work submittedClaimWork,
	defaultBlockNumber *big.Int,
) claimStepResult {
	app := work.app
	currEpoch := work.epoch
	prevEpoch := work.prevEpoch

	if err := s.checkConsensusForAddressChange(app, defaultBlockNumber); err != nil {
		return claimDropped(err)
	}

	fromBlock := currEpoch.LastBlock + 1
	if prevEpoch != nil {
		fromBlock = prevEpoch.LastBlock + 1
	}

	_, currEvent, _, err := s.blockchain.findClaimStagedEventAndSucc(
		s.Context, app, currEpoch, fromBlock, defaultBlockNumber.Uint64(),
	)
	if err != nil {
		return claimDropped(err)
	}

	if currEvent != nil {
		s.Logger.Debug("Found ClaimStaged Event",
			"app", currEvent.AppContract,
			"outputs_merkle_root", fmt.Sprintf("%x", currEvent.OutputsMerkleRoot),
			"machine_hash", fmt.Sprintf("%x", currEvent.MachineMerkleRoot),
			"last_block", currEvent.LastProcessedBlockNumber.Uint64(),
		)
		matches, ok := claimStagedEventMatches(app, currEpoch, currEvent)
		if !ok {
			return claimDropped(s.markMatcherPrecondFailure(app, currEpoch, "stageClaimsAndUpdateDatabase"))
		}
		if !matches {
			return claimDropped(s.markStagingDivergence(app, currEpoch, currEvent, "stageClaimsAndUpdateDatabase"))
		}
		err = s.repository.UpdateEpochToStaged(
			s.Context, currEpoch.ApplicationID, currEpoch.Index,
			currEvent.Raw.BlockNumber)
		if err != nil {
			return claimDropped(err)
		}
		s.Logger.Info("Claim staged",
			"app", app.IApplicationAddress,
			"outputs_merkle_root", hashToHex(currEpoch.TxBufferDataBlock),
			"last_block", currEpoch.LastBlock,
			"staged_at_block", currEvent.Raw.BlockNumber,
		)
		return claimWorkCompleted(1)
	}

	// Foreclosed apps cannot stage new claims. If no matching ClaimStaged event
	// exists up to this block, this submitted claim has no remaining on-chain
	// path.
	if app.ForecloseBlock != 0 {
		if ferr := s.forecloseClaim(app, currEpoch, "stageClaimsAndUpdateDatabase"); ferr != nil {
			return claimDropped(ferr)
		}
		return claimWorkCompleted(1)
	}
	return claimNoProgress()
}
