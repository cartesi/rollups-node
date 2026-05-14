// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
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
	manager := NewMachineManager(repo, testLogger, false, 500)
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
			Status:              model.ApplicationStatus_OK,
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

		// Create manager with a mock instance factory
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app1}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(1))

		repo.AssertCalled(s.T(), "ListApplications", mock.Anything, mock.Anything, mock.Anything, false)
	})

	s.Run("AddsMachineForForeclosedAppWithUndrainedInputs", func() {
		require := s.Require()

		repo := &MockMachineRepository{}
		app := &model.Application{
			ID:                  1,
			Name:                "ForeclosedApp",
			IApplicationAddress: common.HexToAddress("0x1"),
			Enabled:             true,
			Status:              model.ApplicationStatus_OK,
			ForecloseBlock:      100,
			// Caught up (last_input_check_block >= foreclose_block), so the
			// drain query below is trustworthy and gets consulted.
			LastInputCheckBlock: 100,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		// The executable filter selects OK apps that have not foreclosed; the
		// drain filter selects OK apps that have. They are distinguished by
		// ForeclosureRecorded, since a foreclosed app keeps status OK.
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
		repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).
			Return(true, nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil)

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(app.ID))
		repo.AssertExpectations(s.T())
	})

	s.Run("KeepsMachineForForeclosedAppBeforeScanCaughtUp", func() {
		require := s.Require()

		repo := &MockMachineRepository{}
		// Foreclosed app whose historical scan has NOT yet reached
		// foreclose_block (last_input_check_block < foreclose_block). The
		// inputs/epochs table is still incomplete, so the drain query would
		// answer "nothing to drain" on an empty table. The manager must keep
		// the machine and must NOT consult HasUndrainedEpochsBeforeBlock yet.
		app := &model.Application{
			ID:                  1,
			Name:                "ForeclosedAppBootstrapping",
			IApplicationAddress: common.HexToAddress("0x1"),
			Enabled:             true,
			Status:              model.ApplicationStatus_OK,
			ForecloseBlock:      100,
			LastInputCheckBlock: 99,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && !*f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
		repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Status != nil && *f.Status == model.ApplicationStatus_OK &&
				f.ForeclosureRecorded != nil && *f.ForeclosureRecorded
		}), repository.Pagination{}, false).Return([]*model.Application{app}, uint64(1), nil).Once()
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).Return(nil, nil)

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(app.ID), "machine kept while the scan catches up")
		repo.AssertExpectations(s.T())
		repo.AssertNotCalled(s.T(), "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("RemoveDisabledMachines", func() {
		require := s.Require()

		// Create a mock repository
		repo := &MockMachineRepository{}
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		manager := NewMachineManager(repo, testLogger, false, 500)

		// Add mock machines
		app1 := &model.Application{ID: 1, Name: "App1"}
		app2 := &model.Application{ID: 2, Name: "App2"}
		app3 := &model.Application{ID: 3, Name: "App3"}

		mockMachine1 := &DummyMachineInstanceMock{application: app1}
		mockMachine2 := &DummyMachineInstanceMock{application: app2}
		mockMachine3 := &DummyMachineInstanceMock{application: app3}

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

	manager := NewMachineManager(repo, nil, false, 500)
	machine := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}

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

	manager := NewMachineManager(repo, nil, false, 500)
	machine := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}

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

	manager := NewMachineManager(repo, nil, false, 500)
	machine1 := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}
	machine2 := &DummyMachineInstanceMock{application: &model.Application{ID: 2}}

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

	// Close the manager and try to add a new machine
	err := manager.Close()
	require.NoError(err)

	machine3 := &DummyMachineInstanceMock{application: &model.Application{ID: 3}}
	added = manager.addMachine(3, machine3)
	require.False(added, "addMachine must reject additions after Close")
}

