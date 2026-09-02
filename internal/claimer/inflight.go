// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// acceptAttemptKey identifies the retry counter for one app and epoch.
type acceptAttemptKey struct {
	appID      int64
	epochIndex uint64
}

type inFlightTx struct {
	txHash         common.Hash
	firstSeenBlock uint64
}

// maxInFlightReceiptNotFoundBlocks controls how long we wait when
// TransactionReceipt returns ethereum.NotFound. After this many blocks, we
// stop treating the transaction as pending and let the claim flow retry.
// The value is expressed in blocks because the claimer already works with the
// configured default block tag. The current value, 64, is two Ethereum epochs.
const maxInFlightReceiptNotFoundBlocks uint64 = 64

func (tx inFlightTx) ageAt(blockNumber *big.Int) uint64 {
	if blockNumber == nil || blockNumber.Sign() < 0 {
		return 0
	}
	current := blockNumber.Uint64()
	if current <= tx.firstSeenBlock {
		return 0
	}
	return current - tx.firstSeenBlock
}

// checkClaimsInFlight checks submitClaim transactions that were already sent.
// When a transaction is confirmed, the matching epoch can move from
// CLAIM_COMPUTED to CLAIM_SUBMITTED.
//
// It returns the number of confirmed state changes and any error.
func (s *Service) checkClaimsInFlight(
	ctx context.Context,
	computedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	endBlock *big.Int,
) (int, error) {
	confirmed := 0
	var errs []error
	for appID, tx := range s.claimsInFlight {
		result := s.processSubmitInFlight(ctx, appID, tx, submitInFlightWork{
			app:   apps[appID],
			epoch: computedEpochs[appID],
		}, endBlock)
		confirmed += result.progress
		if result.drop {
			delete(computedEpochs, appID)
		}
		if result.err != nil {
			errs = append(errs, result.err)
		}
	}
	return confirmed, errors.Join(errs...)
}

func (s *Service) processSubmitInFlight(
	ctx context.Context,
	appID int64,
	tx inFlightTx,
	work submitInFlightWork,
	endBlock *big.Int,
) claimStepResult {
	txHash := tx.txHash
	ready, receipt, err := s.blockchain.pollTransaction(ctx, txHash, endBlock)
	if err != nil {
		s.Logger.Warn("Claim submission receipt lookup failed; keeping tx in flight.",
			"txHash", txHash,
			"err", err,
		)
		return claimRetryLater(fmt.Errorf("polling claim submission transaction %v: %w", txHash, err))
	}
	if !ready {
		age := tx.ageAt(endBlock)
		if receipt == nil && age >= maxInFlightReceiptNotFoundBlocks {
			s.Logger.Warn("Claim submission receipt not found after timeout; retrying claim lifecycle.",
				"app_id", appID,
				"txHash", txHash,
				"age_blocks", age,
				"timeout_blocks", maxInFlightReceiptNotFoundBlocks,
			)
			s.dropClaimInFlight(appID)
		}
		return claimNoProgress()
	}
	if receipt.Status == 0 {
		s.Logger.Warn("Claim submission reverted, retrying.",
			"txHash", txHash,
			"err", err,
		)
		s.dropClaimInFlight(appID)
		return claimNoProgress()
	}
	if work.epoch == nil {
		s.Logger.Warn("unexpected, claim in flight is not a computed epoch.",
			"id", appID,
			"tx", receipt.TxHash)
		s.dropClaimInFlight(appID)
		return claimNoProgress()
	}
	return s.handleConfirmedSubmitInFlight(ctx, appID, txHash, receipt, work)
}

