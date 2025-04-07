// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/suite"
)

func TestAdvancer(t *testing.T) {
	suite.Run(t, new(AdvancerSuite))
}

type AdvancerSuite struct{ suite.Suite }

func newMockAdvancerService(machineManager *MockMachineManager, repo *MockRepository) (*Service, error) {
	s := &Service{
		machineManager: machineManager,
		repository:     repo,
	}
	serviceArgs := &service.CreateInfo{Name: "advancer", Impl: s}
	err := service.Create(context.Background(), serviceArgs, &s.Service)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AdvancerSuite) TestServiceInterface() {
	s.Run("ServiceMethods", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		repository := &MockRepository{}
		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		// Test service interface methods
		require.True(advancer.Alive())
		require.True(advancer.Ready())
		require.Empty(advancer.Reload())
		require.Empty(advancer.Stop(false))
		require.Equal(advancer.Name, advancer.String())

		// Test Tick method
		machineManager.Map[1] = *newMockMachine(1)
		repository.GetInputsReturn = map[common.Address][]*Input{
			machineManager.Map[1].Application.IApplicationAddress: {},
		}
		tickErrors := advancer.Tick()
		require.Empty(tickErrors)

		// Test Tick with error
		repository.UpdateEpochsError = errors.New("update epochs error")
		tickErrors = advancer.Tick()
		require.NotEmpty(tickErrors)
		require.Contains(tickErrors[0].Error(), "update epochs error")
	})
}

func (s *AdvancerSuite) TestStep() {
	s.Run("Ok", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		app2 := newMockMachine(2)
		machineManager.Map[1] = *app1
		machineManager.Map[2] = *app2
		res1 := randomAdvanceResult(1)
		res2 := randomAdvanceResult(2)
		res3 := randomAdvanceResult(3)

		repository := &MockRepository{
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: {
					newInput(app1.Application.ID, 0, 0, marshal(res1)),
					newInput(app1.Application.ID, 0, 1, marshal(res2)),
				},
				app2.Application.IApplicationAddress: {
					newInput(app2.Application.ID, 0, 0, marshal(res3)),
				},
			},
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		err = advancer.Step(context.Background())
		require.Nil(err)

		require.Len(repository.StoredResults, 3)
	})

	s.Run("Error/UpdateEpochs", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1
		res1 := randomAdvanceResult(1)

		repository := &MockRepository{
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: {
					newInput(app1.Application.ID, 0, 0, marshal(res1)),
				},
			},
			UpdateEpochsError: errors.New("update epochs error"),
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		err = advancer.Step(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "update epochs error")
	})

	s.Run("Error/UpdateMachines", func() {
		require := s.Require()

		machineManager := &MockMachineManager{
			Map:                 map[int64]MockMachineImpl{},
			UpdateMachinesError: errors.New("update machines error"),
		}
		repository := &MockRepository{}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		err = advancer.Step(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "update machines error")
	})

	s.Run("Error/GetInputs", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1

		repository := &MockRepository{
			GetInputsError: errors.New("get inputs error"),
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		err = advancer.Step(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "get inputs error")
	})

	s.Run("NoInputs", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1

		repository := &MockRepository{
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: {},
			},
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		err = advancer.Step(context.Background())
		require.Nil(err)
		require.Len(repository.StoredResults, 0)
	})
}