func (s *MachineManagerSuite) TestRemoveDisabledMachines() {
	require := s.Require()

	manager := NewMachineManager(nil, nil, false, 500)

	// Add machines
	app1 := &model.Application{ID: 1}
	app2 := &model.Application{ID: 2}
	app3 := &model.Application{ID: 3}

	machine1 := &DummyMachineInstanceMock{application: app1}
	machine2 := &DummyMachineInstanceMock{application: app2}
	machine3 := &DummyMachineInstanceMock{application: app3}

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

func (s *MachineManagerSuite) TestUpdateMachinesErrors() {
	s.Run("GetExecutableApplicationsError", func() {
		require := s.Require()

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return(([]*model.Application)(nil), uint64(0), errors.New("db error"))

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		manager := NewMachineManager(repo, testLogger, false, 500)

		err := manager.UpdateMachines(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "db error")
	})

	s.Run("SnapshotCreationFailureFallsBackToTemplate", func() {
		require := s.Require()

		app := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		snapshotPath := "/fake/snapshot/path"
		snapshotInput := &model.Input{
			Index:       2,
			SnapshotURI: &snapshotPath,
		}

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(snapshotInput, nil)
		// ListInputs for synchronization (no inputs to replay)
		repo.On("ListInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Input{}, uint64(0), nil)

		// The snapshot path doesn't exist, so it should fall back to template
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockInstance := &DummyMachineInstanceMock{application: app}
		factory := &MockMachineInstanceFactory{Instance: mockInstance}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		require.True(manager.HasMachine(1))
	})

	s.Run("TemplateCreationFailureSkipsApp", func() {
		require := s.Require()

		app := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		factory := &MockMachineInstanceFactory{Err: errors.New("machine creation failed")}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(factory))

		err := manager.UpdateMachines(context.Background())
		// UpdateMachines should not return an error; it logs and skips
		require.NoError(err)
		require.False(manager.HasMachine(1))
	})

	s.Run("SynchronizeFailureClosesAndSkipsApp", func() {
		require := s.Require()

		app := &model.Application{
			ID:                  1,
			Name:                "App1",
			IApplicationAddress: common.HexToAddress("0x1"),
			Status:              model.ApplicationStatus_OK,
			ProcessedInputs:     3,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    100,
				InspectMaxDeadline:    100,
				MaxConcurrentInspects: 3,
			},
		}

		repo := &MockMachineRepository{}
		repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
			Return([]*model.Application{app}, uint64(1), nil)
		repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
			Return(nil, nil)
		// ListInputs returns an error so the real Synchronize method propagates the failure.
		repo.On("ListInputs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false).
			Return(([]*model.Input)(nil), uint64(0), errors.New("db connection lost"))

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		// Use a factory that builds a real MachineInstanceImpl (with a mock runtime)
		// so that Synchronize actually runs and hits the repo.
		mockRuntime := &MockRollupsMachine{}
		runtimeFactory := &MockMachineRuntimeFactory{
			RuntimeToReturn: mockRuntime,
			ErrorToReturn:   nil,
		}
		realFactory := &realMachineInstanceFactory{runtimeFactory: runtimeFactory}
		manager := NewMachineManager(repo, testLogger, false, 500, WithInstanceFactory(realFactory))

		err := manager.UpdateMachines(context.Background())
		require.NoError(err)
		// Machine should NOT have been added due to sync failure
		require.False(manager.HasMachine(1))
	})
}

func (s *MachineManagerSuite) TestCloseAggregatesErrors() {
	require := s.Require()

	manager := NewMachineManager(nil, nil, false, 500)

	machine1 := &DummyMachineInstanceMock{application: &model.Application{ID: 1}}
	machine2 := &DummyMachineInstanceMock{
		application: &model.Application{ID: 2},
		closeError:  errors.New("close error 2"),
	}
	machine3 := &DummyMachineInstanceMock{
		application: &model.Application{ID: 3},
		closeError:  errors.New("close error 3"),
	}

	manager.addMachine(1, machine1)
	manager.addMachine(2, machine2)
	manager.addMachine(3, machine3)

	err := manager.Close()
	require.Error(err)
	require.Contains(err.Error(), "close error")
	require.Empty(manager.machines)
}

func (s *MachineManagerSuite) TestApplications() {
	require := s.Require()

	repo := &MockMachineRepository{}
	repo.On("GetLastSnapshot", mock.Anything, mock.Anything).
		Return(nil, nil)

	manager := NewMachineManager(repo, nil, false, 500)

	// Add machines
	app1 := &model.Application{ID: 1, Name: "App1"}
	app2 := &model.Application{ID: 2, Name: "App2"}

	machine1 := &DummyMachineInstanceMock{application: app1}
	machine2 := &DummyMachineInstanceMock{application: app2}

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

func (m *MockMachineRepository) HasUndrainedEpochsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	args := m.Called(ctx, appID, blockBound)
	return args.Bool(0), args.Error(1)
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

// ------------------------------------------------------------------------------------------------

// MockMachineInstanceFactory implements MachineInstanceFactory for testing.
// It returns the same instance for every call, ignoring the app/path arguments.
type MockMachineInstanceFactory struct {
	Instance MachineInstance
	Err      error
}

func (f *MockMachineInstanceFactory) NewFromTemplate(
	_ context.Context, _ *model.Application, _ *slog.Logger, _ bool,
) (MachineInstance, error) {
	return f.Instance, f.Err
}

func (f *MockMachineInstanceFactory) NewFromSnapshot(
	_ context.Context, _ *model.Application, _ *slog.Logger, _ bool,
	_ string, _ *common.Hash, _ uint64,
) (MachineInstance, error) {
	return f.Instance, f.Err
}

// realMachineInstanceFactory builds real MachineInstanceImpl values using the
// provided MachineRuntimeFactory. This lets tests exercise the real Synchronize
// path while still mocking the machine runtime. Unlike MockMachineInstanceFactory,
// snapshot path and hash are ignored — it always creates from the runtime factory.
type realMachineInstanceFactory struct {
	runtimeFactory MachineRuntimeFactory
}

func (f *realMachineInstanceFactory) NewFromTemplate(
	ctx context.Context, app *model.Application, logger *slog.Logger, checkHash bool,
) (MachineInstance, error) {
	return NewMachineInstanceWithFactory(ctx, app, 0, logger, checkHash, f.runtimeFactory)
}

func (f *realMachineInstanceFactory) NewFromSnapshot(
	ctx context.Context, app *model.Application, logger *slog.Logger, checkHash bool,
	_ string, _ *common.Hash, inputIndex uint64,
) (MachineInstance, error) {
	return NewMachineInstanceWithFactory(ctx, app, inputIndex+1, logger, checkHash, f.runtimeFactory)
}

// ------------------------------------------------------------------------------------------------

// DummyMachineInstanceMock implements the MachineInstance interface for testing
type DummyMachineInstanceMock struct {
	application    *model.Application
	closeError     error
	synchronizeErr error
}

func (m *DummyMachineInstanceMock) Application() *model.Application {
	return m.application
}

func (m *DummyMachineInstanceMock) ProcessedInputs() uint64 {
	return 0
}

func (m *DummyMachineInstanceMock) OutputsProof(ctx context.Context) (*model.OutputsProof, error) {
	return nil, nil
}

func (m *DummyMachineInstanceMock) Advance(_ context.Context, _ []byte, _ uint64, _ uint64, _ bool) (*model.AdvanceResult, error) {
	return nil, nil
}

func (m *DummyMachineInstanceMock) Inspect(_ context.Context, _ []byte) (*model.InspectResult, error) {
	return nil, nil
}

func (m *DummyMachineInstanceMock) Synchronize(_ context.Context, _ MachineRepository, _ uint64) error {
	return m.synchronizeErr
}

func (m *DummyMachineInstanceMock) CreateSnapshot(_ context.Context, _ uint64, _ string) error {
	return nil
}

func (m *DummyMachineInstanceMock) Hash(_ context.Context) ([32]byte, error) {
	return [32]byte{}, nil
}

func (m *DummyMachineInstanceMock) Close() error {
	return m.closeError
}
