// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestMachineManager(t *testing.T) {
	suite.Run(t, new(MachineManagerSuite))
}

type MachineManagerSuite struct {
	suite.Suite
}

func (s *MachineManagerSuite) TestNewMachineManager() {
	require := s.Require()
	repo := &MockMachineRepository{}
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := NewMachineManager(context.Background(), repo, testLogger, false)
	require.NotNil(manager)
	require.Empty(manager.machines)
	require.Equal(repo, manager.repository)
}

func (s *MachineManagerSuite) TestUpdateMachines() {
	s.Run("AddNewMachines", func() {
		require := s.Require()

		// Setup repository with enabled applications
		repo := &MockMachineRepository{}
		app1 := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			State:               model.ApplicationState_Enabled,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app1}, uint64(1), nil)

		// Empty inputs for synchronization
		repo.On("ListInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Input{}, uint64(0), nil)

		// Mock GetLastSnapshot to return nil (no snapshot available)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		// Create manager with a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		manager := NewMachineManager(context.Background(), repo, testLogger, false)

		// Create a mock factory for testing
		mockRuntime := &MockRollupsMachine{}
		mockFactory := &MockMachineRuntimeFactory{
			RuntimeToReturn: mockRuntime,
			ErrorToReturn:   nil,
		}

		// Replace the default factory with our mock
		originalFactory := defaultFactory
		defaultFactory = mockFactory
		defer func() { defaultFactory = originalFactory }()

		// This test should now succeed with our mock
		err := manager.UpdateMachines(context.Background())
		require.NoError(err)

		repo.AssertCalled(s.T(), "ListApplications", mock.Anything, mock.Anything, mock.Anything, false)
	})

	s.Run("RemoveDisabledMachines", func() {
		require := s.Require()

		// Create a mock repository
		repo := &MockMachineRepository{}
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		manager := NewMachineManager(context.Background(), repo, testLogger, false)

		// Add mock machines
		app1 := &model.Application{ID: 1, Name: "App1"}
		app2 := &model.Application{ID: 2, Name: "App2"}
		app3 := &model.Application{ID: 3, Name: "App3"}

		mockMachine1 := &MockMachineInstance{application: app1}
		mockMachine2 := &MockMachineInstance{application: app2}
		mockMachine3 := &MockMachineInstance{application: app3}

		manager.addMachine(1, mockMachine1)
		manager.addMachine(2, mockMachine2)
		manager.addMachine(3, mockMachine3)

		// Remove machines not in the active list
		manager.removeMachines([]*model.Application{app1, app3})

		// Verify machine2 was removed
		require.Len(manager.machines, 2)
		require.True(manager.HasMachine(1))
		require.False(manager.HasMachine(2))
		require.True(manager.HasMachine(3))
	})
}

func (s *MachineManagerSuite) TestGetMachine() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := NewMachineManager(context.Background(), repo, nil, false)
	machine := &MockMachineInstance{application: &model.Application{ID: 1}}

	// Add a machine
	manager.addMachine(1, machine)

	// Test retrieval
	retrieved, exists := manager.GetMachine(1)
	require.True(exists)
	require.Same(machine, retrieved)

	// Test non-existent machine
	_, exists = manager.GetMachine(2)
	require.False(exists)
}

func (s *MachineManagerSuite) TestHasMachine() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := NewMachineManager(context.Background(), repo, nil, false)
	machine := &MockMachineInstance{application: &model.Application{ID: 1}}

	// Add a machine
	manager.addMachine(1, machine)

	// Test has machine
	require.True(manager.HasMachine(1))

	// Test doesn't have machine
	require.False(manager.HasMachine(2))
}

func (s *MachineManagerSuite) TestAddMachine() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := NewMachineManager(context.Background(), repo, nil, false)
	machine1 := &MockMachineInstance{application: &model.Application{ID: 1}}
	machine2 := &MockMachineInstance{application: &model.Application{ID: 2}}

	// Add first machine
	added := manager.addMachine(1, machine1)
	require.True(added)
	require.Len(manager.machines, 1)

	// Add second machine
	added = manager.addMachine(2, machine2)
	require.True(added)
	require.Len(manager.machines, 2)

	// Try to add duplicate
	added = manager.addMachine(1, machine1)
	require.False(added)
	require.Len(manager.machines, 2)
}

func (s *MachineManagerSuite) TestRemoveDisabledMachines() {
	require := s.Require()

	manager := NewMachineManager(context.Background(), nil, nil, false)

	// Add machines
	app1 := &model.Application{ID: 1}
	app2 := &model.Application{ID: 2}
	app3 := &model.Application{ID: 3}

	machine1 := &MockMachineInstance{application: app1}
	machine2 := &MockMachineInstance{application: app2}
	machine3 := &MockMachineInstance{application: app3}

	manager.addMachine(1, machine1)
	manager.addMachine(2, machine2)
	manager.addMachine(3, machine3)

	// Remove machines not in the active list
	manager.removeMachines([]*model.Application{app1, app3})

	// Verify machine2 was removed
	require.Len(manager.machines, 2)
	require.True(manager.HasMachine(1))
	require.False(manager.HasMachine(2))
	require.True(manager.HasMachine(3))
}

func (s *MachineManagerSuite) TestApplications() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := NewMachineManager(context.Background(), repo, nil, false)

	// Add machines
	app1 := &model.Application{ID: 1, Name: "App1"}
	app2 := &model.Application{ID: 2, Name: "App2"}

	machine1 := &MockMachineInstance{application: app1}
	machine2 := &MockMachineInstance{application: app2}

	manager.addMachine(1, machine1)
	manager.addMachine(2, machine2)

	// Get applications
	apps := manager.Applications()
	require.Len(apps, 2)

	// Verify apps are in the list (order not guaranteed)
	appMap := make(map[int64]*model.Application)
	for _, app := range apps {
		appMap[app.ID] = app
	}

	require.Contains(appMap, int64(1))
	require.Contains(appMap, int64(2))
	require.Equal("App1", appMap[1].Name)
	require.Equal("App2", appMap[2].Name)
}

// Mock repository for testing
type MockMachineRepository struct {
	mock.Mock
}

func (m *MockMachineRepository) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, p, descending)
	return args.Get(0).([]*model.Application), args.Get(1).(uint64), args.Error(2)
}

func (m *MockMachineRepository) ListInputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.InputFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Input, uint64, error) {
	args := m.Called(ctx, nameOrAddress, f, p, descending)
	return args.Get(0).([]*model.Input), args.Get(1).(uint64), args.Error(2)
}

func (m *MockMachineRepository) GetLastSnapshot(
	ctx context.Context,
	nameOrAddress string) (*model.Input, error) {
	args := m.Called(ctx, nameOrAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Input), args.Error(1)
}