func (s *AdvancerSuite) TestGetUnprocessedInputs() {
	s.Run("Success", func() {
		require := s.Require()

		app1 := newMockMachine(1)
		inputs := []*Input{
			newInput(app1.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			newInput(app1.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
		}

		repository := &MockRepository{
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: inputs,
			},
		}

		result, count, err := getUnprocessedInputs(context.Background(), repository, app1.Application.IApplicationAddress.String())
		require.Nil(err)
		require.Equal(uint64(2), count)
		require.Equal(inputs, result)
	})

	s.Run("Error", func() {
		require := s.Require()

		app1 := newMockMachine(1)
		repository := &MockRepository{
			GetInputsError: errors.New("list inputs error"),
		}

		_, _, err := getUnprocessedInputs(context.Background(), repository, app1.Application.IApplicationAddress.String())
		require.Error(err)
		require.Contains(err.Error(), "list inputs error")
	})
}

func (s *AdvancerSuite) TestProcess() {
	setup := func() (*MockMachineManager, *MockRepository, *Service, *MockMachineImpl) {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1
		repository := &MockRepository{}
		advancer, err := newMockAdvancerService(machineManager, repository)
		require.Nil(err)
		return machineManager, repository, advancer, app1
	}

	s.Run("ApplicationStateUpdate", func() {
		require := s.Require()

		_, repository, advancer, app := setup()
		inputs := []*Input{
			newInput(app.Application.ID, 0, 0, []byte("advance error")),
		}

		// Verify application state is updated on error
		err := advancer.processInputs(context.Background(), app.Application, inputs)
		require.Error(err)
		require.Equal(1, repository.ApplicationStateUpdates)
		require.Equal(ApplicationState_Inoperable, repository.LastApplicationState)
		require.NotNil(repository.LastApplicationStateReason)
		require.Equal("advance error", *repository.LastApplicationStateReason)
	})

	s.Run("ApplicationStateUpdateError", func() {
		require := s.Require()

		_, repository, advancer, app := setup()
		inputs := []*Input{
			newInput(app.Application.ID, 0, 0, []byte("advance error")),
		}
		repository.UpdateApplicationStateError = errors.New("update state error")

		// Verify error is still returned even if application state update fails
		err := advancer.processInputs(context.Background(), app.Application, inputs)
		require.Error(err)
		require.Contains(err.Error(), "advance error")
	})

	s.Run("Ok", func() {
		require := s.Require()

		_, repository, advancer, app := setup()
		inputs := []*Input{
			newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			newInput(app.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
			newInput(app.Application.ID, 0, 2, marshal(randomAdvanceResult(2))),
			newInput(app.Application.ID, 0, 3, marshal(randomAdvanceResult(3))),
			newInput(app.Application.ID, 1, 4, marshal(randomAdvanceResult(4))),
			newInput(app.Application.ID, 1, 5, marshal(randomAdvanceResult(5))),
			newInput(app.Application.ID, 2, 6, marshal(randomAdvanceResult(6))),
		}

		err := advancer.processInputs(context.Background(), app.Application, inputs)
		require.Nil(err)
		require.Len(repository.StoredResults, 7)
	})

	s.Run("Noop", func() {
		s.Run("NoInputs", func() {
			require := s.Require()

			_, _, advancer, app := setup()
			inputs := []*Input{}

			err := advancer.processInputs(context.Background(), app.Application, inputs)
			require.Nil(err)
		})
	})

	s.Run("Error", func() {
		s.Run("ErrApp", func() {
			require := s.Require()

			invalidApp := Application{ID: 999}
			_, _, advancer, _ := setup()
			inputs := randomInputs(1, 0, 3)

			err := advancer.processInputs(context.Background(), &invalidApp, inputs)
			expected := fmt.Sprintf("%v: %v", ErrNoApp, invalidApp.ID)
			require.EqualError(err, expected)
		})

		s.Run("Advance", func() {
			require := s.Require()

			_, repository, advancer, app := setup()
			inputs := []*Input{
				newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(app.Application.ID, 0, 1, []byte("advance error")),
				newInput(app.Application.ID, 0, 2, []byte("unreachable")),
			}

			err := advancer.processInputs(context.Background(), app.Application, inputs)
			require.Error(err)
			require.Contains(err.Error(), "advance error")
			require.Len(repository.StoredResults, 1)
		})

		s.Run("StoreAdvance", func() {
			require := s.Require()

			_, repository, advancer, app := setup()
			inputs := []*Input{
				newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(app.Application.ID, 0, 1, []byte("unreachable")),
			}
			repository.StoreAdvanceError = errors.New("store-advance error")

			err := advancer.processInputs(context.Background(), app.Application, inputs)
			require.Error(err)
			require.Contains(err.Error(), "store-advance error")
			require.Len(repository.StoredResults, 1)
		})
	})
}

// TestContextCancellation tests how the advancer handles context cancellation
func (s *AdvancerSuite) TestContextCancellation() {
	s.Run("CancelDuringStep", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1

		// Create a repository that will block until we cancel the context
		repository := &MockRepository{
			GetInputsBlock: true,
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		// Create a context that we can cancel
		ctx, cancel := context.WithCancel(context.Background())

		// Start the Step operation in a goroutine
		errCh := make(chan error)
		go func() {
			errCh <- advancer.Step(ctx)
		}()

		// Cancel the context after a short delay
		time.Sleep(10 * time.Millisecond)
		cancel()

		// Check that the operation was canceled
		select {
		case err := <-errCh:
			require.Error(err)
			require.ErrorIs(err, context.Canceled)
		case <-time.After(100 * time.Millisecond):
			require.Fail("Step operation did not respect context cancellation")
		}
	})

	s.Run("CancelDuringProcessInputs", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		// Create a machine that will block during Advance until we cancel the context
		app1.AdvanceBlock = true
		machineManager.Map[1] = *app1

		repository := &MockRepository{}
		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		// Create inputs and a context that we can cancel
		inputs := []*Input{
			newInput(app1.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
		}
		ctx, cancel := context.WithCancel(context.Background())

		// Start the processInputs operation in a goroutine
		errCh := make(chan error)
		go func() {
			errCh <- advancer.processInputs(ctx, app1.Application, inputs)
		}()

		// Cancel the context after a short delay
		time.Sleep(10 * time.Millisecond)
		cancel()

		// Check that the operation was canceled
		select {
		case err := <-errCh:
			require.Error(err)
			require.ErrorIs(err, context.Canceled)
		case <-time.After(100 * time.Millisecond):
			require.Fail("processInputs operation did not respect context cancellation")
		}
	})
}

// TestLargeNumberOfInputs how the advancer handles large volumes of inputs
func (s *AdvancerSuite) TestLargeNumberOfInputs() {
	s.Run("LargeNumberOfInputs", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1
		repository := &MockRepository{}
		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		// Create a large number of inputs
		const inputCount = 10000
		inputs := make([]*Input, inputCount)
		for i := range inputCount {
			inputs[i] = newInput(app1.Application.ID, 0, uint64(i), marshal(randomAdvanceResult(uint64(i))))
		}

		// Process the inputs
		err = advancer.processInputs(context.Background(), app1.Application, inputs)
		require.Nil(err)

		// Verify all inputs were processed
		require.Len(repository.StoredResults, inputCount)
	})
}

// TestErrorRecovery tests how the advancer recovers from temporary failures
func (s *AdvancerSuite) TestErrorRecovery() {
	s.Run("TemporaryRepositoryFailure", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		machineManager.Map[1] = *app1

		// Repository that fails on first attempt but succeeds on second
		repository := &MockRepository{
			StoreAdvanceFailCount: 1,
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		// Create inputs
		inputs := []*Input{
			newInput(app1.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			newInput(app1.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
		}

		// First attempt should fail
		err = advancer.processInputs(context.Background(), app1.Application, inputs)
		require.Error(err)
		require.Contains(err.Error(), "temporary failure")

		// Second attempt should succeed
		err = advancer.processInputs(context.Background(), app1.Application, inputs)
		require.Nil(err)
		require.Len(repository.StoredResults, 2)
	})
}

type MockMachineImpl struct {
	Application  *Application
	AdvanceBlock bool
	AdvanceError error
}

func (mock *MockMachineImpl) Advance(
	ctx context.Context,
	input []byte,
	_ uint64,
) (*AdvanceResult, error) {
	// If AdvanceBlock is true, block until context is canceled
	if mock.AdvanceBlock {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Hour): // Long timeout to ensure we're waiting for cancellation
			// This should never be reached in tests
			return nil, errors.New("advance timeout without cancellation")
		}
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// If there's a predefined error, return it
	if mock.AdvanceError != nil {
		return nil, mock.AdvanceError
	}

	var res AdvanceResult
	err := json.Unmarshal(input, &res)
	if err != nil {
		return nil, errors.New(string(input))
	}
	return &res, nil
}

func newMockMachine(id int64) *MockMachineImpl {
	return &MockMachineImpl{
		Application: &Application{
			ID:                  id,
			IApplicationAddress: randomAddress(),
		},
	}
}

// ------------------------------------------------------------------------------------------------

type MockMachineManager struct {
	Map                 map[int64]MockMachineImpl
	UpdateMachinesError error
}

func newMockMachineManager() *MockMachineManager {
	return &MockMachineManager{
		Map: map[int64]MockMachineImpl{},
	}
}

func (mock *MockMachineManager) GetMachine(appID int64) (manager.MachineInstance, bool) {
	machine, exists := mock.Map[appID]
	if !exists {
		return nil, false
	}

	// For testing purposes, we'll create a mock MachineInstance
	// that has the same Application but delegates the methods to our mock
	mockInstance := &MockMachineInstance{
		application: machine.Application,
		machineImpl: &machine,
	}

	return mockInstance, true
}

func (mock *MockMachineManager) UpdateMachines(ctx context.Context) error {
	return mock.UpdateMachinesError
}

func (mock *MockMachineManager) Applications() []*Application {
	apps := make([]*Application, 0, len(mock.Map))
	for _, v := range mock.Map {
		apps = append(apps, v.Application)
	}
	return apps
}

func (mock *MockMachineManager) HasMachine(appID int64) bool {
	_, exists := mock.Map[appID]
	return exists
}

// MockMachineInstance is a test implementation of manager.MachineInstance
type MockMachineInstance struct {
	application *Application
	machineImpl *MockMachineImpl
}

// Advance implements the MachineInstance interface for testing
func (m *MockMachineInstance) Advance(ctx context.Context, input []byte, index uint64) (*AdvanceResult, error) {
	return m.machineImpl.Advance(ctx, input, index)
}

// Inspect implements the MachineInstance interface for testing
func (m *MockMachineInstance) Inspect(ctx context.Context, query []byte) (*InspectResult, error) {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil, nil
}

// Application returns the application associated with this machine
func (m *MockMachineInstance) Application() *Application {
	return m.application
}

// Synchronize implements the MachineInstance interface for testing
func (m *MockMachineInstance) Synchronize(ctx context.Context, repo manager.MachineRepository) error {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil
}

// Close implements the MachineInstance interface for testing
func (m *MockMachineInstance) Close() error {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil
}

// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	GetInputsReturn             map[common.Address][]*Input
	GetInputsError              error
	GetInputsBlock              bool
	StoreAdvanceError           error
	StoreAdvanceFailCount       int
	UpdateApplicationStateError error
	UpdateEpochsError           error
	UpdateEpochsCount           int64

	StoredResults              []*AdvanceResult
	ApplicationStateUpdates    int
	LastApplicationState       ApplicationState
	LastApplicationStateReason *string

	mu sync.Mutex
}

func (mock *MockRepository) ListInputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.InputFilter,
	p repository.Pagination,
) ([]*Input, uint64, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	// If GetInputsBlock is true, block until context is canceled
	if mock.GetInputsBlock {
		<-ctx.Done()
		return nil, 0, ctx.Err()
	}

	address := common.HexToAddress(nameOrAddress)
	return mock.GetInputsReturn[address], uint64(len(mock.GetInputsReturn[address])), mock.GetInputsError
}

