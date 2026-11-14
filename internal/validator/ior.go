// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func (s *Service) reportWithheldSettlement(app *model.Application, epoch *model.Epoch, observation ...any) {
	attrs := make([]any, 0, 18+len(observation))
	attrs = append(attrs, "condition", "ior_settlement_withheld", "application", app.Name, "address", app.IApplicationAddress,
		"epoch_index", epoch.Index, "declared_outputs_root", epoch.TxBufferDataBlock,
		"calculated_outputs_root", epoch.CalculatedOutputsRoot, "epoch_status", epoch.Status,
		"application_status", app.Status, "foreclose_block", app.ForecloseBlock)
	attrs = append(attrs, observation...)
	s.Logger.Warn("Local publication is prohibited for invalid outputs root", attrs...)
}

// Recover stored conditions once per application in this service instance.
// Failed reads remain retryable without blocking eligible computation.
func (s *Service) recoverWithheldSettlements(ctx context.Context, app *model.Application) {
	if s.recoveredIOR[app.ID] {
		return
	}
	epochs, _, err := s.repository.ListEpochs(ctx, app.IApplicationAddress.Hex(),
		repository.EpochFilter{InvalidOutputsRoot: true}, repository.Pagination{}, false)
	if err != nil {
		if ctx.Err() == nil {
			s.Logger.Warn("Unable to inspect stored outputs-root mismatches",
				"application", app.Name, "address", app.IApplicationAddress, "error", err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	for _, epoch := range epochs {
		if ctx.Err() != nil {
			return
		}
		if !epoch.HasInvalidOutputsRoot() {
			continue
		}
		if epoch.Status == model.EpochStatus_ClaimStaged && epoch.StagedAtBlock != nil {
			s.Logger.Error("Stored epoch with invalid outputs root is staged", "condition", "ior_result_staged",
				"application", app.Name, "address", app.IApplicationAddress, "epoch_index", epoch.Index,
				"declared_outputs_root", epoch.TxBufferDataBlock, "calculated_outputs_root", epoch.CalculatedOutputsRoot,
				"staged_at_block", *epoch.StagedAtBlock, "staging_period", app.ClaimStagingPeriod,
				"observation_source", "stored", "application_status", app.Status, "foreclose_block", app.ForecloseBlock)
		}
		s.reportWithheldSettlement(app, epoch, "observation_source", "stored", "join_state", "not_checked",
			"commitment", epoch.Commitment, "root_tournament", epoch.TournamentAddress)
	}
	if ctx.Err() != nil {
		return
	}
	if s.recoveredIOR == nil {
		s.recoveredIOR = make(map[int64]bool)
	}
	s.recoveredIOR[app.ID] = true
}
