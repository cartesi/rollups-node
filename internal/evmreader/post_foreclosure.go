// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
)

// checkPostForeclosure dispatches per-tick observation work for apps that
// have already been foreclosed on chain.
//
// Apps with `foreclose_block == 0` are skipped — they are still in the
// pre-foreclosure scan surface. For each foreclosed app, dispatches to:
//   - checkForDriveProved while `accounts_drive_proved_block == 0`
//     (discover the proveAccountsDriveMerkleRoot tx by checking the one-way
//     wasProved boolean, then filtering the event when it flips).
//   - checkForPostForeclosureWithdrawals once the drive has been proved
//     (discover Withdrawal events via FindTransitions on the on-chain
//     getNumberOfWithdrawals counter, then FilterLogs on the 1-block
//     window and persist each event).
//
// The two halves are mutually exclusive — once drive-prove lands, the
// withdrawal scan takes over for that app. They are mutually exclusive
// because the contract enforces drive-must-be-proved-before-withdraw, so
// the cursor relationship is one-way.
func (r *Service) checkPostForeclosure(
	ctx context.Context,
	apps []appContracts,
	mostRecentBlockNumber uint64,
) {
	for _, app := range apps {
		if app.application.ForecloseBlock == 0 {
			continue
		}
		if app.application.AccountsDriveProvedBlock == 0 {
			r.checkForDriveProved(ctx, app, mostRecentBlockNumber)
		} else {
			r.checkForPostForeclosureWithdrawals(ctx, app, mostRecentBlockNumber)
		}
	}
}

// abortPostForeclosureLoop mirrors abortForeclosureLoop's
// context-error convention: context.Canceled is graceful (silent return),
// context.DeadlineExceeded means the tick's budget is gone and every
// remaining per-app RPC would fail the same way — log once at the site
// and stop the loop. Other errors stay per-app so a transient RPC failure
// on one app does not block the rest.
func abortPostForeclosureLoop(r *Service, err error, where string) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		r.Logger.Error("Post-foreclosure scan deadline exceeded; aborting remaining apps",
			"site", where, "error", err)
		return true
	}
	return false
}
