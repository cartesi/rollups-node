// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import "github.com/cartesi/rollups-node/internal/model"

type iorStageKey struct {
	applicationID int64
	epochIndex    uint64
}

// Only the epoch observer owns this marker; restart reconstructs it from facts.
func (s *Service) reportStagedIOR(
	app *model.Application, epoch *model.Epoch, stagedAt uint64, source string, observedBlock uint64,
) {
	if !epoch.HasInvalidOutputsRoot() {
		return
	}
	key := iorStageKey{app.ID, epoch.Index}
	if previous, ok := s.iorStages[key]; ok && previous == stagedAt {
		return
	}
	s.Logger.Error("Observed staged result with invalid outputs root", "condition", "ior_result_staged",
		"application", app.Name, "address", app.IApplicationAddress, "epoch_index", epoch.Index,
		"declared_outputs_root", epoch.TxBufferDataBlock, "calculated_outputs_root", epoch.CalculatedOutputsRoot,
		"staged_at_block", stagedAt, "staging_period", app.ClaimStagingPeriod, "foreclose_block", app.ForecloseBlock,
		"observation_source", source, "observation_policy", s.defaultBlock, "observation_block", observedBlock)
	if s.iorStages == nil {
		s.iorStages = make(map[iorStageKey]uint64)
	}
	s.iorStages[key] = stagedAt
}

func (s *Service) reportAcceptedIOR(
	app *model.Application, epoch *model.Epoch, source string, observedBlock uint64, observation ...any,
) {
	delete(s.iorStages, iorStageKey{app.ID, epoch.Index})
	if !epoch.HasInvalidOutputsRoot() {
		return
	}
	attrs := make([]any, 0, 18+len(observation))
	attrs = append(attrs, "condition", "ior_result_accepted", "application", app.Name, "address", app.IApplicationAddress,
		"epoch_index", epoch.Index, "declared_outputs_root", epoch.TxBufferDataBlock,
		"calculated_outputs_root", epoch.CalculatedOutputsRoot, "observation_source", source,
		"observation_policy", s.defaultBlock, "observation_block", observedBlock)
	s.Logger.Error("Observed acceptance of an invalid outputs-root result", append(attrs, observation...)...)
}
