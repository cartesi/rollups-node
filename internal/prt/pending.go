// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type tournamentAction string

const (
	tournamentActionJoin   tournamentAction = "join"
	tournamentActionStage  tournamentAction = "stage"
	tournamentActionAccept tournamentAction = "accept"
)

// pendingTournamentTransaction is a local exclusion slot, not a delivery queue.
// It is lost on restart. Pending and missing transactions have no retry deadline.
type pendingTournamentTransaction struct {
	Action     tournamentAction
	Hash       common.Hash
	EpochIndex uint64
}

// waitForTournamentTransaction blocks new actions for this tick, including when
// a mined transaction releases its slot. The next tick must read fresh state.
// Receipts provide diagnostics; they do not change stored claim state.
func (s *Service) waitForTournamentTransaction(ctx context.Context, app *Application) (bool, error) {
	tx, exists := s.pendingTransactions[app.ID]
	if !exists {
		return false, nil
	}
	_, pending, err := s.client.TransactionByHash(ctx, tx.Hash)
	if err != nil {
		return true, fmt.Errorf("checking %s transaction %s: %w", tx.Action, tx.Hash, err)
	}
	if pending {
		s.Logger.Debug("Tournament transaction is still pending",
			"application", app.Name, "epoch_index", tx.EpochIndex, "action", tx.Action, "tx", tx.Hash)
		return true, nil
	}
	receipt, err := s.client.TransactionReceipt(ctx, tx.Hash)
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