func (s *Service) handleConfirmedSubmitInFlight(
	ctx context.Context,
	appID int64,
	txHash common.Hash,
	receipt *types.Receipt,
	work submitInFlightWork,
) claimStepResult {
	app := work.app
	computedEpoch := work.epoch
	appAddress := common.Address{}
	if app != nil {
		appAddress = app.IApplicationAddress
	}

	// Fast path for v3 contracts: the submitClaim receipt may also contain
	// ClaimStaged for the same app, epoch, outputs, and machine root. When it
	// does, write COMPUTED -> SUBMITTED -> STAGED in one DB transaction.
	// Authority always uses this path. Quorum uses it for the deciding vote.
	outcome := stageReceiptNoMatch
	var divErr error
	if app != nil {
		outcome, divErr = s.tryStageFromReceipt(ctx, receipt, app, computedEpoch)
	}
	switch outcome {
	case stageReceiptStaged:
		s.Logger.Info("Claim submitted (and staged in same tx)",
			"app", appAddress,
			"receipt_block_number", receipt.BlockNumber,
			"outputs_merkle_root", hashToHex(computedEpoch.OutputsMerkleRoot),
			"last_block", computedEpoch.LastBlock,
			"tx", txHash)
		s.dropClaimInFlight(appID)
		return claimWorkCompleted(2)
	case stageReceiptDivergent:
		s.Logger.Warn("Submit tx revealed divergent staging; app set DIVERGED",
			"app", appAddress,
			"epoch_index", computedEpoch.Index,
			"last_block", computedEpoch.LastBlock,
			"tx", txHash)
		s.dropClaimInFlight(appID)
		if divErr != nil {
			return claimDropped(fmt.Errorf("handling staging divergence for epoch %d (%d): %w",
				computedEpoch.Index, computedEpoch.VirtualIndex, divErr))
		}
		return claimDropped(nil)
	case stageReceiptPrecondFailure:
		s.Logger.Warn("Submit tx receipt matched our epoch but local row is missing fields; app set CORRUPTED",
			"app", appAddress,
			"epoch_index", computedEpoch.Index,
			"last_block", computedEpoch.LastBlock,
			"tx", txHash)
		s.dropClaimInFlight(appID)
		if divErr != nil {
			return claimDropped(fmt.Errorf("marking app CORRUPTED on matcher pre-cond failure for epoch %d (%d): %w",
				computedEpoch.Index, computedEpoch.VirtualIndex, divErr))
		}
		return claimDropped(nil)
	case stageReceiptDBPending:
		// The receipt matched, but the DB write failed. Keep both the
		// in-flight transaction and the computed epoch. The next tick can read
		// the same receipt and try the DB write again.
		s.Logger.Warn("staging fast-path: atomic DB write failed; deferring to next tick",
			"app", appAddress,
			"epoch_index", computedEpoch.Index,
			"last_block", computedEpoch.LastBlock,
			"tx", txHash,
			"error", divErr)
		return claimRetryLater(divErr)
	case stageReceiptNoMatch:
		err := s.repository.UpdateEpochWithSubmittedClaim(
			ctx,
			computedEpoch.ApplicationID,
			computedEpoch.Index,
			receipt.TxHash,
		)
		if err != nil {
			return claimRetryLater(fmt.Errorf("updating epoch %d (%d) with submitted claim: %w",
				computedEpoch.Index, computedEpoch.VirtualIndex, err))
		}
		s.Logger.Info("Claim submitted",
			"app", appAddress,
			"receipt_block_number", receipt.BlockNumber,
			"outputs_merkle_root", hashToHex(computedEpoch.OutputsMerkleRoot),
			"last_block", computedEpoch.LastBlock,
			"tx", txHash)
		s.dropClaimInFlight(appID)
		return claimWorkCompleted(1)
	default:
		return claimRetryLater(fmt.Errorf("unhandled stageReceiptOutcome %d for tx %v", outcome, txHash))
	}
}

// checkAcceptsInFlight checks acceptClaim transactions that were already sent.
// When a transaction is confirmed, the matching epoch can move to
// CLAIM_ACCEPTED.
func (s *Service) checkAcceptsInFlight(
	ctx context.Context,
	stagedEpochs map[int64]*model.Epoch,
	apps map[int64]*model.Application,
	endBlock *big.Int,
) (int, error) {
	confirmed := 0
	var errs []error
	for appID, tx := range s.acceptsInFlight {
		result, pollErr := s.processAcceptInFlight(ctx, appID, tx, acceptInFlightWork{
			app:   apps[appID],
			epoch: stagedEpochs[appID],
		}, endBlock)
		if pollErr != nil {
			errs = append(errs, pollErr)
			continue
		}
		confirmed += result.progress
		if result.drop {
			delete(stagedEpochs, appID)
		}
		if result.err != nil {
			errs = append(errs, result.err)
		}
	}
	return confirmed, errors.Join(errs...)
}

