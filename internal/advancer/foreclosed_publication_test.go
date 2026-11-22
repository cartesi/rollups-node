// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
)

// The mock manager supplies the runtime; manager tests cover its retention.
func (s *AdvancerSuite) TestForeclosedClosedEpochPublishesWithoutNewInput() {
	env := s.setupOneApp()
	app := env.app.Application
	app.Status, app.ForecloseBlock, app.LastInputCheckBlock = model.ApplicationStatus_OK, 80, 80
	app.ProcessedInputs, env.app.processedInputs = 2, 2
	epoch := &model.Epoch{ApplicationID: app.ID, Index: 5, FirstBlock: 50, LastBlock: 59,
		InputIndexLowerBound: 1, InputIndexUpperBound: 2, Status: model.EpochStatus_Closed}
	env.repo.GetEpochsReturn = map[common.Address][]*model.Epoch{app.IApplicationAddress: {epoch}}
	// There are no new inputs. If manager preserves the runtime, a subsequent
	// Step can finish IP even after the first publication write was unavailable.
	failure := errors.New("publication write unavailable")
	env.repo.UpdateEpochsError = failure
	_, err := env.service.Step(context.Background())
	s.Require().ErrorIs(err, failure)
	s.Equal(1, env.repo.EpochInputsProcessedCount)
	env.repo.UpdateEpochsError = nil
	_, err = env.service.Step(context.Background())
	s.Require().NoError(err)
	s.Equal(2, env.repo.EpochInputsProcessedCount)
	s.Equal(env.app.StateProofReturn, env.repo.PublishedStateProof)
	s.Empty(env.repo.StoredResults, "do not replay or advance another input to publish CLOSED")
	s.Zero(env.repo.ApplicationStatusUpdates)
}

func (s *AdvancerSuite) TestForeclosedFinalizationFailurePreservesClosedEpoch() {
	for _, snapshotFailure := range []bool{false, true} {
		name := "state proof failure"
		if snapshotFailure {
			name = "epoch snapshot failure"
		}
		s.Run(name, func() {
			env := s.setupOneApp()
			app := env.app.Application
			app.ConsensusType = model.Consensus_PRT
			app.Name, app.Status = "resident-before-foreclosure", model.ApplicationStatus_OK
			// The manager keeps the application from runtime creation. A later
			// repository observation returns a separate foreclosed application.
			repositoryApp := *app
			repositoryApp.ForecloseBlock = 80
			s.NotSame(app, &repositoryApp)
			s.False(app.IsForeclosed())
			s.True(repositoryApp.IsForeclosed())
			app.LastInputCheckBlock, app.LastEpochCheckBlock = 80, 80
			app.ProcessedInputs, env.app.processedInputs = 2, 2
			root := common.HexToAddress("0x57")
			epoch := &model.Epoch{ApplicationID: app.ID, Index: 5, FirstBlock: 80, LastBlock: 80,
				InputIndexLowerBound: 1, InputIndexUpperBound: 2, Status: model.EpochStatus_Closed,
				TournamentAddress: &root}
			env.repo.GetEpochsReturn = map[common.Address][]*model.Epoch{app.IApplicationAddress: {epoch}}
			instance := env.mm.Map[app.ID]
			if snapshotFailure {
				app.ExecutionParameters.SnapshotPolicy = model.SnapshotPolicy_EveryEpoch
				env.service.snapshotsDir = s.T().TempDir()
				last := repotest.NewInputBuilder().WithIndex(1).WithEpochIndex(epoch.Index).
					WithStatus(model.InputCompletionStatus_Accepted).Build()
				last.EpochApplicationID = app.ID
				env.repo.GetLastProcessedInputReturn, env.repo.GetLastInputReturn = last, last
				env.repo.GetEpochReturn = epoch
				instance.createSnapshotError = errors.New("snapshot child process failed")
				instance.destroyAfterSnapshotError = true
			} else {
				env.app.StateProofError = manager.ErrMachineClosed
			}

			_, err := env.service.Step(s.T().Context())

			s.Require().ErrorIs(err, manager.ErrMachineClosed)
			s.Equal(model.ApplicationStatus_Failed, app.Status)
			s.Equal(model.ApplicationStatus_Failed, env.repo.LastApplicationStatus)
			s.Equal(model.EpochStatus_Closed, epoch.Status)
			s.Zero(env.repo.EpochInputsProcessedCount, "a failed producer must not invent successful publication")
			s.Nil(env.repo.PublishedStateProof)
			s.Zero(instance.advanceCalls, "retained finalization must not execute another input")
			s.Empty(env.repo.StoredResults)
			s.False(env.repo.SnapshotURIUpdated)
			s.ErrorContains(err, "application "+app.Name+" epoch 5")
			s.Zero(app.ForecloseBlock, "the failure diagnostic must not refresh the resident application")
			s.Equal(uint64(80), repositoryApp.ForecloseBlock)

			// Ordinary service turns cannot recover FAILED or erase CLOSED.
			_, err = env.service.Step(s.T().Context())
			s.Require().NoError(err)
			s.False(env.mm.HasMachine(app.ID))
			s.Equal(model.EpochStatus_Closed, epoch.Status)
			s.Equal(1, env.repo.ApplicationStatusUpdates)
		})
	}
}

