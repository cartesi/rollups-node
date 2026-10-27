// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/replay"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFactoriesSelectPrtMachineIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		consensus model.Consensus
		want      bool
	}{
		{"authority", model.Consensus_Authority, false},
		{"quorum", model.Consensus_Quorum, false},
		{"prt", model.Consensus_PRT, true},
	} {
		for _, snapshot := range []bool{false, true} {
			kind := "template"
			if snapshot {
				kind = "snapshot"
			}
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				expectedHash := common.HexToHash("0x123")
				app := &model.Application{
					ConsensusType: tc.consensus, TemplateURI: "identity-template", TemplateHash: expectedHash,
					ExecutionParameters: model.ExecutionParameters{AdvanceMaxDeadline: time.Second},
				}
				runtime := &MockRollupsMachine{HashReturn: machine.Hash(expectedHash)}
				calls := 0
				loader := func(_ context.Context, _ *slog.Logger, cfg *machine.MachineConfig) (machine.Machine, error) {
					calls++
					require.Equal(t, tc.want, cfg.RequirePrtMachineIdentity)
					require.Equal(t, app.ExecutionParameters, cfg.ExecutionParameters)
					if snapshot {
						require.Equal(t, "identity-snapshot", cfg.Path)
					} else {
						require.Equal(t, app.TemplateURI, cfg.Path)
					}
					return runtime, nil
				}
				var factory MachineRuntimeFactory = &DefaultMachineRuntimeFactory{CheckTemplateHash: true, loader: loader}
				if snapshot {
					factory = &SnapshotMachineRuntimeFactory{
						SnapshotPath: "identity-snapshot", ExpectedHash: expectedHash, loader: loader,
					}
				}
				loaded, err := factory.CreateMachineRuntime(t.Context(), app, slog.New(slog.NewTextHandler(io.Discard, nil)))
				require.NoError(t, err)
				require.Same(t, runtime, loaded)
				require.Equal(t, 1, calls)
				require.EqualValues(t, 1, runtime.HashCalls.Load())
				require.NoError(t, loaded.Close())
			})
		}
	}
}

type identityWiringInstanceFactory struct {
	loader machineLoader
}

func (f *identityWiringInstanceFactory) NewFromTemplate(
	ctx context.Context, app *model.Application, logger *slog.Logger, check bool,
) (MachineInstance, error) {
	return NewMachineInstanceWithFactory(ctx, app, 0, logger,
		&DefaultMachineRuntimeFactory{CheckTemplateHash: check, loader: f.loader})
}

func (f *identityWiringInstanceFactory) NewFromSnapshot(
	ctx context.Context, app *model.Application, logger *slog.Logger,
	path string, expected common.Hash, index uint64,
) (MachineInstance, error) {
	return newMachineInstanceFromSnapshot(ctx, app, logger, path, expected, index, f.loader)
}

func TestPrtSnapshotIdentityFailureRetainsTemplateFallback(t *testing.T) {
	app := &model.Application{
		ID: 1, Name: "identity-fallback", ConsensusType: model.Consensus_PRT,
		Enabled: true, Status: model.ApplicationStatus_OK, ProcessedInputs: 1,
		TemplateURI: "identity-template", TemplateHash: common.Hash(newHash(1)),
		ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline: time.Second, InspectMaxDeadline: time.Second, MaxConcurrentInspects: 1,
		},
	}
	path := t.TempDir()
	snapshotHash := common.HexToHash("0x321")
	repo := &MockMachineRepository{}
	repo.On("ListApplications", mock.Anything, repository.ExecutableApplicationsFilter(), repository.Pagination{}, false).
		Return([]*model.Application{app}, uint64(1), nil).Once()
	repo.On("ListApplications", mock.Anything, foreclosedMachineDrainFilter(), repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), nil).Once()
	repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.String()).Return(&model.Input{
		Index: 0, SnapshotURI: &path, MachineHash: &snapshotHash,
	}, nil).Once()
	fork := newForkableMock()
	fork.AdvanceRemainingReturn = machine.InputEntryCapacity
	runtime := &MockRollupsMachine{HashReturn: newHash(1), ForkReturn: fork}
	loadedPaths := []string{}
	factory := &identityWiringInstanceFactory{loader: func(
		_ context.Context, _ *slog.Logger, cfg *machine.MachineConfig,
	) (machine.Machine, error) {
		require.True(t, cfg.RequirePrtMachineIdentity)
		loadedPaths = append(loadedPaths, cfg.Path)
		if cfg.Path == path {
			return nil, fmt.Errorf("snapshot identity mismatch: %w", model.ErrIncompatibleStateTransition)
		}
		require.Equal(t, app.TemplateURI, cfg.Path)
		return runtime, nil
	}}
	replayCalls := 0
	manager := NewMachineManager(repo, slog.New(slog.NewTextHandler(io.Discard, nil)), true, 1,
		WithInstanceFactory(factory), withReplayRun(func(
			ctx context.Context, _ repository.ReplayRepository, executor replay.Executor, opts replay.Options,
		) (replay.Result, error) {
			replayCalls++
			require.Equal(t, repository.ReplayVerificationCanonical, opts.Verification)
			require.Zero(t, opts.FromInput)
			require.Equal(t, app.ProcessedInputs, opts.ToInputExclusive)
			result, err := executor.Advance(ctx, []byte("identity replay input"), 0, 0, true)
			require.NoError(t, err)
			require.Equal(t, model.InputCompletionStatus_Accepted, result.Status)
			require.Equal(t, app.ProcessedInputs, executor.ProcessedInputs())
			return replay.Result{ReplayedInputs: 1}, nil
		}))
	t.Cleanup(manager.Close)
	require.NoError(t, manager.UpdateMachines(t.Context()))
	require.Equal(t, []string{path, app.TemplateURI}, loadedPaths)
	require.Equal(t, 1, replayCalls)
	require.True(t, manager.HasMachine(app.ID))
	require.False(t, manager.HasPendingApplicationFailures())
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}