func (mock *MockRepository) StoreAdvanceResult(
	ctx context.Context,
	appID int64,
	res *AdvanceResult,
) error {
	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Thread-safe operations
	mock.mu.Lock()
	defer mock.mu.Unlock()

	// For temporary failure testing
	if mock.StoreAdvanceFailCount > 0 {
		mock.StoreAdvanceFailCount--
		return errors.New("temporary failure")
	}

	mock.StoredResults = append(mock.StoredResults, res)
	return mock.StoreAdvanceError
}

func (mock *MockRepository) UpdateEpochsInputsProcessed(ctx context.Context, nameOrAddress string) (int64, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	return mock.UpdateEpochsCount, mock.UpdateEpochsError
}

func (mock *MockRepository) UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error {
	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	mock.ApplicationStateUpdates++
	mock.LastApplicationState = state
	mock.LastApplicationStateReason = reason
	return mock.UpdateApplicationStateError
}

// ------------------------------------------------------------------------------------------------

func randomAddress() common.Address {
	address := make([]byte, 20)
	_, err := crand.Read(address)
	if err != nil {
		panic(err)
	}
	return common.BytesToAddress(address)
}

func randomHash() common.Hash {
	hash := make([]byte, 32)
	_, err := crand.Read(hash)
	if err != nil {
		panic(err)
	}
	return common.BytesToHash(hash)
}

