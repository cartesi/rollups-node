// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type rootBondRecovery struct {
	EpochIndex        uint64
	Tournament        common.Address
	TxHash            *common.Hash
	FirstMissingBlock *uint64
}

// discoverForeclosedRootBond finds the current root even when foreclosure
// prevented an accept attempt. Only published, signer-owned recoverable bonds
// enter the queue. Running roots remain eligible for discovery on a later tick.
func (s *Service) discoverForeclosedRootBond(
	ctx context.Context, app *model.Application, epochs []*model.Epoch, consensus DaveConsensusAdapter, observedBlock uint64,
) error {
	if !s.submissionEnabled || consensus == nil || !app.ForeclosureScanCaughtUp() ||
		observedBlock < app.ForecloseBlock || observedBlock < app.LastTournamentCheckBlock {
		return nil
	}
	sealed, err := consensus.GetCurrentSealedEpoch(pinnedCallOpts(ctx, observedBlock))
	if err != nil {
		return fmt.Errorf("reading foreclosed current root for bond recovery: %w", err)
	}
	if s.discoveredForeclosedRootBonds[app.ID] == sealed.Tournament {
		return nil
	}
	for _, epoch := range epochs {
		if epoch.Index != sealed.EpochNumber || epoch.TournamentAddress == nil ||
			*epoch.TournamentAddress != sealed.Tournament || epoch.Commitment == nil {
			continue
		}
		tournament, err := s.repository.GetTournament(ctx, app.IApplicationAddress.Hex(), sealed.Tournament.Hex())
		if err != nil {
			return fmt.Errorf("loading foreclosed current root for bond recovery: %w", err)
		}
		if tournament == nil || tournament.Snapshot.AsOfBlock > app.LastTournamentCheckBlock ||
			tournament.Snapshot.AsOfBlock > observedBlock {
			return nil
		}
		recovery := tournament.Snapshot.BondRecovery
		if recovery.Disposition != model.BondDispositionRecoverable || recovery.Claimer == nil ||
			*recovery.Claimer != s.txOptsFactory.From() {
			return nil
		}
		s.queueRootBondRecovery(app.ID, epoch.Index, sealed.Tournament)
		// Keep this marker after retirement: a successful transaction can have
		// a failed payment push, which must not restart an automatic retry loop.
		// Restart clears both the marker and the existing in-memory queue.
		s.markForeclosedRootBondDiscovered(app.ID, sealed.Tournament)
		return nil
	}
	return nil
}

func (s *Service) markForeclosedRootBondDiscovered(appID int64, tournament common.Address) {
	if s.discoveredForeclosedRootBonds == nil {
		s.discoveredForeclosedRootBonds = make(map[int64]common.Address)
	}
	s.discoveredForeclosedRootBonds[appID] = tournament
}

func (s *Service) queueRootBondRecovery(appID int64, epochIndex uint64, tournament common.Address) {
	for _, candidate := range s.rootBondRecoveries[appID] {
		if candidate.EpochIndex == epochIndex && candidate.Tournament == tournament {
			return
		}
	}
	s.rootBondRecoveries[appID] = append(s.rootBondRecoveries[appID], &rootBondRecovery{
		EpochIndex: epochIndex,
		Tournament: tournament,
	})
	s.Logger.Info("Queued root tournament bond recovery candidate; restart clears this in-memory candidate",
		"application_id", appID,
		"epoch_index", epochIndex,
		"tournament", tournament)
}

func (s *Service) rootBondRecoveryInFlight(appID int64) (*rootBondRecovery, error) {
	var found *rootBondRecovery
	for _, candidate := range s.rootBondRecoveries[appID] {
		if candidate.TxHash == nil {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("application %d has multiple root bond recovery transactions in flight", appID)
		}
		found = candidate
	}
	return found, nil
}

func (s *Service) hasNonRecoveryMutationInFlight(appID int64) bool {
	_, pending := s.pendingTransactions[appID]
	return pending
}

