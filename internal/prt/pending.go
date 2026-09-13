// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type tournamentAction string

const (
	tournamentActionJoin   tournamentAction = "join"
	tournamentActionStage  tournamentAction = "stage"
	tournamentActionAccept tournamentAction = "accept"
)

// Match the claimer's 64-block recovery horizon, but count latest blocks from
// the first missing lookup, not finalized blocks from submission. This releases
// stale local exclusion only; it cannot ensure delivery before a join deadline.
const maxMissingTournamentTransactionBlocks uint64 = 64

// pendingTournamentTransaction is a local exclusion slot, not a delivery queue.
// It is lost on restart. Known-pending transactions have no retry deadline.
type pendingTournamentTransaction struct {
	Action            tournamentAction
	Hash              common.Hash
	EpochIndex        uint64
	FirstMissingBlock *uint64
}

// waitForTournamentTransaction blocks new actions for this tick, including when
// a mined or missing transaction releases its slot. The next tick reads fresh state.
// Receipts provide diagnostics; they do not change stored claim state.
func (s *Service) waitForTournamentTransaction(ctx context.Context, app *model.Application, latestBlock uint64) (bool, error) {
	tx, exists := s.pendingTransactions[app.ID]
	if !exists {
		return false, nil
	}
	_, pending, err := s.client.TransactionByHash(ctx, tx.Hash)
	missing := errors.Is(err, ethereum.NotFound)
	if err != nil && !missing {
		return true, fmt.Errorf("checking %s transaction %s: %w", tx.Action, tx.Hash, err)
	}
	if !missing {
		tx.FirstMissingBlock = nil
		s.pendingTransactions[app.ID] = tx
	}
	if !missing && pending {
		s.Logger.Debug("Tournament transaction is still pending",
			"application", app.Name, "epoch_index", tx.EpochIndex, "action", tx.Action, "tx", tx.Hash)
		return true, nil
	}
	// A transaction lookup can be missing while its receipt is available.
	// Require both lookups to report NotFound before aging the local slot.
	receipt, err := s.client.TransactionReceipt(ctx, tx.Hash)
	if missing && errors.Is(err, ethereum.NotFound) {
		s.waitForMissingTournamentTransaction(app, tx, latestBlock)
		return true, nil
	}
	if err != nil {
		return true, fmt.Errorf("reading %s transaction receipt %s: %w", tx.Action, tx.Hash, err)
	}
	if receipt == nil || receipt.TxHash != tx.Hash {
		return true, fmt.Errorf("%s transaction %s has a missing or mismatched receipt", tx.Action, tx.Hash)
	}
	if _, err := checkedUint64(receipt.BlockNumber, "tournament transaction receipt block"); err != nil {
		return true, err
	}
	if receipt.Status != types.ReceiptStatusFailed && receipt.Status != types.ReceiptStatusSuccessful {
		return true, fmt.Errorf("%s transaction %s has invalid receipt status %d", tx.Action, tx.Hash, receipt.Status)
	}
	delete(s.pendingTransactions, app.ID)
	if receipt.Status == types.ReceiptStatusFailed {
		s.Logger.Error("Tournament transaction reverted; waiting for a fresh chain snapshot",
			"application", app.Name, "epoch_index", tx.EpochIndex, "action", tx.Action, "tx", tx.Hash)
	} else {
		s.Logger.Debug("Tournament transaction was mined; waiting for a fresh chain snapshot",
			"application", app.Name, "epoch_index", tx.EpochIndex, "action", tx.Action, "tx", tx.Hash)
	}
	return true, nil
}

func (s *Service) waitForMissingTournamentTransaction(
	app *model.Application, tx pendingTournamentTransaction, latestBlock uint64,
) {
	if tx.FirstMissingBlock == nil || latestBlock < *tx.FirstMissingBlock {
		tx.FirstMissingBlock = new(latestBlock)
		s.pendingTransactions[app.ID] = tx
		return
	}
	age := latestBlock - *tx.FirstMissingBlock
	if age < maxMissingTournamentTransactionBlocks {
		return
	}
	delete(s.pendingTransactions, app.ID)
	s.Logger.Warn("Tournament transaction remains missing; releasing local slot for fresh chain reconciliation",
		"application", app.Name, "epoch_index", tx.EpochIndex, "action", tx.Action, "tx", tx.Hash,
		"missing_since_block", *tx.FirstMissingBlock, "latest_block", latestBlock, "age_blocks", age,
		"timeout_blocks", maxMissingTournamentTransactionBlocks)
}
