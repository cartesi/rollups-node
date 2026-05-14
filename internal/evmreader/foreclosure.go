// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// checkForForeclosure runs once per evmreader tick. For each app with a
// zero foreclose_block (the unset sentinel), it polls isForeclosed()
// (cheap, one CallContract).
// On a true result, it filters Foreclosure events over the window
// `[max(deploymentBlock, last_foreclose_check_block+1), mostRecentBlockNumber]`
// and persists (block, txHash) of the first match to the application row.
//
// If isForeclosed() is false, no event scan is needed and the cursor advances
// to mostRecentBlockNumber because the state query proves the app was not
// foreclosed up to that block. If isForeclosed() is true but no matching event
// is found, the scan cursor is left unchanged so the next tick retries the
// same window; advancing would permanently exclude the only block range where
// the event can be found.
//
// Once foreclose_block is non-zero, the app is skipped on subsequent ticks —
// the flag is one-way (the contract has no un-foreclose).
func (r *Service) checkForForeclosure(
	ctx context.Context,
	apps []appContracts,
	mostRecentBlockNumber uint64,
) {
	for _, app := range apps {
		if app.application.ForecloseBlock != 0 {
			continue
		}
		var deploymentBlock uint64
		if app.application.LastForecloseCheckBlock == 0 {
			var err error
			deploymentBlock, err = r.foreclosureSearchFloor(ctx, &app, mostRecentBlockNumber)
			if err != nil {
				if abortForeclosureLoop(r, err, "getDeploymentBlock") {
					return
				}
				if errors.Is(err, errContractNotDeployedAtBlock) {
					r.Logger.Debug("Skipping foreclosure check before application deployment",
						"application", app.application.Name,
						"address", app.application.IApplicationAddress,
						"block", mostRecentBlockNumber,
					)
					continue
				}
				r.Logger.Error("Failed to compute Foreclosure search start block",
					"application", app.application.Name,
					"address", app.application.IApplicationAddress,
					"error", err)
				continue
			}
			if deploymentBlock > mostRecentBlockNumber {
				r.Logger.Debug("Skipping foreclosure check before application deployment block",
					"application", app.application.Name,
					"address", app.application.IApplicationAddress,
					"deployment_block", deploymentBlock,
					"most_recent_block", mostRecentBlockNumber,
				)
				continue
			}
		}
		if mostRecentBlockNumber <= app.application.LastForecloseCheckBlock {
			r.Logger.Debug("Not checking foreclosure: already checked up to the most recent block",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"last_foreclose_check_block", app.application.LastForecloseCheckBlock,
				"most_recent_block", mostRecentBlockNumber,
			)
			continue
		}
		foreclosed, err := app.applicationContract.IsForeclosed(
			&bind.CallOpts{
				Context:     ctx,
				BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
			},
		)
		if err != nil {
			if abortForeclosureLoop(r, err, "isForeclosed") {
				return
			}
			r.Logger.Error("Failed to query isForeclosed",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"error", err)
			continue
		}
		if !foreclosed {
			if app.application.LastForecloseCheckBlock < mostRecentBlockNumber {
				r.advanceLastForecloseCheckBlock(ctx, app.application.ID, mostRecentBlockNumber)
				app.application.LastForecloseCheckBlock = mostRecentBlockNumber
			}
			continue
		}

		// On-chain says the app is foreclosed but we don't yet know which
		// block emitted Foreclosure(). Determine the lower bound of the
		// filter window. Once LastForecloseCheckBlock is non-zero we've
		// scanned through deployment already, so we never need to read it
		// again for the lifetime of this wait state.
		startBlock := app.application.LastForecloseCheckBlock + 1
		if app.application.LastForecloseCheckBlock == 0 {
			// deploymentBlock was resolved at the top of the loop under this
			// same zero-LastForecloseCheckBlock precondition.
			startBlock = deploymentBlock
		}
		if startBlock > mostRecentBlockNumber {
			// Already scanned past the current head; nothing new since the
			// previous tick. Re-check isForeclosed next tick.
			continue
		}

		events, err := app.applicationContract.RetrieveForeclosureEvents(
			&bind.FilterOpts{
				Context: ctx,
				Start:   startBlock,
				End:     &mostRecentBlockNumber,
			},
		)
		if err != nil {
			if abortForeclosureLoop(r, err, "retrieveForeclosureEvents") {
				return
			}
			r.Logger.Error("Failed to fetch Foreclosure events",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"error", err)
			continue
		}
		if len(events) == 0 {
			r.Logger.Warn(
				"isForeclosed() is true but no Foreclosure event found in search window — will retry same window next tick",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"start_block", startBlock,
				"end_block", mostRecentBlockNumber)
			continue
		}
		// `Foreclosure()` is one-way; multiple events on the same app are
		// not possible. Use the first.
		ev := events[0]
		block := ev.Raw.BlockNumber
		txHash := ev.Raw.TxHash

		err = r.repository.UpdateApplicationForeclosure(
			ctx, app.application.ID, block, txHash, mostRecentBlockNumber,
		)
		if errors.Is(err, repository.ErrNotFound) {
			// Row was deleted between the ListApplications scan at the top
			// of the tick and now. Skip without touching the in-memory
			// marker — writing it would diverge from a row that no longer
			// exists.
			r.Logger.Warn(
				"Foreclosure observed but application row is missing; skipping",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"foreclose_block", block,
				"foreclose_transaction", txHash)
			continue
		}
		if err != nil {
			r.Logger.Error("Failed to record foreclosure",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"foreclose_block", block,
				"foreclose_transaction", txHash,
				"error", err)
			continue
		}
		// Reflect the write in the in-memory app so other code paths in
		// this tick see the marker. Safe both for "we wrote" and the
		// idempotent "already foreclosed" case: the Foreclosure() event
		// is one-way so all observers see the same (block, txHash).
		app.application.ForecloseBlock = block
		txHashCopy := txHash
		app.application.ForecloseTransaction = &txHashCopy
		app.application.LastForecloseCheckBlock = mostRecentBlockNumber
		r.Logger.Info("Application foreclosure observed",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"foreclose_block", block,
			"foreclose_transaction", txHash)
	}
}

