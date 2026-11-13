// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"sort"
)

// scanApplicationInputs shares a rotating application order across consensus
// types. The next tick starts after the last attempted application, so a busy
// or failing application cannot repeatedly take the entire budget first.
func (r *Service) scanApplicationInputs(ctx context.Context, plan blockScanPlan, blockNumber uint64) bool {
	targets := make([]appContracts, 0, len(plan.daveEpochTargets)+len(plan.iConsensusInputTargets))
	targets = append(targets, plan.daveEpochTargets...)
	targets = append(targets, plan.iConsensusInputTargets...)
	if len(targets) == 0 {
		return true
	}
	// IDs provide a stable order even when applications are renamed, added,
	// removed, or returned in a different order by the repository.
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].application.ID < targets[j].application.ID
	})
	start := sort.Search(len(targets), func(i int) bool {
		return targets[i].application.ID > r.lastInputApplicationID
	})
	if start == len(targets) {
		start = 0
	}

	success := true
	for offset := range len(targets) {
		if ctx.Err() != nil {
			return false
		}
		if r.inputBudgetExhausted() {
			break
		}
		app := targets[(start+offset)%len(targets)]
		if app.application.IsDaveConsensus() {
			success = r.scanDaveConsensusEpochsAndInputs(ctx, []appContracts{app}, blockNumber) && success
		} else {
			// Dispatch one application at a time so InputBox/cursor grouping
			// cannot reorder applications ahead of the rotating schedule.
			success = r.scanIConsensusInputs(ctx, []appContracts{app}, blockNumber) && success
		}
		r.lastInputApplicationID = app.application.ID
	}
	return success
}
