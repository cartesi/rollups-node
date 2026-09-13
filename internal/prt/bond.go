// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type rootBondRecovery struct {
	EpochIndex uint64
	Tournament common.Address
	TxHash     *common.Hash
}

// These markers are temporary action guards, not published bond state. Once the
// published observation block reaches the block where payment was checked,
// use the database view again, including after a reorg.
func (s *Service) prunePaidRootBondObservations(appID int64, observedBlock uint64) {
	for tournament, paidAt := range s.paidRootBondObservations[appID] {
		if observedBlock >= paidAt {
			delete(s.paidRootBondObservations[appID], tournament)
		}
	}
	if len(s.paidRootBondObservations[appID]) == 0 {
		delete(s.paidRootBondObservations, appID)
	}
}

func (s *Service) recordPaidRootBond(appID int64, tournament common.Address, checkedBlock uint64) {
	if s.paidRootBondObservations == nil {
		s.paidRootBondObservations = make(map[int64]map[common.Address]uint64)
	}
	if s.paidRootBondObservations[appID] == nil {
		s.paidRootBondObservations[appID] = make(map[common.Address]uint64)
	}
	s.paidRootBondObservations[appID][tournament] = checkedBlock
}

func (s *Service) selectRootBondRecovery(appID int64, epochIndex uint64, tournament common.Address) {
	if s.rootBondRecoveries == nil {
		s.rootBondRecoveries = make(map[int64]*rootBondRecovery)
	}
	s.rootBondRecoveries[appID] = &rootBondRecovery{
		EpochIndex: epochIndex,
		Tournament: tournament,
	}
	s.Logger.Debug("Selected published root tournament bond recovery candidate",
		"application_id", appID,
		"epoch_index", epochIndex,
		"tournament", tournament)
}

func (s *Service) rootBondRecoveryInFlight(appID int64) *rootBondRecovery {
	candidate := s.rootBondRecoveries[appID]
	if candidate != nil && candidate.TxHash != nil {
		return candidate
	}
	return nil
}

func (s *Service) hasNonRecoveryMutationInFlight(appID int64) bool {
	_, pending := s.pendingTransactions[appID]
	return pending
}

// recoverRootBonds maintains an existing action or checks a selected candidate.
// Unsent failures are deferred to a later sweep; sent transactions stay tracked.
func (s *Service) recoverRootBonds(ctx context.Context, app *model.Application, mostRecentBlock uint64) error {
	candidate := s.rootBondRecoveries[app.ID]
	if candidate == nil || s.hasNonRecoveryMutationInFlight(app.ID) {
		return nil
	}
	if candidate.TxHash != nil {
		return s.reconcileRootBondRecoveryTransaction(ctx, app, candidate, mostRecentBlock)
	}
	err := s.recoverRootBond(ctx, app, candidate, pinnedCallOpts(ctx, mostRecentBlock))
	if candidate.TxHash == nil {
		delete(s.rootBondRecoveries, app.ID)
	}
	return err
}

func (s *Service) recoverRootBond(
	ctx context.Context,
	app *model.Application,
	candidate *rootBondRecovery,
	callOpts *bind.CallOpts,
) error {
	tournament, err := s.adapterFactory.CreateTournamentAdapter(candidate.Tournament)
	if err != nil {
		return fmt.Errorf("binding root tournament %s for bond recovery: %w", candidate.Tournament, err)
	}
	recovery, err := tournament.BondRecovery(callOpts)
	if err != nil {
		return fmt.Errorf("reading root bond recovery for tournament %s: %w", candidate.Tournament, err)
	}

	switch recovery.Disposition {
	case model.BondDispositionTournamentRunning:
		s.Logger.Warn("Root tournament bond is still running",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		return nil
	case model.BondDispositionNoWinner:
		s.Logger.Error("Root tournament bond has no winner and cannot be recovered",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		return nil
	case model.BondDispositionRecovered:
		s.Logger.Info("Root tournament bond is recovered",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament)
		s.recordPaidRootBond(app.ID, candidate.Tournament, callOpts.BlockNumber.Uint64())
		return nil
	case model.BondDispositionRecoverable:
		if recovery.Claimer != s.txOptsFactory.From() {
			s.Logger.Info("Root tournament bond recovery candidate belongs to another claimer; retiring candidate",
				"application", app.Name,
				"epoch_index", candidate.EpochIndex,
				"tournament", candidate.Tournament,
				"claimer", recovery.Claimer,
				"payment", recovery.Payment)
			return nil
		}
		return s.broadcastRootBondRecovery(ctx, app, candidate, tournament, recovery)
	default:
		return fmt.Errorf("root tournament %s returned unknown bond disposition %s",
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
		s.Logger.Info("Root bond recovery nonce rejected; check fresh bond state before retry",
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
	if err != nil {
		return fmt.Errorf("checking root bond recovery transaction %s: %w", txHash, err)
	}
	if pending {
		return nil
	}
	receipt, err := s.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return fmt.Errorf("fetching root bond recovery receipt %s: %w", txHash, err)
	}
	if receipt == nil || receipt.BlockNumber == nil || receipt.BlockNumber.Sign() < 0 {
		return fmt.Errorf("root bond recovery transaction %s has an invalid receipt block", txHash)
	}
	if receipt.TxHash != txHash {
		return fmt.Errorf("root bond recovery receipt hash %s differs from transaction %s", receipt.TxHash, txHash)
	}
	if receipt.Status != types.ReceiptStatusFailed && receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("root bond recovery transaction %s has invalid receipt status %d", txHash, receipt.Status)
	}
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
		s.recordPaidRootBond(app.ID, candidate.Tournament, callBlock.Uint64())
		s.Logger.Info("Root tournament bond recovery is confirmed",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament,
			"tx", txHash,
			"status", receipt.Status)
		delete(s.rootBondRecoveries, app.ID)
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
			delete(s.rootBondRecoveries, app.ID)
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
		delete(s.rootBondRecoveries, app.ID)
		return nil
	case model.BondDispositionNoWinner:
		s.Logger.Error("Root tournament bond has no winner after recovery transaction",
			"application", app.Name,
			"epoch_index", candidate.EpochIndex,
			"tournament", candidate.Tournament,
			"tx", txHash,
			"status", receipt.Status)
		delete(s.rootBondRecoveries, app.ID)
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