func (s *Service) retireRootBondRecovery(appID int64, target *rootBondRecovery) {
	candidates := s.rootBondRecoveries[appID]
	for i, candidate := range candidates {
		if candidate != target {
			continue
		}
		candidates = append(candidates[:i], candidates[i+1:]...)
		if len(candidates) == 0 {
			delete(s.rootBondRecoveries, appID)
		} else {
			s.rootBondRecoveries[appID] = candidates
		}
		return
	}
}

// recoverRootBonds reconciles queued root tournament bonds from oldest to
// newest. It can submit at most one recovery transaction in one call.
func (s *Service) recoverRootBonds(ctx context.Context, app *model.Application, mostRecentBlock uint64) error {
	if len(s.rootBondRecoveries[app.ID]) == 0 {
		return nil
	}
	if s.hasNonRecoveryMutationInFlight(app.ID) {
		return nil
	}

	inFlight, err := s.rootBondRecoveryInFlight(app.ID)
	if err != nil {
		return err
	}
	if inFlight != nil {
		return s.reconcileRootBondRecoveryTransaction(ctx, app, inFlight, mostRecentBlock)
	}

	consensus, err := s.adapterFactory.CreateDaveConsensusAdapter(app.IConsensusAddress)
	if err != nil {
		return fmt.Errorf("binding DaveConsensus for root bond recovery: %w", err)
	}
	callOpts := pinnedCallOpts(ctx, mostRecentBlock)
	sealed, err := consensus.GetCurrentSealedEpoch(callOpts)
	if err != nil {
		return fmt.Errorf("reading current epoch for root bond recovery: %w", err)
	}

	for len(s.rootBondRecoveries[app.ID]) > 0 {
		candidate := s.rootBondRecoveries[app.ID][0]
		// Live apps recover only after acceptance advances the sealed epoch.
		// Foreclosure prevents acceptance, but the current root can still
		// finish and release its bond. Its disposition and claimer decide below.
		if candidate.EpochIndex > sealed.EpochNumber ||
			(candidate.EpochIndex == sealed.EpochNumber && !app.IsForeclosed()) {
			return nil
		}
		mutated, err := s.recoverRootBond(ctx, app, candidate, callOpts)
		if err != nil || mutated {
			return err
		}
		if len(s.rootBondRecoveries[app.ID]) > 0 && s.rootBondRecoveries[app.ID][0] == candidate {
			return nil
		}
	}
	return nil
}