func randomBytes() []byte {
	size := mrand.Intn(100) + 1
	bytes := make([]byte, size)
	_, err := crand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}

func randomSliceOfBytes() [][]byte {
	size := mrand.Intn(10) + 1
	slice := make([][]byte, size)
	for i := range size {
		slice[i] = randomBytes()
	}
	return slice
}

func newInput(appId int64, epochIndex uint64, inputIndex uint64, data []byte) *Input {
	return &Input{
		EpochApplicationID: appId,
		EpochIndex:         epochIndex,
		Index:              inputIndex,
		RawData:            data,
	}
}

func randomInputs(appId int64, epochIndex uint64, size int) []*Input {
	slice := make([]*Input, size)
	for i := range size {
		slice[i] = newInput(appId, epochIndex, uint64(i), randomBytes())
	}
	return slice
}

func randomAdvanceResult(inputIndex uint64) *AdvanceResult {
	hash := randomHash()
	res := &AdvanceResult{
		InputIndex:  inputIndex,
		Status:      InputCompletionStatus_Accepted,
		Outputs:     randomSliceOfBytes(),
		Reports:     randomSliceOfBytes(),
		OutputsHash: randomHash(),
		MachineHash: &hash,
	}
	return res
}

func marshal(res *AdvanceResult) []byte {
	data, err := json.Marshal(*res)
	if err != nil {
		panic(err)
	}
	return data
}