func (s *Service) processAcceptInFlight(
	ctx context.Context,
	appID int64,
	tx inFlightTx,
	work acceptInFlightWork,
	endBlock *big.Int,
) (claimStepResult, error) {
	txHash := tx.txHash
	ready, receipt, err := s.blockchain.pollTransaction(ctx, txHash, endBlock)
	if err != nil {
		s.Logger.Warn("Accept submission receipt lookup failed; keeping tx in flight.",
			"txHash", txHash, "err", err)
		return claimNoProgress(), fmt.Errorf("polling accept transaction %v: %w", txHash, err)
	}
	if !ready {
		age := tx.ageAt(endBlock)
		if receipt == nil && age >= maxInFlightReceiptNotFoundBlocks {
			s.Logger.Warn("Accept submission receipt not found after timeout; retrying accept lifecycle.",
				"app_id", appID,
				"txHash", txHash,
				"age_blocks", age,
				"timeout_blocks", maxInFlightReceiptNotFoundBlocks,
			)
			s.dropAcceptInFlight(appID)
		}
		return claimNoProgress(), nil
	}

	stagedEpoch := work.epoch
	if stagedEpoch == nil {
		s.Logger.Warn("unexpected: accept-in-flight is not a staged epoch.",
			"id", appID, "tx", receipt.TxHash)
		s.dropAcceptInFlight(appID)
		return claimNoProgress(), nil
	}
	appAddress := common.Address{}
	if work.app != nil {
		appAddress = work.app.IApplicationAddress
	}
	if receipt.Status == 0 {
		return s.handleRevertedAcceptInFlight(ctx, appID, txHash, work, endBlock, appAddress), nil
	}
	return s.handleConfirmedAcceptInFlight(ctx, appID, txHash, receipt, work, appAddress), nil
}

func (s *Service) handleRevertedAcceptInFlight(
	ctx context.Context,
	appID int64,
	txHash common.Hash,
	work acceptInFlightWork,
	endBlock *big.Int,
	appAddress common.Address,
) claimStepResult {
	app := work.app
	stagedEpoch := work.epoch

	// Our acceptClaim transaction reached the chain but reverted. Read the
	// contract state now to understand what happened. This is faster than
	// waiting for the next event scan.
	s.dropAcceptInFlight(appID)
	if app == nil {
		s.Logger.Warn("Accept tx reverted but app record missing; cannot classify.",
			"id", appID, "tx", txHash)
		return claimNoProgress()
	}
	claim, gerr := s.blockchain.getClaimStatus(ctx, app, stagedEpoch, endBlock)
	if gerr != nil {
		s.Logger.Warn("Accept tx reverted; classifying getClaim failed, will retry next tick",
			"app", appAddress, "tx", txHash, "err", gerr)
		return claimNoProgress()
	}
	if app.ForecloseBlock != 0 && claim.Status != claimStatusAccepted {
		// Foreclosed: no on-chain path remains for a non-accepted claim, so
		// terminalize to CLAIM_FORECLOSED rather than marking the app FAILED.
		return s.terminalizeForeclosedStagedClaim(ctx, app, stagedEpoch)
	}
	switch claim.Status {
	case claimStatusAccepted:
		if err := s.updateEpochAcceptedFromClaimStatus(ctx, app, stagedEpoch, claim, "checkAcceptsInFlight"); err != nil {
			return claimRetryLater(fmt.Errorf("reconciling accept-revert front-run for epoch %d (%d): %w",
				stagedEpoch.Index, stagedEpoch.VirtualIndex, err))
		}
		s.dropAcceptAttempt(acceptAttemptKey{stagedEpoch.ApplicationID, stagedEpoch.Index})
		s.Logger.Info("Claim accepted by front-runner (own accept tx reverted; reconciled via getClaim)",
			"app", appAddress, "tx", txHash,
			"outputs_merkle_root", hashToHex(stagedEpoch.OutputsMerkleRoot),
			"last_block", stagedEpoch.LastBlock)
		return claimWorkCompleted(1)
	case claimStatusStaged:
		// Our claim is still STAGED. The transaction reverted for some other
		// reason. The next tick can send acceptClaim again.
		if err := s.verifyClaimOutputsMatch(ctx, app, stagedEpoch, claim, "checkAcceptsInFlight"); err != nil {
			return claimRetryLater(fmt.Errorf("staged-outputs mismatch on accept-revert classification: %w", err))
		}
		s.Logger.Warn("Accept tx reverted but claim still STAGED on chain; will retry next tick",
			"app", appAddress, "tx", txHash,
			"last_block", stagedEpoch.LastBlock)
	case claimStatusUnstaged:
		// The DB says CLAIM_STAGED, but the contract says UNSTAGED. This should
		// not happen when reading a finalized block. Mark the app FAILED so the
		// operator can fix configuration and re-enable it.
		if ferr := appstatus.SetFailedf(ctx, s.Logger, s.repository, app,
			"accept tx %v reverted and getClaim reports UNSTAGED for our "+
				"(app, lpbn, machine) tuple — DB inconsistent with chain; check "+
				"default block and node_config, then re-enable",
			txHash); ferr != nil {
			return claimRetryLater(fmt.Errorf("marking app FAILED after UNSTAGED accept revert: %w", ferr))
		}
		s.Logger.Warn("Accept tx reverted with UNSTAGED chain state — app set FAILED",
			"app", appAddress, "tx", txHash)
	}
	return claimNoProgress()
}

