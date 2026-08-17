// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

const withdrawalLedgerDivergenceReasonPrefix = "withdrawal_ledger_divergence:"

func hasWithdrawalLedgerDivergence(app *Application) bool {
	return app.Status == ApplicationStatus_Corrupted &&
		app.Reason != nil &&
		strings.HasPrefix(*app.Reason, withdrawalLedgerDivergenceReasonPrefix)
}

// checkForPostForeclosureWithdrawals runs once per evmreader tick for each
// foreclosed app whose accounts drive has been proved. It performs a
// FindTransitions search on the on-chain `getNumberOfWithdrawals()` counter
// (monotonic) over
// `[max(accounts_drive_proved_block, last_withdrawal_check_block+1), mostRecent]`.
//
// On each transition block N (the counter increased), filter Withdrawal
// events with a 1-block window `[N, N]`.
//
// After a successful scan, the observed events and the per-app
// last_withdrawal_check_block cursor are committed in one repository
// transaction. That keeps the DB withdrawal count aligned with the cursor, so
// the next tick can use the DB count as the previous counter. On any scan or
// persist failure, neither cursor nor in-memory mirror advances.
func (r *Service) checkForPostForeclosureWithdrawals(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) {
	// A withdrawal-ledger divergence (detected below) means this local ledger
	// cannot be trusted. Stop re-deriving that specific failure every tick, but
	// keep indexing when CORRUPTED came from another subsystem: an output or
	// sealed-epoch disagreement says nothing about the withdrawal ledger.
	if hasWithdrawalLedgerDivergence(app.application) {
		return
	}

	startBlock := app.application.LastWithdrawalCheckBlock + 1
	if floor := app.application.AccountsDriveProvedBlock; startBlock < floor {
		startBlock = floor
	}
	if startBlock > mostRecentBlockNumber {
		return
	}

	query := func(ctx context.Context, block uint64) (*big.Int, error) {
		return app.applicationContract.GetNumberOfWithdrawals(&bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		})
	}

	var withdrawals []*Withdrawal

	onHit := func(block uint64) error {
		blockWithdrawals, err := r.withdrawalsAtBlock(ctx, app, block)
		if err != nil {
			return err
		}
		withdrawals = append(withdrawals, blockWithdrawals...)
		return nil
	}

	prevValue, err := r.previousWithdrawalCount(ctx, app, startBlock)
	if err != nil {
		if abortPostForeclosureLoop(r, err, "getPreviousWithdrawalCount") {
			return
		}
		r.Logger.Error("Failed to read previous withdrawal count",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"start_block", startBlock,
			"error", err)
		return
	}

	_, err = ethutil.FindTransitions(
		ctx,
		startBlock,
		mostRecentBlockNumber,
		prevValue,
		query,
		onHit,
	)
	if err != nil {
		if errors.Is(err, ethutil.ErrMonotonicViolation) {
			// The local withdrawal-row count exceeds the on-chain counter at
			// the scan boundary: the node has recorded withdrawals the chain
			// does not. On a single canonical chain with no reorg this is
			// unreachable, so it can only come from a reorg, an inconsistent
			// RPC, or DB tampering — all of which mean local state has diverged
			// from the chain. Fail closed: mark the app terminal and stop. We
			// deliberately do not advance the cursor or re-seed from the chain;
			// either would silently rewrite committed withdrawal history, which
			// is never acceptable for a ledger of fund movements.
			r.Logger.Error("Withdrawal ledger divergence detected",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"start_block", startBlock,
				"end_block", mostRecentBlockNumber,
				"error", err)
			// setApplicationCorrupted always returns non-nil (the reason text),
			// and logs any DB failure. An existing integrity-terminal status keeps
			// its first cause, so this branch will deliberately detect and report
			// the withdrawal divergence again on a later tick rather than relying
			// on process-local suppression.
			_ = r.setApplicationCorrupted(ctx, app.application,
				withdrawalLedgerDivergenceReasonPrefix+
					" local withdrawal count exceeds chain while scanning from block %d: %v",
				startBlock, err)
			return
		}
		if abortPostForeclosureLoop(r, err, "findTransitionsWithdrawals") {
			return
		}
		r.Logger.Error("Failed to scan withdrawal transitions",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"start_block", startBlock,
			"end_block", mostRecentBlockNumber,
			"error", err)
		return
	}

	if err := r.repository.StoreWithdrawalEvents(
		ctx,
		app.application.ID,
		withdrawals,
		mostRecentBlockNumber,
	); err != nil {
		r.Logger.Error("Failed to persist withdrawal scan",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"withdrawals", len(withdrawals),
			"last_withdrawal_check_block", mostRecentBlockNumber,
			"error", err)
		return
	}

	for _, w := range withdrawals {
		r.Logger.Info("Withdrawal observed",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"account_index", w.AccountIndex,
			"block", w.BlockNumber,
			"transaction_hash", w.TransactionHash,
		)
	}
	app.application.LastWithdrawalCheckBlock = mostRecentBlockNumber
}

func (r *Service) previousWithdrawalCount(
	ctx context.Context,
	app appContracts,
	startBlock uint64,
) (*big.Int, error) {
	// No withdrawal can happen before the accounts drive is proved: withdraw()
	// validates against the proved root and reverts while it is missing.
	if startBlock == app.application.AccountsDriveProvedBlock {
		return big.NewInt(0), nil
	}

	count, err := r.repository.GetNumberOfWithdrawals(ctx, app.application.ID)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetUint64(count), nil
}

// withdrawalsAtBlock fetches all Withdrawal events emitted at the given block
// via the IApplication adapter. Multiple Withdrawal events can fire in the
// same block (different account indices); each gets its own row with a
// distinct (application_id, account_index) primary key when persisted.
func (r *Service) withdrawalsAtBlock(
	ctx context.Context,
	app appContracts,
	block uint64,
) ([]*Withdrawal, error) {
	events, err := app.applicationContract.RetrieveWithdrawalEvents(&bind.FilterOpts{
		Context: ctx,
		Start:   block,
		End:     &block,
	})
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		r.Logger.Warn(
			"Withdrawal counter transition reported but no Withdrawal log in block",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"block", block,
		)
		return nil, fmt.Errorf("withdrawal counter transition at block %d has no Withdrawal event", block)
	}

	withdrawals := make([]*Withdrawal, 0, len(events))
	for _, ev := range events {
		withdrawals = append(withdrawals, &Withdrawal{
			ApplicationID:   app.application.ID,
			AccountIndex:    ev.AccountIndex,
			Account:         append([]byte{}, ev.Account...),
			Output:          append([]byte{}, ev.Output...),
			BlockNumber:     ev.Raw.BlockNumber,
			TransactionHash: ev.Raw.TxHash,
			LogIndex:        ev.Raw.Index,
		})
	}
	return withdrawals, nil
}
