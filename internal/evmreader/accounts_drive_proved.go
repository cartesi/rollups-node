// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"math/big"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// checkForDriveProved runs once per evmreader tick for each post-foreclosure
// app whose `accounts_drive_proved_block` is still zero. It first checks the
// one-way getAccountsDriveMerkleRoot().wasProved flag at mostRecent. If the
// flag is false, the scan cursor advances because no prove event exists up to
// that block. If the flag is true, it does a FilterLogs over
// `[max(foreclose_block, last_accounts_drive_proved_check_block+1), mostRecent]`
// for `AccountsDriveMerkleRootProved` events on the IApplication contract.
//
// The contract reverts on a second `proveAccountsDriveMerkleRoot` call
// (`AccountsDriveMerkleRootAlreadyProved`), so at most one event can fire per
// app over its lifetime. On the (at most one) event in the window:
//
//  1. Persist via
//     UpdateAccountsDriveProved with the
//     event's (block, txHash, root) and the scanner cursor. Idempotent on
//     first observation.
//  2. Mirror the values onto app.application so this tick's downstream
//     dispatcher (checkPostForeclosure) sees the drive-proved marker and
//     routes the next tick to the withdrawal scan.
//
// If the on-chain flag says proved but the log scan returns no event, the
// cursor is left unchanged so a transient eth_call/eth_getLogs disagreement
// cannot skip the only prove event.
func (r *Service) checkForDriveProved(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) {
	startBlock := app.application.LastAccountsDriveProvedCheckBlock + 1
	if floor := app.application.ForecloseBlock; startBlock < floor {
		startBlock = floor
	}
	if startBlock > mostRecentBlockNumber {
		// Cursor already past head (rare; e.g. defaultBlock policy drift).
		// Nothing to scan; do not regress the cursor.
		return
	}

	proved, _, err := app.applicationContract.GetAccountsDriveMerkleRoot(&bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	})
	if err != nil {
		if abortPostForeclosureLoop(r, err, "getAccountsDriveMerkleRoot") {
			return
		}
		r.Logger.Error("Failed to query accounts drive proved state",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"block", mostRecentBlockNumber,
			"error", err)
		return
	}
	if !proved {
		r.advanceLastAccountsDriveProvedCheckBlock(ctx, app.application.ID, mostRecentBlockNumber)
		app.application.LastAccountsDriveProvedCheckBlock = mostRecentBlockNumber
		return
	}

	events, err := app.applicationContract.RetrieveAccountsDriveProvedEvents(&bind.FilterOpts{
		Context: ctx,
		Start:   startBlock,
		End:     &mostRecentBlockNumber,
	})
	if err != nil {
		if abortPostForeclosureLoop(r, err, "retrieveAccountsDriveProvedEvents") {
			return
		}
		r.Logger.Error("Failed to scan accounts-drive-proved events",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"start_block", startBlock,
			"end_block", mostRecentBlockNumber,
			"error", err)
		return
	}
	if len(events) == 0 {
		// proveAccountsDriveMerkleRoot is onlyForeclosed (Application.sol), so a
		// prove event is always emitted at a block >= foreclose_block and thus
		// always lands inside this scan window ([>= foreclose_block, mostRecent]).
		// An honest, self-consistent RPC therefore always returns it here. The
		// flag reading proved at head while no event is found in the window is a
		// transient eth_call/eth_getLogs view skew (a persistent miss would mean
		// a broken RPC, handled outside the node): leave the cursor unchanged and
		// retry next tick, which cannot skip the only prove event.
		r.Logger.Warn(
			"getAccountsDriveMerkleRoot() is proved but no AccountsDriveMerkleRootProved event found; will retry same window",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"start_block", startBlock,
			"end_block", mostRecentBlockNumber)
		return
	}

	// The contract caps lifetime emissions at one; defensively take the first
	// if more than one slipped through.
	ev := events[0]
	if err := r.persistDriveProved(ctx, app, ev, mostRecentBlockNumber); err != nil {
		return
	}
	app.application.LastAccountsDriveProvedCheckBlock = mostRecentBlockNumber
}

// persistDriveProved writes the (block, txHash, root) tuple from the on-chain
// event and the scan cursor to the application row in one repository
// transaction, then mirrors the marker onto the in-memory model. Returning an
// error tells the caller to leave the in-memory cursor unchanged so the event
// can be retried on the next tick.
func (r *Service) persistDriveProved(
	ctx context.Context,
	app appContracts,
	ev *iapplication.IApplicationAccountsDriveMerkleRootProved,
	mostRecentBlockNumber uint64,
) error {
	block := ev.Raw.BlockNumber
	txHash := ev.Raw.TxHash
	root := common.Hash(ev.AccountsDriveMerkleRoot)

	err := r.repository.UpdateAccountsDriveProved(
		ctx, app.application.ID, block, txHash, root, mostRecentBlockNumber,
	)
	if errors.Is(err, repository.ErrNotFound) {
		// Row was deleted between the ListApplications scan and now.
		// Skip the in-memory marker write — diverging from a row that no
		// longer exists is worse than missing a marker.
		r.Logger.Warn(
			"Drive-prove observed but application row is missing; skipping",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"accounts_drive_proved_block", block,
		)
		return nil
	}
	if err != nil {
		r.Logger.Error("Failed to record accounts drive proved",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"accounts_drive_proved_block", block,
			"error", err)
		return err
	}

	app.application.AccountsDriveProvedBlock = block
	txHashCopy := txHash
	rootCopy := root
	app.application.AccountsDriveProvedTransaction = &txHashCopy
	app.application.AccountsDriveMerkleRoot = &rootCopy

	r.Logger.Info("Accounts drive proved observed",
		"application", app.application.Name,
		"address", app.application.IApplicationAddress,
		"accounts_drive_proved_block", block,
		"accounts_drive_proved_transaction", txHash,
		"accounts_drive_merkle_root", root,
	)
	return nil
}

// advanceLastAccountsDriveProvedCheckBlock persists the new cursor value
// and logs (does not surface) any DB error. A failed write is non-fatal:
// the next tick will re-scan the same window, paying the cost but producing
// correct behavior.
func (r *Service) advanceLastAccountsDriveProvedCheckBlock(ctx context.Context, appID int64, head uint64) {
	if err := r.repository.UpdateApplicationLastAccountsDriveProvedCheckBlock(ctx, appID, head); err != nil {
		r.Logger.Warn("Failed to advance last_accounts_drive_proved_check_block",
			"application_id", appID,
			"head", head,
			"error", err)
	}
}