func (s *Service) handleConfirmedAcceptInFlight(
	ctx context.Context,
	appID int64,
	txHash common.Hash,
	receipt *types.Receipt,
	work acceptInFlightWork,
	appAddress common.Address,
) claimStepResult {
	stagedEpoch := work.epoch
	// Normal path: claim_transaction_hash was set when the epoch moved to
	// CLAIM_SUBMITTED. Pass nil so the repository keeps that hash.
	err := s.repository.UpdateEpochWithAcceptedClaim(
		ctx, stagedEpoch.ApplicationID, stagedEpoch.Index, nil)
	if err != nil {
		return claimRetryLater(fmt.Errorf("updating epoch %d (%d) with accepted claim: %w",
			stagedEpoch.Index, stagedEpoch.VirtualIndex, err))
	}
	s.dropAcceptAttempt(acceptAttemptKey{stagedEpoch.ApplicationID, stagedEpoch.Index})

	s.Logger.Info("Claim accepted (own tx)",
		"app", appAddress,
		"receipt_block_number", receipt.BlockNumber,
		"outputs_merkle_root", hashToHex(stagedEpoch.OutputsMerkleRoot),
		"last_block", stagedEpoch.LastBlock,
		"tx", txHash)
	s.dropAcceptInFlight(appID)
	return claimWorkCompleted(1)
}

// cleanupOrphanedInFlight removes in-memory entries that no longer have a
// matching app or epoch in the current work maps.
//
// For example, an app may become FAILED, DIVERGED, CORRUPTED, or disabled
// while a transaction is still in memory. That app will not appear in the next
// DB query result. Without this cleanup, claimsInFlight, acceptsInFlight, and
// acceptAttempts could keep entries forever.
//
// This also covers sent transactions that never get a receipt.
//
// claimsInFlight and acceptsInFlight are keyed by appID. An in-flight submit
// implies the source epoch is still CLAIM_COMPUTED; an in-flight accept implies
// the source epoch is still CLAIM_STAGED. We keep an entry only if that app is
// still in the matching work map. acceptAttempts is keyed by (appID,
// epochIndex), so it is cleared when that staged epoch is gone.
func (s *Service) cleanupOrphanedInFlight(
	computedApps map[int64]*model.Application,
	stagedApps map[int64]*model.Application,
	stagedEpochs map[int64]*model.Epoch,
) {
	for appID, tx := range s.claimsInFlight {
		if _, ok := computedApps[appID]; ok {
			continue
		}
		s.Logger.Debug("Dropping orphaned submit-in-flight entry",
			"app_id", appID, "tx", tx.txHash)
		s.dropClaimInFlight(appID)
	}
	for appID, tx := range s.acceptsInFlight {
		if _, ok := stagedApps[appID]; ok {
			continue
		}
		s.Logger.Debug("Dropping orphaned accept-in-flight entry",
			"app_id", appID, "tx", tx.txHash)
		s.dropAcceptInFlight(appID)
	}
	for key, attempts := range s.acceptAttempts {
		if epoch, ok := stagedEpochs[key.appID]; ok && epoch.Index == key.epochIndex {
			continue
		}
		s.Logger.Debug("Dropping orphaned accept-attempt counter",
			"app_id", key.appID, "epoch_index", key.epochIndex, "attempts", attempts)
		s.dropAcceptAttempt(key)
	}
}
