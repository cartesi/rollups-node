// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/cartesi/rollups-node/internal/txsender"
)

func protectionLocallyReady(epoch *model.Epoch) bool {
	if epoch == nil || epoch.Commitment == nil || epoch.MachineHash == nil || epoch.TxBufferDataBlock == nil {
		return false
	}
	if epoch.Status == model.EpochStatus_ClaimStaged {
		return true
	}
	if epoch.Status != model.EpochStatus_ClaimComputed {
		return false
	}
	_, err := epoch.StateProof()
	return err == nil
}

func pendingExecutionInputs(lower, upper, processed uint64) (uint64, error) {
	if lower > upper {
		return 0, errors.New("sealed epoch input bounds are inverted")
	}
	cursor := min(upper, max(lower, processed))
	return upper - cursor, nil
}

func (s *Service) reportSealBacklog(app *model.Application, target protectionTarget,
	clock prtprotection.Clock, facts txsender.BlockRef,
) {
	if protectionLocallyReady(target.epoch) {
		return
	}
	status := "unavailable"
	if target.epoch != nil {
		status = target.epoch.Status.String()
	}
	args := []any{"application", app.Name, "address", app.IApplicationAddress, "epoch_index", target.index,
		"root_tournament", target.root, "epoch_status", status, "configured_head", facts.Number,
		"advance_max_deadline_ns", int64(app.ExecutionParameters.AdvanceMaxDeadline),
		"clock_available", clock.ClosingBlock != nil, "clock_source", clock.Source,
		"validation_pending", true, "execution_budget_scope", "execution only"}
	if clock.ClosingBlock != nil {
		args = append(args, "join_close_block", *clock.ClosingBlock, "join_window_closed", facts.Number >= *clock.ClosingBlock)
	}
	if clock.Err != nil {
		args = append(args, "clock_error", clock.Err)
	}
	pending, err := pendingExecutionInputs(target.lower, target.upper, app.ProcessedInputs)
	if err != nil {
		args = append(args, "backlog_error", err)
	} else {
		args = append(args, "pending_execution_inputs", pending)
		if deadline := app.ExecutionParameters.AdvanceMaxDeadline; deadline > 0 {
			// This exact budget excludes loading, replay, validation, scheduling,
			// RPC, signing, and inclusion. It is not an ETA or completion bound.
			budget := new(big.Int).Mul(new(big.Int).SetUint64(pending), big.NewInt(int64(deadline)))
			args = append(args, "execution_budget_ns", budget.String())
		} else {
			args = append(args, "execution_budget_ns", "unbounded")
		}
	}
	s.Logger.Warn("PRT sealed epoch is not locally ready for publication", args...)
}