func (s *AdvancerSuite) TestForeclosedRejectedOnlyEpochGetsFreshFinalProof() {
	env := s.setupOneApp()
	app := env.app.Application
	app.ConsensusType = model.Consensus_PRT
	rejected := randomAdvanceResult(0)
	rejected.Status = model.InputCompletionStatus_Rejected
	rejected.Outputs = nil
	rejected.StateProof = model.StateProof{}
	processed, terminal, err := env.service.processInputs(s.T().Context(), app,
		[]*model.Input{newInput(app.ID, 0, 0, marshal(rejected))})
	s.Require().NoError(err)
	s.Equal(uint64(1), processed)
	s.False(terminal)
	s.Require().Len(env.repo.StoredResults, 1)
	s.Equal(model.InputCompletionStatus_Rejected, env.repo.StoredResults[0].Status)
	s.False(env.repo.StoredResults[0].IsComplete(), "a rejected input is not a normal accepted-state proof")

	app.ForecloseBlock, app.LastInputCheckBlock, app.LastEpochCheckBlock = 80, 80, 80
	app.ProcessedInputs, env.app.processedInputs = 1, 1
	epoch := &model.Epoch{ApplicationID: app.ID, Status: model.EpochStatus_Closed,
		InputIndexUpperBound: 1, LastBlock: 80, TournamentAddress: new(common.HexToAddress("0x57"))}
	s.Require().True(env.app.StateProofReturn.IsComplete())
	s.Require().NoError(env.service.finalizeEpoch(s.T().Context(), app, epoch))
	s.Equal(1, env.repo.EpochInputsProcessedCount)
	s.Same(env.app.StateProofReturn, env.repo.PublishedStateProof,
		"finalization must obtain a new complete proof from the resident state")
	s.Len(env.repo.StoredResults, 1, "finalization does not execute or rewrite a rejected input")
	s.Zero(env.repo.ApplicationStatusUpdates)
}

func (s *AdvancerSuite) TestEpochSnapshotFailureOutcomes() {
	for _, test := range []struct {
		name          string
		cancelService bool
		closedRuntime bool
		failed        bool
	}{
		{name: "nonclosed error"},
		{name: "service cancellation", cancelService: true, closedRuntime: true},
		{name: "destroyed runtime", closedRuntime: true, failed: true},
	} {
		s.Run(test.name, func() {
			env := s.setupOneApp()
			var logs bytes.Buffer
			env.service.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			env.service.snapshotsDir = s.T().TempDir()
			app := env.app.Application
			app.Status, app.ForecloseBlock = model.ApplicationStatus_OK, 80
			app.ExecutionParameters.SnapshotPolicy = model.SnapshotPolicy_EveryEpoch
			app.ProcessedInputs, env.app.processedInputs = 2, 2
			epoch := &model.Epoch{ApplicationID: app.ID, Index: 5, Status: model.EpochStatus_Closed,
				InputIndexLowerBound: 1, InputIndexUpperBound: 2}
			last := repotest.NewInputBuilder().WithIndex(1).WithEpochIndex(epoch.Index).
				WithStatus(model.InputCompletionStatus_Accepted).Build()
			last.EpochApplicationID = app.ID
			env.repo.GetLastProcessedInputReturn, env.repo.GetLastInputReturn = last, last
			env.repo.GetEpochReturn = epoch
			failure := errors.New("snapshot admission unavailable")
			instance := env.mm.Map[app.ID]
			instance.createSnapshotError = failure
			instance.destroyAfterSnapshotError = test.closedRuntime
			ctx, cancel := context.WithCancel(s.T().Context())
			defer cancel()
			if test.cancelService {
				// Cancel only when the snapshot operation is reached. The error
				// still reports a destroyed runtime, as with a child shutdown race.
				instance.createSnapshotHook = cancel
			}

			err := env.service.finalizeEpoch(ctx, app, epoch)

			s.Require().ErrorIs(err, failure)
			if test.closedRuntime {
				s.ErrorIs(err, manager.ErrMachineClosed)
			}
			s.Equal(model.EpochStatus_Closed, epoch.Status)
			s.Zero(env.repo.EpochInputsProcessedCount)
			s.Nil(env.repo.PublishedStateProof)
			s.False(env.repo.SnapshotURIUpdated)
			if test.failed {
				s.Equal(model.ApplicationStatus_Failed, app.Status)
				s.Equal(1, env.repo.ApplicationStatusRequests)
				s.Equal(1, env.repo.ApplicationStatusUpdates)
				s.Contains(logs.String(), "repair the cause before clearing FAILED")
			} else {
				s.Equal(model.ApplicationStatus_OK, app.Status)
				s.Zero(env.repo.ApplicationStatusRequests, "do not request FAILED for shutdown or a nonclosed error")
				s.Zero(env.repo.ApplicationStatusUpdates)
				s.Empty(env.mm.RecordedApplicationFailures)
				s.False(env.mm.PendingApplicationFailures)
				s.NotContains(logs.String(), "FAILED")
				s.NotContains(logs.String(), "operator repair")
				s.NotContains(logs.String(), "repair the cause")
			}
		})
	}
}