func (s *Service) recoverRootBond(
	ctx context.Context,
	app *model.Application,
	candidate *rootBondRecovery,
	callOpts *bind.CallOpts,
) (bool, error) {
	tournament, err := s.adapterFactory.CreateTournamentAdapter(candidate.Tournament)
	if err != nil {
		return false, fmt.Errorf("binding root tournament %s for bond recovery: %w", candidate.Tournament, err)
	}
	recovery, err := tournament.BondRecovery(callOpts)
	if err != nil {
		return false, fmt.Errorf("reading root bond recovery for tournament %s: %w", candidate.Tournament, err)
	}

	switch recovery.Disposition {
	case model.BondDispositionTournamentRunning:
		s.Logger.Warn("Root tournament bond is still running",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		return false, nil
	case model.BondDispositionNoWinner:
		s.Logger.Error("Root tournament bond has no winner and cannot be recovered",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		s.retireRootBondRecovery(app.ID, candidate)
		return false, nil
	case model.BondDispositionRecovered:
		s.Logger.Info("Root tournament bond is recovered",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		s.retireRootBondRecovery(app.ID, candidate)
		return false, nil
	case model.BondDispositionRecoverable:
		if recovery.Claimer != s.txOptsFactory.From() {
			s.Logger.Info("Root tournament bond recovery candidate belongs to another claimer; retiring candidate",
				"application", app.Name,
				"epoch_index", candidate.EpochIndex,
				"tournament", candidate.Tournament,
				"claimer", recovery.Claimer,
				"payment", recovery.Payment)
			s.retireRootBondRecovery(app.ID, candidate)
			return false, nil
		}
		if err := s.broadcastRootBondRecovery(ctx, app, candidate, tournament, recovery); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, fmt.Errorf("root tournament %s returned unknown bond disposition %s",
			candidate.Tournament, recovery.Disposition)
	}
}

func (s *Service) broadcastRootBondRecovery(
	ctx context.Context,
	app *model.Application,
	candidate *rootBondRecovery,
	tournament TournamentAdapter,
	recovery BondRecovery,
) error {
	txCtx, cancel := context.WithTimeout(ctx, s.submissionTimeout)
	defer cancel()
	txOpts, err := s.txOptsFactory.NewTransactOpts(txCtx)
	if err != nil {
		return fmt.Errorf("creating transaction options to recover epoch %d root bond: %w", candidate.EpochIndex, err)
	}
	tx, err := tournament.TryRecoveringBond(txOpts)
	if err != nil {
		return s.handleRootBondRecoveryRevert(app, candidate, err)
	}
	if tx == nil {
		return errors.New("root bond recovery returned a nil transaction")
	}
	txHash := tx.Hash()
	candidate.TxHash = &txHash
	candidate.FirstMissingBlock = nil
	s.Logger.Info("Sent root tournament bond recovery transaction",
		"application", app.Name,
		"epoch_index", candidate.EpochIndex,
		"tournament", candidate.Tournament,
		"claimer", recovery.Claimer,
		"payment", recovery.Payment,
		"tx", txHash)
	return nil
}

func (s *Service) handleRootBondRecoveryRevert(
	app *model.Application,
	candidate *rootBondRecovery,
	err error,
) error {
	switch {
	case isTournamentError(err, "TournamentNotFinished"):
		s.Logger.Warn("Root bond recovery observed a running tournament; waiting for a fresh view",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		return nil
	case isTournamentError(err, "NoWinner"):
		s.Logger.Warn("Root bond recovery observed no winner; waiting for a fresh view",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		return nil
	case ethutil.IsNonceTooLowError(err):
		s.Logger.Info("Root bond recovery nonce is too low; waiting for chain reconciliation",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		return nil
	default:
		return err
	}
}

func (s *Service) reconcileRootBondRecoveryTransaction(
	ctx context.Context,
	app *model.Application,
	candidate *rootBondRecovery,
	mostRecentBlock uint64,
) error {
	txHash := *candidate.TxHash
	_, pending, err := s.client.TransactionByHash(ctx, txHash)
	missing := errors.Is(err, ethereum.NotFound)
	if err != nil && !missing {
		return fmt.Errorf("checking root bond recovery transaction %s: %w", txHash, err)
	}
	if !missing {
		candidate.FirstMissingBlock = nil
	}
	if !missing && pending {
		return nil
	}
	receipt, err := s.client.TransactionReceipt(ctx, txHash)
	if missing && errors.Is(err, ethereum.NotFound) {
		if candidate.FirstMissingBlock == nil || mostRecentBlock < *candidate.FirstMissingBlock {
			candidate.FirstMissingBlock = new(mostRecentBlock)
			return nil
		}
		if mostRecentBlock-*candidate.FirstMissingBlock >= maxMissingTournamentTransactionBlocks {
			s.Logger.Warn("Root bond recovery transaction remains missing; waiting for fresh bond state before retry",
				"application", app.Name, "epoch_index", candidate.EpochIndex, "tournament", candidate.Tournament,
				"tx", txHash, "missing_since_block", *candidate.FirstMissingBlock, "latest_block", mostRecentBlock)
			candidate.TxHash, candidate.FirstMissingBlock = nil, nil
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("fetching root bond recovery receipt %s: %w", txHash, err)
	}
	if receipt == nil {
		return fmt.Errorf("root bond recovery transaction %s has an invalid receipt block", txHash)
	}
	if _, err := checkedUint64(receipt.BlockNumber, "root bond recovery transaction receipt block"); err != nil {
		return err
	}
	if receipt.TxHash != txHash {
		return fmt.Errorf("root bond recovery receipt hash %s differs from transaction %s", receipt.TxHash, txHash)
	}
	if receipt.Status != types.ReceiptStatusFailed && receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("root bond recovery transaction %s has invalid receipt status %d", txHash, receipt.Status)
	}
	candidate.FirstMissingBlock = nil
	callBlock := new(big.Int).SetUint64(mostRecentBlock)
	if receipt.BlockNumber.Cmp(callBlock) > 0 {
		callBlock.Set(receipt.BlockNumber)
	}

	tournament, err := s.adapterFactory.CreateTournamentAdapter(candidate.Tournament)
	if err != nil {
		return fmt.Errorf("binding root tournament %s after recovery transaction: %w", candidate.Tournament, err)
	}
	recovery, err := tournament.BondRecovery(&bind.CallOpts{Context: ctx, BlockNumber: callBlock})
	if err != nil {
		if errors.Is(err, errInvalidBondRecovery) {
			candidate.TxHash = nil
		}
		return fmt.Errorf("reading root bond state after transaction %s: %w", txHash, err)
	}

	switch recovery.Disposition {
	case model.BondDispositionRecovered:
		s.Logger.Info("Root tournament bond recovery is confirmed",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament,
			"tx", txHash,
			"status", receipt.Status)
		s.retireRootBondRecovery(app.ID, candidate)
		return nil
	case model.BondDispositionRecoverable:
		if recovery.Claimer != s.txOptsFactory.From() {
			s.Logger.Warn("Root tournament bond now belongs to another claimer; retiring recovery",
				"application", app.Name,
				"epoch_index", candidate.EpochIndex,
				"tournament", candidate.Tournament,
				"tx", txHash,
				"status", receipt.Status,
				"claimer", recovery.Claimer,
				"payment", recovery.Payment)
			s.retireRootBondRecovery(app.ID, candidate)
			return nil
		}
		if receipt.Status == types.ReceiptStatusFailed {
			candidate.TxHash = nil
			s.Logger.Warn("Root tournament bond recovery transaction reverted; waiting for a fresh retry",
				"application", app.Name,
				"epoch_index", candidate.EpochIndex,
				"tournament", candidate.Tournament,
				"tx", txHash,
				"claimer", recovery.Claimer,
				"payment", recovery.Payment)
			return nil
		}
		s.Logger.Error("Root tournament bond payment failed; suppressing automatic retry",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament,
			"tx", txHash,
			"status", receipt.Status,
			"claimer", recovery.Claimer,
			"payment", recovery.Payment,
			"outcome", "failed_push")
		if app.IsForeclosed() {
			// A candidate queued before foreclosure can finish before the
			// observation view catches up. Later discovery must not retry it.
			s.markForeclosedRootBondDiscovered(app.ID, candidate.Tournament)
		}
		s.retireRootBondRecovery(app.ID, candidate)
		return nil
	case model.BondDispositionNoWinner:
		s.Logger.Error("Root tournament bond has no winner after recovery transaction",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament,
			"tx", txHash,
			"status", receipt.Status)
		s.retireRootBondRecovery(app.ID, candidate)
		return nil
	case model.BondDispositionTournamentRunning:
		candidate.TxHash = nil
		return fmt.Errorf("root tournament %s reports a running bond after mined recovery transaction %s",
			candidate.Tournament, txHash)
	default:
		candidate.TxHash = nil
		return fmt.Errorf("root tournament %s returned unknown bond disposition %s after transaction %s",
			candidate.Tournament, recovery.Disposition, txHash)
	}
}
