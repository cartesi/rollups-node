// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTemplateFailurePersistsWithRetryAndSibling(t *testing.T) {
	app := &model.Application{
		ID: 1, Name: "failed-template", IApplicationAddress: common.HexToAddress("0x123"),
		Enabled: true, Status: model.ApplicationStatus_OK, ConsensusType: model.Consensus_PRT,
	}
	sibling := &model.Application{
		ID: 2, Name: "healthy-template", IApplicationAddress: common.HexToAddress("0x456"),
		Enabled: true, Status: model.ApplicationStatus_OK, ConsensusType: model.Consensus_Authority,
	}
	repo := &MockMachineRepository{}
	repo.On("ListApplications", mock.Anything, repository.ExecutableApplicationsFilter(), repository.Pagination{}, false).
		Return([]*model.Application{app, sibling}, uint64(2), nil).Twice()
	repo.On("ListApplications", mock.Anything, foreclosedMachineDrainFilter(), repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), nil).Twice()
	for _, current := range []*model.Application{app, sibling} {
		repo.On("GetLastSnapshot", mock.Anything, current.IApplicationAddress.Hex()).Return(nil, nil).Once()
	}
	root := common.HexToAddress("0x789")
	epoch := &model.Epoch{
		ApplicationID: app.ID, Index: 3, LastBlock: 100,
		TournamentAddress: &root, Status: model.EpochStatus_Closed,
	}
	repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), repository.EpochFilter{
		HasTournament: new(true), Status: model.NonTerminalEpochStatuses(),
	}, repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), root.Hex()).Return(&model.Tournament{
		ApplicationID: app.ID, EpochIndex: epoch.Index, Address: root, StartInstant: 100, Allowance: 50,
	}, nil).Once()

	factoryErr := errors.New("template configuration is incompatible")
	writeErr, readErr := errors.New("status write unavailable"), errors.New("status read unavailable")
	partial := &DummyMachineInstanceMock{application: app, closeError: errors.New("partial close failed")}
	healthy := &DummyMachineInstanceMock{application: sibling}
	factory := &MockMachineInstanceFactory{TemplateFn: func(current *model.Application) (MachineInstance, error) {
		if current.ID == app.ID {
			return partial, factoryErr
		}
		return healthy, nil
	}}
	var output bytes.Buffer
	manager := newTestMachineManager(repo, captureLogger(&output), false, 10, WithInstanceFactory(factory))
	t.Cleanup(manager.Close)
	wantReason := "failed to create machine instance: template configuration is incompatible"
	statusWrite := func() *mock.Call {
		return repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
			mock.MatchedBy(func(reason *string) bool { return reason != nil && *reason == wantReason })).
			Run(func(mock.Arguments) {
				require.Equal(t, 1, partial.closeCalls)
				require.True(t, manager.HasPendingApplicationFailures())
			})
	}
	statusWrite().Return(writeErr).Once()
	repo.On("GetApplication", mock.Anything, app.IApplicationAddress.Hex()).Return(nil, readErr).Once()
	statusWrite().Return(nil).Once()

	err := manager.UpdateMachines(t.Context())
	require.ErrorIs(t, err, ErrApplicationFailureNotDurable)
	require.ErrorIs(t, err, writeErr)
	require.ErrorIs(t, err, readErr)
	require.True(t, manager.HasPendingApplicationFailures())
	require.False(t, manager.HasMachine(app.ID))
	require.True(t, manager.HasMachine(sibling.ID))
	require.Equal(t, 1, partial.closeCalls)
	require.Zero(t, partial.replayCalls)
	require.Equal(t, 1, healthy.replayCalls)
	require.Equal(t, 2, factory.TemplateCalls)
	require.Contains(t, output.String(), "level=ERROR")
	require.Contains(t, output.String(), "application=failed-template")
	require.Contains(t, output.String(), app.IApplicationAddress.Hex())
	require.Contains(t, output.String(), factoryErr.Error())
	require.Contains(t, output.String(), "join_close_block=150")
	require.Contains(t, output.String(), "epoch_index=3")

	require.NoError(t, manager.UpdateMachines(t.Context()))
	require.False(t, manager.HasPendingApplicationFailures())
	require.Equal(t, model.ApplicationStatus_Failed, app.Status)
	require.NotNil(t, app.Reason)
	require.Equal(t, wantReason, *app.Reason)
	require.False(t, manager.HasMachine(app.ID))
	require.True(t, manager.HasMachine(sibling.ID))
	require.Equal(t, 2, factory.TemplateCalls)
	require.Equal(t, 1, partial.closeCalls)
	repo.AssertExpectations(t)
}

