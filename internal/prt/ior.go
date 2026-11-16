// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type iorStageKey struct {
	applicationID int64
	epochIndex    uint64
}

type iorStageIdentity struct {
	tournament common.Address
	block      uint64
}

// The observer owns this emission marker. It is reconstructed after restart.
func (s *Service) reportStagedIOR(
	app *model.Application, epoch *model.Epoch, snapshot daveConsensusSnapshot, observed txsender.BlockRef,
) {
	if !epoch.HasInvalidOutputsRoot() || epoch.StagedAtBlock == nil || epoch.TournamentAddress == nil {
		return
	}
	key := iorStageKey{app.ID, epoch.Index}
	identity := iorStageIdentity{*epoch.TournamentAddress, *epoch.StagedAtBlock}
	if previous, ok := s.iorStages[key]; ok && previous == identity {
		return
	}
	s.Logger.Error("Observed staged result with invalid outputs root", "condition", "ior_result_staged",
		"application", app.Name, "address", app.IApplicationAddress, "epoch_index", epoch.Index,
		"declared_outputs_root", epoch.TxBufferDataBlock, "calculated_outputs_root", epoch.CalculatedOutputsRoot,
		"root_tournament", *epoch.TournamentAddress, "staged_at_block", *epoch.StagedAtBlock,
		"staging_period", app.ClaimStagingPeriod, "sentries_agree", snapshot.accept.DoAllSentriesAgreeWithStagedTournamentResult,
		"staging_period_over", snapshot.accept.IsClaimStagingPeriodOver, "foreclose_block", app.ForecloseBlock,
		"observation_source", "consensus_snapshot",
		"observation_policy", s.defaultBlock, "observation_block", observed.Number, "observation_hash", observed.Hash)
	if s.iorStages == nil {
		s.iorStages = make(map[iorStageKey]iorStageIdentity)
	}
	s.iorStages[key] = identity
}

func (s *Service) reportAcceptedIOR(app *model.Application, epoch *model.Epoch, event *types.Log) {
	delete(s.iorStages, iorStageKey{app.ID, epoch.Index})
	if winner, ok := s.iorWinners[app.ID]; ok && winner.epochIndex == epoch.Index {
		delete(s.iorWinners, app.ID)
	}
	if !epoch.HasInvalidOutputsRoot() {
		return
	}
	s.Logger.Error("Observed acceptance of an invalid outputs-root result", "condition", "ior_result_accepted",
		"application", app.Name, "address", app.IApplicationAddress, "epoch_index", epoch.Index,
		"declared_outputs_root", epoch.TxBufferDataBlock, "calculated_outputs_root", epoch.CalculatedOutputsRoot,
		"foreclose_block", app.ForecloseBlock, "observation_source", "epoch_sealed_event", "observation_policy", s.defaultBlock,
		"event_block", event.BlockNumber, "event_block_hash", event.BlockHash, "event_transaction", event.TxHash)
}

// Only the existing observer of the current sealed epoch owns this marker.
type iorWinnerIdentity struct {
	epochIndex uint64
	tournament common.Address
	winner     common.Hash
}

// Call only after the normal result/proof and consensus compatibility checks.
func (s *Service) reportUnstagedIORWinner(
	app *model.Application, epoch *model.Epoch, snapshot daveConsensusSnapshot, observed txsender.BlockRef,
) {
	if !app.Enabled || app.IsForeclosed() || !epoch.HasInvalidOutputsRoot() || !snapshot.stage.IsFinished ||
		snapshot.stage.IsTournamentFailed || snapshot.sealed.IsTournamentResultStaged ||
		snapshot.stage.WinnerCommitment == (common.Hash{}) {
		delete(s.iorWinners, app.ID)
		return
	}
	identity := iorWinnerIdentity{epoch.Index, snapshot.sealed.Tournament, snapshot.stage.WinnerCommitment}
	if previous, ok := s.iorWinners[app.ID]; ok && previous == identity {
		return
	}
	s.Logger.Error("Current epoch has a compatible winner with invalid outputs root; local settlement is withheld. "+
		"If no other actor settles, the next sealed epoch cannot start. Ask the guardian to review foreclosure.",
		"condition", "ior_winner_settlement_withheld", "application", app.Name, "address", app.IApplicationAddress,
		"epoch_index", epoch.Index, "root_tournament", snapshot.sealed.Tournament,
		"winner_commitment", snapshot.stage.WinnerCommitment,
		"declared_outputs_root", epoch.TxBufferDataBlock, "calculated_outputs_root", epoch.CalculatedOutputsRoot,
		"foreclose_block", app.ForecloseBlock, "observation_source", "consensus_snapshot",
		"observation_policy", s.defaultBlock, "observation_block", observed.Number, "observation_hash", observed.Hash)
	if s.iorWinners == nil {
		s.iorWinners = make(map[int64]iorWinnerIdentity)
	}
	s.iorWinners[app.ID] = identity
}
