// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func (s *AdvancerSuite) TestProtectionDeadlineConsumers() {
	const (
		advanceFailure         = "advance failure"
		executionTerminal      = "execution terminal"
		invalidOutputsLength   = "invalid outputs length"
		proofFailure           = "proof failure"
		unconfirmedPersistence = "unconfirmed persistence"
	)
	for _, scenario := range []string{
		advanceFailure, "snapshot failure", executionTerminal,
		invalidOutputsLength, unconfirmedPersistence, proofFailure,
	} {
		s.Run(scenario, func() {
			env := s.setupOneApp()
			app := env.app.Application
			app.ConsensusType = model.Consensus_PRT
			app.Status = model.ApplicationStatus_OK
			app.Enabled = true
			var output bytes.Buffer
			env.service.Logger = slog.New(slog.NewTextHandler(&output, nil))
			env.repo.Tournaments = make(map[common.Address]*model.Tournament)
			var epochs []*model.Epoch
			for index := uint64(0); index < 3; index++ {
				root := common.Address{byte(index + 1)}
				epoch := &model.Epoch{ApplicationID: app.ID, Index: index,
					LastBlock: 100 + index, Status: model.EpochStatus_Closed, TournamentAddress: &root}
				epochs = append(epochs, epoch)
				env.repo.Tournaments[root] = &model.Tournament{ApplicationID: app.ID, EpochIndex: index, Address: root,
					StartInstant: epoch.LastBlock, Allowance: 50}
			}
			env.repo.GetEpochsReturn = map[common.Address][]*model.Epoch{app.IApplicationAddress: epochs}
			result := randomAdvanceResult(0)
			result.EpochIndex = 1
			switch scenario {
			case advanceFailure:
				env.app.AdvanceError = errors.New("runtime failed")
				env.repo.UpdateApplicationStatusError = errors.New("status write failed")
			case "snapshot failure":
				app.ExecutionParameters.SnapshotPolicy = model.SnapshotPolicy_EveryInput
				env.service.snapshotsDir = s.T().TempDir()
				instance := env.mm.Map[app.ID]
				instance.createSnapshotError = errors.New("snapshot failed")
				instance.destroyAfterSnapshotError = true
			case executionTerminal:
				result.Status = model.InputCompletionStatus_MachineHalted
			case invalidOutputsLength:
				result.Status = model.InputCompletionStatus_InvalidOutputsRoot
			case unconfirmedPersistence:
				env.repo.StoreAdvanceError = errors.New("commit outcome unavailable")
			case proofFailure:
				env.app.StateProofError = errors.New("machine instance is closed")
			}
			if scenario == proofFailure {
				// A closed runtime is the automatic-stop branch of publication preparation.
				env.app.StateProofError = manager.ErrMachineClosed
				_, err := env.service.prepareEpochPublication(s.T().Context(), app, epochs[1])
				require.Error(s.T(), err)
			} else {
				_, _, err := env.service.processInputs(s.T().Context(), app, []*model.Input{newInput(app.ID, 1, 0, marshal(result))})
				if scenario == executionTerminal || scenario == invalidOutputsLength {
					require.NoError(s.T(), err)
				} else {
					require.Error(s.T(), err)
				}
			}
			logs := output.String()
			switch scenario {
			case unconfirmedPersistence:
				require.Contains(s.T(), logs, "PRT protection deadline discovery unavailable")
				require.Contains(s.T(), logs, "epoch_index=1")
				require.Contains(s.T(), logs, "clock_available=false")
				require.Empty(s.T(), env.repo.TournamentReads)
				require.Empty(s.T(), env.repo.EpochFilters)
				require.True(s.T(), env.supervisor.StopCalled.Load())
			case executionTerminal, invalidOutputsLength:
				require.Len(s.T(), env.repo.TournamentReads, 2)
				require.NotContains(s.T(), logs, "root_tournament="+epochs[0].TournamentAddress.Hex())
				require.Contains(s.T(), logs, "join_close_block=151")
				require.Contains(s.T(), logs, "join_close_block=152")
			default:
				require.Len(s.T(), env.repo.TournamentReads, 3)
				require.Equal(s.T(), 3, strings.Count(logs, "PRT protection stopped"))
				require.Contains(s.T(), logs, "join_close_block=150")
				require.Contains(s.T(), logs, "join_close_block=152")
			}
			if scenario == advanceFailure {
				require.True(s.T(), env.mm.PendingApplicationFailures)
				require.False(s.T(), env.service.Ready())
			}
		})
	}
}

func TestProtectionAlertDoesNotTurnShutdownIntoFailure(t *testing.T) {
	mm := newMockMachineManager()
	machine := newMockMachine(1)
	machine.Application.ConsensusType = model.Consensus_PRT
	mm.Map[1] = newMockInstance(machine)
	repo := &MockRepository{}
	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(t, err)
	var output bytes.Buffer
	svc.Logger = slog.New(slog.NewTextHandler(&output, nil))
	machine.AdvanceError = context.Canceled
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = svc.processInputs(ctx, machine.Application, []*model.Input{newInput(1, 0, 0, []byte("input"))})
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, output.String(), "PRT protection")
	require.Empty(t, repo.TournamentReads)
	require.Zero(t, repo.ApplicationStatusUpdates)
}

func (s *AdvancerSuite) TestFatalStopsReportAllPRTProtection() {
	for _, consensus := range model.ConsensusAllValues {
		for _, test := range []struct {
			name                 string
			statusRace, shutdown bool
		}{
			{name: "ambiguous persistence"},
			{name: "close after status race", statusRace: true},
			{name: "close during shutdown", shutdown: true},
		} {
			s.Run(consensus.String()+"/"+test.name, func() {
				env := s.setupOneApp()
				app := env.app.Application
				app.ConsensusType = consensus
				app.Name = "source application"
				var output bytes.Buffer
				env.service.Logger = slog.New(slog.NewTextHandler(&output, nil))
				ctx, cancel := context.WithCancel(s.T().Context())
				defer cancel()
				switch {
				case test.statusRace:
					env.repo.StoreAdvanceError = repository.ErrApplicationNotRunnable
					env.mm.Map[app.ID].closeError = errors.New("runtime close failed")
				case test.shutdown:
					env.repo.StoreAdvanceHook = func(context.Context) error { cancel(); return context.Canceled }
					env.mm.Map[app.ID].closeError = errors.New("runtime close failed")
				default:
					env.repo.StoreAdvanceError = errors.New("commit outcome unavailable")
				}
				_, _, err := env.service.processInputs(ctx, app, []*model.Input{newInput(app.ID, 1, 0, marshal(randomAdvanceResult(0)))})
				require.Error(s.T(), err)
				require.True(s.T(), env.supervisor.StopCalled.Load())
				require.Contains(s.T(), output.String(), "PRT global protection deadline inventory unavailable")
				require.Contains(s.T(), output.String(), "affected_applications=\"all enabled PRT applications\"")
				require.Contains(s.T(), output.String(), "clock_available=false")
				require.Empty(s.T(), env.repo.EpochFilters)
				require.Empty(s.T(), env.repo.TournamentReads)
				require.Zero(s.T(), env.repo.ApplicationStatusUpdates)
			})
		}
	}
}