// advanceLastForecloseCheckBlock persists the new value and logs (does not
// surface) any DB error. A failed write is non-fatal: the next tick will
// re-scan the same window, paying the cost but producing correct behavior.
func (r *Service) advanceLastForecloseCheckBlock(ctx context.Context, appID int64, head uint64) {
	if err := r.repository.UpdateApplicationLastForecloseCheckBlock(ctx, appID, head); err != nil {
		r.Logger.Warn("Failed to advance last_foreclose_check_block",
			"application_id", appID,
			"head", head,
			"error", err)
	}
}

// abortForeclosureLoop reports whether the RPC error should abort the
// per-tick loop entirely. Context cancellation is graceful shutdown —
// silent return. A deadline-exceeded error means the tick's budget is
// gone; every remaining app's RPC call would fail with the same error,
// producing one ERROR log per app for no operational benefit. Log once
// at the site and abort. Other errors stay per-app so a transient RPC
// failure on one app does not block the rest. Mirrors the convention
// documented at memory/feedback_context_error_semantics.md.
func abortForeclosureLoop(r *Service, err error, where string) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		r.Logger.Error("Foreclosure scan deadline exceeded; aborting remaining apps",
			"site", where, "error", err)
		return true
	}
	return false
}

// foreclosureSearchFloor reads the application's on-chain deployment block.
// Called only on the first tick of the wait state (LastForecloseCheckBlock
// == 0); once it advances past zero, the deployment block is irrelevant to
// subsequent ticks.
func (r *Service) foreclosureSearchFloor(
	ctx context.Context,
	app *appContracts,
	mostRecentBlockNumber uint64,
) (uint64, error) {
	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	}
	deploymentBlock, err := app.applicationContract.GetDeploymentBlockNumber(callOpts)
	if err != nil {
		if errors.Is(err, bind.ErrNoCode) {
			return 0, fmt.Errorf("%w: application %s at block %d: %w",
				errContractNotDeployedAtBlock,
				app.application.IApplicationAddress,
				mostRecentBlockNumber,
				err)
		}
		return 0, fmt.Errorf("get deployment block: %w", err)
	}
	// Zero is accepted: anvil / genesis-snapshot fixtures can legitimately
	// place contract code at block 0, and a zero floor only widens the scan
	// window (a performance hit bounded by last_foreclose_check_block on
	// the next tick, not a correctness break). The original guard rejected
	// zero defensively but produced a hard error on otherwise-valid fixtures.
	// A negative value is impossible from a uint256 return.
	return deploymentBlock.Uint64(), nil
}