func TestTemplateFailureCancellation(t *testing.T) {
	dependencyErr := errors.New("template read failed")
	for _, tc := range []struct {
		name     string
		shutdown bool
		cause    error
		quiet    bool
	}{
		{"active cancellation", false, context.Canceled, false},
		{"shutdown cancellation", true, fmt.Errorf("load: %w", context.Canceled), true},
		{"shutdown joined cancellations", true, errors.Join(context.Canceled, context.Canceled), true},
		{"shutdown independent failure", true, errors.Join(context.Canceled, dependencyErr), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			app := &model.Application{
				ID: 5, Name: "cancellation-template", IApplicationAddress: common.HexToAddress("0x555"),
				Enabled: true, Status: model.ApplicationStatus_OK, ConsensusType: model.Consensus_Authority,
			}
			repo := &MockMachineRepository{}
			repo.On("ListApplications", mock.Anything, repository.ExecutableApplicationsFilter(), repository.Pagination{}, false).
				Return([]*model.Application{app}, uint64(1), nil).Once()
			repo.On("ListApplications", mock.Anything, foreclosedMachineDrainFilter(), repository.Pagination{}, false).
				Return([]*model.Application{}, uint64(0), nil).Once()
			repo.On("GetLastSnapshot", mock.Anything, app.IApplicationAddress.Hex()).Return(nil, nil).Once()
			partial := &DummyMachineInstanceMock{application: app}
			factory := &MockMachineInstanceFactory{TemplateFn: func(*model.Application) (MachineInstance, error) {
				if tc.shutdown {
					cancel()
				}
				return partial, tc.cause
			}}
			if !tc.quiet {
				write := repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
					mock.MatchedBy(func(reason *string) bool {
						return reason != nil && *reason == "failed to create machine instance: "+tc.cause.Error()
					}))
				if tc.shutdown {
					write.Return(context.Canceled).Once()
					repo.On("GetApplication", mock.Anything, app.IApplicationAddress.Hex()).Return(nil, context.Canceled).Once()
				} else {
					write.Return(nil).Once()
				}
			}
			var output bytes.Buffer
			manager := newTestMachineManager(repo, captureLogger(&output), false, 10, WithInstanceFactory(factory))
			t.Cleanup(manager.Close)
			err := manager.UpdateMachines(ctx)
			if tc.shutdown {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
				require.Equal(t, model.ApplicationStatus_Failed, app.Status)
			}
			require.Equal(t, 1, partial.closeCalls)
			require.Zero(t, partial.replayCalls)
			require.Equal(t, 1, factory.TemplateCalls)
			require.False(t, manager.HasMachine(app.ID))
			if tc.quiet {
				require.NotContains(t, output.String(), "level=ERROR")
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				require.False(t, manager.HasPendingApplicationFailures())
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.Contains(t, output.String(), "level=ERROR")
				require.Contains(t, output.String(), context.Canceled.Error())
				if errors.Is(tc.cause, dependencyErr) {
					require.Contains(t, output.String(), dependencyErr.Error())
				}
				if tc.shutdown {
					require.True(t, manager.HasPendingApplicationFailures())
					require.ErrorIs(t, err, ErrApplicationFailureNotDurable)
				}
			}
			repo.AssertExpectations(t)
		})
	}
}
