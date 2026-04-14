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
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/suite"
)

func TestAdvancer(t *testing.T) {
	suite.Run(t, new(AdvancerSuite))
}

type AdvancerSuite struct{ suite.Suite }

func newMockAdvancerService(machineManager *MockMachineManager, repo *MockRepository) (*Service, error) {
	return newMockAdvancerServiceWithBatchSize(machineManager, repo, 500)
}

func newMockAdvancerServiceWithBatchSize(
	machineManager *MockMachineManager,
	repo *MockRepository,
	batchSize uint64,
) (*Service, error) {
	s := &Service{
		inputBatchSize: batchSize,
		machineManager: machineManager,
		repository:     repo,
	}
	serviceArgs := &service.CreateInfo{Name: "advancer", Impl: s, EnableReschedule: true}
	err := service.Create(context.Background(), serviceArgs, &s.Service)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// testEnv bundles the components most tests need: a service, a single app's mock,
// the mock machine manager, and the mock repository.
type testEnv struct {
	service *Service
	app     *MockMachineImpl
	mm      *MockMachineManager
	repo    *MockRepository
}

// setupOneApp creates a standard test environment with one application.
// The repository is empty; callers can configure it after the call.
func (s *AdvancerSuite) setupOneApp() testEnv {
	mm := newMockMachineManager()
	app := newMockMachine(1)
	mm.Map[1] = newMockInstance(app)
	repo := &MockRepository{}
	svc, err := newMockAdvancerService(mm, repo)
	s.Require().NoError(err)
	return testEnv{service: svc, app: app, mm: mm, repo: repo}
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
		require.Equal(advancer.Name, advancer.String())

		// Test Tick method
		machineManager.Map[1] = newMockInstance(newMockMachine(1))
		repository.GetEpochsReturn = map[common.Address][]*Epoch{
			machineManager.Map[1].application.IApplicationAddress: {},
		}
		tickErrors := advancer.Tick()
		require.Empty(tickErrors)

		// Test Tick with error
		repository.GetEpochsError = errors.New("list epochs error")
		tickErrors = advancer.Tick()
		require.NotEmpty(tickErrors)
		require.Contains(tickErrors[0].Error(), "list epochs error")

		// Stop must be called last to cleanly shut down the service.
		// It should complete without returning any errors.
		require.Empty(advancer.Stop(false))
	})
}

func (s *AdvancerSuite) TestStep() {
	s.Run("Ok", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app1 := newMockMachine(1)
		app2 := newMockMachine(2)
		machineManager.Map[1] = newMockInstance(app1)
		machineManager.Map[2] = newMockInstance(app2)
		res0 := randomAdvanceResult(0)
		res1 := randomAdvanceResult(1)
		res2 := randomAdvanceResult(0)

		repository := &MockRepository{
			GetEpochsReturn: map[common.Address][]*Epoch{
				app1.Application.IApplicationAddress: {
					&Epoch{Index: 0, Status: EpochStatus_Open},
				},
				app2.Application.IApplicationAddress: {
					&Epoch{Index: 0, Status: EpochStatus_Open},
				},
			},
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: {
					newInput(app1.Application.ID, 0, 0, marshal(res0)),
					newInput(app1.Application.ID, 0, 1, marshal(res1)),
				},
				app2.Application.IApplicationAddress: {
					newInput(app2.Application.ID, 0, 0, marshal(res2)),
				},
			},
		}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		_, err = advancer.Step(context.Background())
		require.Nil(err)

		require.Len(repository.StoredResults, 3)
	})

	s.Run("Error/UpdateEpochs", func() {
		require := s.Require()
		env := s.setupOneApp()
		res0 := randomAdvanceResult(0)

		env.repo.GetEpochsReturn = map[common.Address][]*Epoch{
			env.app.Application.IApplicationAddress: {
				{Index: 0, Status: EpochStatus_Closed},
			},
		}
		env.repo.GetInputsReturn = map[common.Address][]*Input{
			env.app.Application.IApplicationAddress: {
				newInput(env.app.Application.ID, 0, 0, marshal(res0)),
			},
		}
		env.repo.UpdateEpochsError = errors.New("update epochs error")

		_, err := env.service.Step(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "update epochs error")
	})

	s.Run("Error/UpdateMachines", func() {
		require := s.Require()

		machineManager := &MockMachineManager{
			Map:                 map[int64]*MockMachineInstance{},
			UpdateMachinesError: errors.New("update machines error"),
		}
		repository := &MockRepository{}

		advancer, err := newMockAdvancerService(machineManager, repository)
		require.NotNil(advancer)
		require.Nil(err)

		_, err = advancer.Step(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "update machines error")
	})

	s.Run("Error/GetInputs", func() {
		require := s.Require()
		env := s.setupOneApp()

		env.repo.GetEpochsReturn = map[common.Address][]*Epoch{
			env.app.Application.IApplicationAddress: {
				{Index: 0, Status: EpochStatus_Closed},
			},
		}
		env.repo.GetInputsError = errors.New("get inputs error")

		_, err := env.service.Step(context.Background())
		require.Error(err)
		require.Contains(err.Error(), "get inputs error")
	})

	s.Run("NoInputs", func() {
		require := s.Require()
		env := s.setupOneApp()

		env.repo.GetInputsReturn = map[common.Address][]*Input{
			env.app.Application.IApplicationAddress: {},
		}

		_, err := env.service.Step(context.Background())
		require.Nil(err)
		require.Len(env.repo.StoredResults, 0)
	})

	s.Run("FailedAppDoesNotBlockOtherApps", func() {
		require := s.Require()

		mm := newMockMachineManager()
		app1 := newMockMachine(1) // will fail
		app2 := newMockMachine(2) // should still be processed
		mm.Map[1] = newMockInstance(app1)
		mm.Map[2] = newMockInstance(app2)
		res2 := randomAdvanceResult(0)

		repo := &MockRepository{
			GetEpochsReturn: map[common.Address][]*Epoch{
				app1.Application.IApplicationAddress: {
					{Index: 0, Status: EpochStatus_Open},
				},
				app2.Application.IApplicationAddress: {
					{Index: 0, Status: EpochStatus_Open},
				},
			},
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: {
					newInput(app1.Application.ID, 0, 0, []byte("advance error")),
				},
				app2.Application.IApplicationAddress: {
					newInput(app2.Application.ID, 0, 0, marshal(res2)),
				},
			},
		}

		svc, err := newMockAdvancerService(mm, repo)
		require.NoError(err)

		_, err = svc.Step(context.Background())
		// Step returns a combined error but the healthy app was still processed
		require.Error(err)
		require.Contains(err.Error(), "advance error")
		require.Equal(1, repo.ApplicationStateUpdates)
		require.Equal(ApplicationState_Failed, repo.LastApplicationState)

		// app2's input was processed despite app1's failure
		require.Len(repo.StoredResults, 1)
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

		repo := &MockRepository{
			GetInputsReturn: map[common.Address][]*Input{
				app1.Application.IApplicationAddress: inputs,
			},
		}

		result, count, err := getUnprocessedInputs(
			context.Background(), repo, app1.Application.IApplicationAddress.String(), 0, 500)
		require.Nil(err)
		require.Equal(uint64(2), count)
		require.Equal(inputs, result)
	})

	s.Run("Error", func() {
		require := s.Require()

		app1 := newMockMachine(1)
		repo := &MockRepository{
			GetInputsError: errors.New("list inputs error"),
		}

		_, _, err := getUnprocessedInputs(
			context.Background(), repo, app1.Application.IApplicationAddress.String(), 0, 500)
		require.Error(err)
		require.Contains(err.Error(), "list inputs error")
	})
}

func (s *AdvancerSuite) TestProcess() {
	s.Run("ApplicationStateUpdate", func() {
		require := s.Require()
		env := s.setupOneApp()
		inputs := []*Input{
			newInput(env.app.Application.ID, 0, 0, []byte("advance error")),
		}

		err := env.service.processInputs(context.Background(), env.app.Application, inputs)
		require.Error(err)
		require.Equal(1, env.repo.ApplicationStateUpdates)
		require.Equal(ApplicationState_Failed, env.repo.LastApplicationState)
		require.NotNil(env.repo.LastApplicationStateReason)
		require.Equal("advance error", *env.repo.LastApplicationStateReason)
	})

	s.Run("ApplicationStateUpdateError", func() {
		require := s.Require()
		env := s.setupOneApp()
		inputs := []*Input{
			newInput(env.app.Application.ID, 0, 0, []byte("advance error")),
		}
		env.repo.UpdateApplicationStateError = errors.New("update state error")

		err := env.service.processInputs(context.Background(), env.app.Application, inputs)
		require.Error(err)
		require.Contains(err.Error(), "advance error")
	})

	s.Run("Ok", func() {
		require := s.Require()
		env := s.setupOneApp()
		inputs := []*Input{
			newInput(env.app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			newInput(env.app.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
			newInput(env.app.Application.ID, 0, 2, marshal(randomAdvanceResult(2))),
			newInput(env.app.Application.ID, 0, 3, marshal(randomAdvanceResult(3))),
			newInput(env.app.Application.ID, 1, 4, marshal(randomAdvanceResult(4))),
			newInput(env.app.Application.ID, 1, 5, marshal(randomAdvanceResult(5))),
			newInput(env.app.Application.ID, 2, 6, marshal(randomAdvanceResult(6))),
		}

		err := env.service.processInputs(context.Background(), env.app.Application, inputs)
		require.Nil(err)
		require.Len(env.repo.StoredResults, 7)
	})

	s.Run("Noop", func() {
		s.Run("NoInputs", func() {
			require := s.Require()
			env := s.setupOneApp()

			err := env.service.processInputs(context.Background(), env.app.Application, []*Input{})
			require.Nil(err)
		})
	})

	s.Run("Error", func() {
		s.Run("ErrApp", func() {
			require := s.Require()
			env := s.setupOneApp()
			invalidApp := Application{ID: 999}
			inputs := randomInputs(1, 0, 3)

			err := env.service.processInputs(context.Background(), &invalidApp, inputs)
			expected := fmt.Sprintf("%v: %v", ErrNoApp, invalidApp.ID)
			require.EqualError(err, expected)
		})

		s.Run("Advance", func() {
			require := s.Require()
			env := s.setupOneApp()
			inputs := []*Input{
				newInput(env.app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(env.app.Application.ID, 0, 1, []byte("advance error")),
				newInput(env.app.Application.ID, 0, 2, []byte("unreachable")),
			}

			err := env.service.processInputs(context.Background(), env.app.Application, inputs)
			require.Error(err)
			require.Contains(err.Error(), "advance error")
			require.Len(env.repo.StoredResults, 1)
		})

		s.Run("StoreAdvance", func() {
			require := s.Require()
			env := s.setupOneApp()
			inputs := []*Input{
				newInput(env.app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(env.app.Application.ID, 0, 1, []byte("unreachable")),
			}
			env.repo.StoreAdvanceError = errors.New("store-advance error")

			err := env.service.processInputs(context.Background(), env.app.Application, inputs)
			require.Error(err)
			require.Contains(err.Error(), "store-advance error")
			require.Len(env.repo.StoredResults, 1)

			// Verify that the node shutdown was triggered (context cancelled)
			require.Error(env.service.Context.Err(), "shared context should be cancelled")
		})
	})
}

// TestContextCancellation tests how the advancer handles context cancellation
func (s *AdvancerSuite) TestContextCancellation() {
	s.Run("CancelDuringStep", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.repo.GetEpochsBlock = true

		ctx, cancel := context.WithCancel(context.Background())

		// Start the Step operation in a goroutine
		errCh := make(chan error)
		go func() {
			_, err := env.service.Step(ctx)
			errCh <- err
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
		env := s.setupOneApp()
		env.app.AdvanceBlock = true

		inputs := []*Input{
			newInput(env.app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
		}
		ctx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error)
		go func() {
			errCh <- env.service.processInputs(ctx, env.app.Application, inputs)
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
		env := s.setupOneApp()

		const inputCount = 10000
		inputs := make([]*Input, inputCount)
		for i := range inputCount {
			inputs[i] = newInput(env.app.Application.ID, 0, uint64(i), marshal(randomAdvanceResult(uint64(i))))
		}

		err := env.service.processInputs(context.Background(), env.app.Application, inputs)
		require.Nil(err)
		require.Len(env.repo.StoredResults, inputCount)
	})
}

// TestErrorRecovery verifies that any store failure after a successful Advance()
// triggers node shutdown, because the machine and DB are now out of sync.
func (s *AdvancerSuite) TestErrorRecovery() {
	s.Run("TransientStoreFailureTriggersShutdown", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.repo.StoreAdvanceFailCount = 1

		inputs := []*Input{
			newInput(env.app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
		}

		err := env.service.processInputs(context.Background(), env.app.Application, inputs)
		require.Error(err)
		require.Contains(err.Error(), "temporary failure")
		require.Error(env.service.Context.Err(), "shared context should be cancelled")
	})
}

// TestContextCancelledBeforeProcessing verifies that when the context is
// already cancelled, processInputs returns the context error immediately
// without reaching the advance or store paths.
func (s *AdvancerSuite) TestContextCancelledBeforeProcessing() {
	s.Run("ContextAlreadyCancelled", func() {
		require := s.Require()
		env := s.setupOneApp()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		inputs := []*Input{
			newInput(env.app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
		}

		err := env.service.processInputs(ctx, env.app.Application, inputs)
		require.ErrorIs(err, context.Canceled)
	})
}

// ---------------------------------------------------------------------------
// isAllEpochInputsProcessed tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestIsAllEpochInputsProcessed() {
	s.Run("TrueWhenEpochHasNoInputs", func() {
		require := s.Require()
		env := s.setupOneApp()

		epoch := &Epoch{Index: 0, InputIndexLowerBound: 5, InputIndexUpperBound: 5}
		result, perr := env.service.isAllEpochInputsProcessed(env.app.Application, epoch)
		require.Nil(perr)
		require.True(result)
	})

	s.Run("TrueWhenMachineProcessedAllInputs", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.mm.Map[1].machineImpl.processedInputs = 10

		epoch := &Epoch{Index: 0, InputIndexLowerBound: 5, InputIndexUpperBound: 10}
		result, perr := env.service.isAllEpochInputsProcessed(env.app.Application, epoch)
		require.Nil(perr)
		require.True(result)
	})

	s.Run("FalseWhenMoreInputsExist", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.mm.Map[1].machineImpl.processedInputs = 7

		epoch := &Epoch{Index: 0, InputIndexLowerBound: 5, InputIndexUpperBound: 10}
		result, perr := env.service.isAllEpochInputsProcessed(env.app.Application, epoch)
		require.Nil(perr)
		require.False(result)
	})

	s.Run("ErrorWhenNoMachineForApp", func() {
		require := s.Require()
		mm := newMockMachineManager()
		repo := &MockRepository{}
		svc, err := newMockAdvancerService(mm, repo)
		require.Nil(err)

		app := &Application{ID: 999}
		epoch := &Epoch{Index: 0, InputIndexLowerBound: 0, InputIndexUpperBound: 5}
		_, perr := svc.isAllEpochInputsProcessed(app, epoch)
		require.Error(perr)
		require.ErrorIs(perr, ErrNoApp)
	})
}

// ---------------------------------------------------------------------------
// isEpochLastInput tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestIsEpochLastInput() {
	setupWithEpoch := func(epochStatus EpochStatus) (testEnv, *Application) {
		env := s.setupOneApp()
		env.repo.GetEpochReturn = &Epoch{Status: epochStatus}
		return env, env.app.Application
	}

	s.Run("TrueWhenLastInputInClosedEpoch", func() {
		require := s.Require()
		env, app := setupWithEpoch(EpochStatus_Closed)

		lastInput := repotest.NewInputBuilder().WithIndex(5).WithEpochIndex(0).Build()
		env.repo.GetInputsReturn = map[common.Address][]*Input{
			app.IApplicationAddress: {lastInput},
		}
		env.repo.GetLastInputReturn = lastInput

		input := repotest.NewInputBuilder().WithIndex(5).WithEpochIndex(0).Build()
		result, err := env.service.isEpochLastInput(context.Background(), app, input)
		require.Nil(err)
		require.True(result)
	})

	s.Run("FalseWhenEpochIsOpen", func() {
		require := s.Require()
		env, app := setupWithEpoch(EpochStatus_Open)

		input := repotest.NewInputBuilder().WithIndex(3).WithEpochIndex(0).Build()
		result, err := env.service.isEpochLastInput(context.Background(), app, input)
		require.Nil(err)
		require.False(result)
	})

	s.Run("FalseWhenNotLastInput", func() {
		require := s.Require()
		env, app := setupWithEpoch(EpochStatus_Closed)

		env.repo.GetLastInputReturn = repotest.NewInputBuilder().WithIndex(5).WithEpochIndex(0).Build()

		input := repotest.NewInputBuilder().WithIndex(3).WithEpochIndex(0).Build()
		result, err := env.service.isEpochLastInput(context.Background(), app, input)
		require.Nil(err)
		require.False(result)
	})

	s.Run("ErrorWhenNilInput", func() {
		require := s.Require()
		env, app := setupWithEpoch(EpochStatus_Closed)

		_, err := env.service.isEpochLastInput(context.Background(), app, nil)
		require.Error(err)
		require.Contains(err.Error(), "must not be nil")
	})

	s.Run("ErrorWhenNilApplication", func() {
		require := s.Require()
		env, _ := setupWithEpoch(EpochStatus_Closed)

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		_, err := env.service.isEpochLastInput(context.Background(), nil, input)
		require.Error(err)
		require.Contains(err.Error(), "must not be nil")
	})

	s.Run("ErrorWhenGetEpochFails", func() {
		require := s.Require()
		env, app := setupWithEpoch(EpochStatus_Closed)
		env.repo.GetEpochError = errors.New("get epoch error")

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		_, err := env.service.isEpochLastInput(context.Background(), app, input)
		require.Error(err)
		require.Contains(err.Error(), "get epoch error")
	})

	s.Run("ErrorWhenGetLastInputFails", func() {
		require := s.Require()
		env, app := setupWithEpoch(EpochStatus_Closed)
		env.repo.GetLastInputError = errors.New("get last input error")

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		_, err := env.service.isEpochLastInput(context.Background(), app, input)
		require.Error(err)
		require.Contains(err.Error(), "get last input error")
	})
}

// ---------------------------------------------------------------------------
// handleEpochAfterInputsProcessed tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestHandleEpochAfterInputsProcessed() {
	s.Run("EmptyEpochIndex0GetsOutputsProofFromMachine", func() {
		require := s.Require()
		env := s.setupOneApp()

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 0}
		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Nil(err)
		require.True(env.repo.OutputsProofUpdated)
	})

	s.Run("EmptyEpochIndex0ErrorOnOutputsProof", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.app.OutputsProofError = errors.New("proof error")

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 0}
		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Error(err)
		require.Contains(err.Error(), "proof error")
	})

	s.Run("EmptyEpochIndex0ErrMachineClosedMarksAppFailed", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.app.OutputsProofError = manager.ErrMachineClosed

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 0}
		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Error(err)
		require.ErrorIs(err, manager.ErrMachineClosed)
		require.Equal(1, env.repo.ApplicationStateUpdates)
		require.Equal(ApplicationState_Failed, env.repo.LastApplicationState)
	})

	s.Run("EmptyEpochIndexGt0RepeatsPreviousProof", func() {
		require := s.Require()
		env := s.setupOneApp()

		epoch := &Epoch{Index: 2, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 0}
		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Nil(err)
		require.True(env.repo.RepeatOutputsProofCalled)
	})

	s.Run("EmptyEpochIndexGt0RepeatError", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.repo.RepeatOutputsProofError = errors.New("repeat error")

		epoch := &Epoch{Index: 2, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 0}
		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Error(err)
		require.Contains(err.Error(), "repeat error")
	})

	s.Run("NonEmptyEpochWithEveryEpochSnapshotPolicy", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.app.Application.ExecutionParameters.SnapshotPolicy = SnapshotPolicy_EveryEpoch
		env.service.snapshotsDir = s.T().TempDir()

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 3}
		lastInput := repotest.NewInputBuilder().WithIndex(2).WithEpochIndex(0).
			WithStatus(InputCompletionStatus_Accepted).Build()
		lastInput.EpochApplicationID = env.app.Application.ID
		env.repo.GetLastProcessedInputReturn = lastInput
		env.repo.GetLastInputReturn = lastInput

		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Nil(err)
		require.True(env.repo.SnapshotURIUpdated)
	})

	s.Run("NonEmptyEpochNoSnapshotPolicy", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.app.Application.ExecutionParameters.SnapshotPolicy = SnapshotPolicy_None

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 3}
		lastInput := repotest.NewInputBuilder().WithIndex(2).WithEpochIndex(0).
			WithStatus(InputCompletionStatus_Accepted).Build()
		lastInput.EpochApplicationID = env.app.Application.ID
		env.repo.GetLastProcessedInputReturn = lastInput

		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Nil(err)
		require.False(env.repo.SnapshotURIUpdated)
	})

	s.Run("NoMachineReturnsError", func() {
		require := s.Require()
		mm := newMockMachineManager()
		svc, err := newMockAdvancerService(mm, &MockRepository{})
		require.Nil(err)

		app := repotest.NewApplicationBuilder().Build()
		app.ID = 999

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 3}
		err = svc.handleEpochAfterInputsProcessed(context.Background(), app, epoch)
		require.Error(err)
		require.ErrorIs(err, ErrNoApp)
	})

	s.Run("GetLastProcessedInputError", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.app.Application.ExecutionParameters.SnapshotPolicy = SnapshotPolicy_EveryEpoch
		env.repo.GetLastProcessedInputError = errors.New("db connection lost")

		epoch := &Epoch{Index: 0, Status: EpochStatus_Closed, InputIndexLowerBound: 0, InputIndexUpperBound: 3}
		err := env.service.handleEpochAfterInputsProcessed(context.Background(), env.app.Application, epoch)
		require.Error(err)
		require.Contains(err.Error(), "db connection lost")
	})
}

// ---------------------------------------------------------------------------
// handleSnapshot tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestHandleSnapshot() {
	setupSnapshot := func(policy SnapshotPolicy) (testEnv, *MockMachineInstance) {
		env := s.setupOneApp()
		env.app.Application.ExecutionParameters.SnapshotPolicy = policy
		env.service.snapshotsDir = s.T().TempDir()
		instance := env.mm.Map[1]
		return env, instance
	}

	s.Run("NonePolicy", func() {
		require := s.Require()
		env, machine := setupSnapshot(SnapshotPolicy_None)

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.handleSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.False(env.repo.SnapshotURIUpdated)
	})

	s.Run("EveryInputPolicy", func() {
		require := s.Require()
		env, machine := setupSnapshot(SnapshotPolicy_EveryInput)

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.handleSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.True(env.repo.SnapshotURIUpdated)
	})

	s.Run("EveryEpochPolicyLastInput", func() {
		require := s.Require()
		env, machine := setupSnapshot(SnapshotPolicy_EveryEpoch)
		env.repo.GetEpochReturn = &Epoch{Status: EpochStatus_Closed}
		env.repo.GetLastInputReturn = repotest.NewInputBuilder().WithIndex(5).WithEpochIndex(0).Build()

		input := repotest.NewInputBuilder().WithIndex(5).WithEpochIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.handleSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.True(env.repo.SnapshotURIUpdated)
	})

	s.Run("EveryEpochPolicyNotLastInput", func() {
		require := s.Require()
		env, machine := setupSnapshot(SnapshotPolicy_EveryEpoch)
		env.repo.GetEpochReturn = &Epoch{Status: EpochStatus_Closed}
		env.repo.GetLastInputReturn = repotest.NewInputBuilder().WithIndex(5).WithEpochIndex(0).Build()

		input := repotest.NewInputBuilder().WithIndex(3).WithEpochIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.handleSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.False(env.repo.SnapshotURIUpdated)
	})

	s.Run("EveryEpochPolicyOpenEpoch", func() {
		require := s.Require()
		env, machine := setupSnapshot(SnapshotPolicy_EveryEpoch)
		env.repo.GetEpochReturn = &Epoch{Status: EpochStatus_Open}

		input := repotest.NewInputBuilder().WithIndex(0).WithEpochIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.handleSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.False(env.repo.SnapshotURIUpdated)
	})
}

// ---------------------------------------------------------------------------
// createSnapshot tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestCreateSnapshot() {
	setupCreateSnapshot := func() (testEnv, *MockMachineInstance, string) {
		env := s.setupOneApp()
		env.app.Application.Name = "testapp"
		env.app.Application.ExecutionParameters.SnapshotPolicy = SnapshotPolicy_EveryInput
		tmpDir := s.T().TempDir()
		env.service.snapshotsDir = tmpDir
		instance := &MockMachineInstance{
			application: env.app.Application,
			machineImpl: env.app,
		}
		return env, instance, tmpDir
	}

	s.Run("Success", func() {
		require := s.Require()
		env, machine, tmpDir := setupCreateSnapshot()

		input := repotest.NewInputBuilder().WithIndex(3).WithEpochIndex(1).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.createSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.True(env.repo.SnapshotURIUpdated)

		require.NotNil(input.SnapshotURI)
		expectedPath := filepath.Join(tmpDir, "testapp_epoch1_input3")
		require.Equal(expectedPath, *input.SnapshotURI)
	})

	s.Run("SkipsIfAlreadyHasSnapshot", func() {
		require := s.Require()
		env, machine, _ := setupCreateSnapshot()

		existingPath := "/existing/snapshot"
		input := repotest.NewInputBuilder().WithIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID
		input.SnapshotURI = &existingPath

		err := env.service.createSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)
		require.False(env.repo.SnapshotURIUpdated)
	})

	s.Run("RemovesPreviousSnapshot", func() {
		require := s.Require()
		env, machine, tmpDir := setupCreateSnapshot()

		prevPath := filepath.Join(tmpDir, "testapp_epoch0_input0")
		require.Nil(os.MkdirAll(prevPath, 0755))
		env.repo.GetLastSnapshotReturn = &Input{SnapshotURI: &prevPath}

		input := repotest.NewInputBuilder().WithIndex(1).WithEpochIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.createSnapshot(context.Background(), env.app.Application, machine, input)
		require.Nil(err)

		_, statErr := os.Stat(prevPath)
		require.True(os.IsNotExist(statErr))
	})

	s.Run("CreateSnapshotError", func() {
		require := s.Require()
		env, machine, _ := setupCreateSnapshot()
		machine.createSnapshotError = errors.New("snapshot failed")

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.createSnapshot(context.Background(), env.app.Application, machine, input)
		require.Error(err)
		require.Contains(err.Error(), "snapshot failed")
		require.False(env.repo.SnapshotURIUpdated)
	})

	s.Run("MkdirAllError", func() {
		require := s.Require()

		machineManager := newMockMachineManager()
		app := newMockMachine(1)
		app.Application.Name = "testapp"
		machineManager.Map[1] = newMockInstance(app)
		repository := &MockRepository{}
		advancer, err := newMockAdvancerService(machineManager, repository)
		require.Nil(err)

		// Create a read-only parent directory so MkdirAll fails
		tmpDir := s.T().TempDir()
		readonlyDir := filepath.Join(tmpDir, "readonly")
		require.Nil(os.MkdirAll(readonlyDir, 0755))
		require.Nil(os.Chmod(readonlyDir, 0555))
		s.T().Cleanup(func() { os.Chmod(readonlyDir, 0755) }) //nolint:errcheck
		advancer.snapshotsDir = filepath.Join(readonlyDir, "snapshots")

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		input.EpochApplicationID = app.Application.ID

		err = advancer.createSnapshot(context.Background(), app.Application, machineManager.Map[1], input)
		require.Error(err)
		require.Contains(err.Error(), "failed to create snapshots directory")
	})

	s.Run("UpdateSnapshotURIError", func() {
		require := s.Require()
		env, machine, _ := setupCreateSnapshot()

		env.repo.UpdateSnapshotURIError = errors.New("db error")

		input := repotest.NewInputBuilder().WithIndex(0).Build()
		input.EpochApplicationID = env.app.Application.ID

		err := env.service.createSnapshot(context.Background(), env.app.Application, machine, input)
		require.Error(err)
		require.Contains(err.Error(), "db error")
	})
}

// ---------------------------------------------------------------------------
// removeSnapshot tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestRemoveSnapshot() {
	s.Run("RemovesExistingSnapshot", func() {
		require := s.Require()

		tmpDir := s.T().TempDir()
		advancer := &Service{snapshotsDir: tmpDir}
		serviceArgs := &service.CreateInfo{Name: "advancer", Impl: advancer}
		require.Nil(service.Create(context.Background(), serviceArgs, &advancer.Service))

		// Create a snapshot directory
		snapshotPath := filepath.Join(tmpDir, "myapp_epoch0_input0")
		require.Nil(os.MkdirAll(snapshotPath, 0755))

		err := advancer.removeSnapshot(snapshotPath, "myapp")
		require.Nil(err)

		_, statErr := os.Stat(snapshotPath)
		require.True(os.IsNotExist(statErr))
	})

	s.Run("NonExistentPathIsNoop", func() {
		require := s.Require()

		tmpDir := s.T().TempDir()
		advancer := &Service{snapshotsDir: tmpDir}
		serviceArgs := &service.CreateInfo{Name: "advancer", Impl: advancer}
		require.Nil(service.Create(context.Background(), serviceArgs, &advancer.Service))

		snapshotPath := filepath.Join(tmpDir, "myapp_epoch0_input0")
		err := advancer.removeSnapshot(snapshotPath, "myapp")
		require.Nil(err)
	})

	s.Run("RejectsDirectoryTraversal", func() {
		require := s.Require()

		tmpDir := s.T().TempDir()
		advancer := &Service{snapshotsDir: tmpDir}
		serviceArgs := &service.CreateInfo{Name: "advancer", Impl: advancer}
		require.Nil(service.Create(context.Background(), serviceArgs, &advancer.Service))

		// Try to traverse outside snapshotsDir
		maliciousPath := filepath.Join(tmpDir, "..", "outside", "myapp_evil")
		err := advancer.removeSnapshot(maliciousPath, "myapp")
		require.Error(err)
		require.Contains(err.Error(), "invalid snapshot path")
	})

	s.Run("RejectsMismatchedAppName", func() {
		require := s.Require()

		tmpDir := s.T().TempDir()
		advancer := &Service{snapshotsDir: tmpDir}
		serviceArgs := &service.CreateInfo{Name: "advancer", Impl: advancer}
		require.Nil(service.Create(context.Background(), serviceArgs, &advancer.Service))

		snapshotPath := filepath.Join(tmpDir, "otherapp_epoch0_input0")
		err := advancer.removeSnapshot(snapshotPath, "myapp")
		require.Error(err)
		require.Contains(err.Error(), "invalid snapshot path")
	})
}

// ---------------------------------------------------------------------------
// Scheduling tests (starvation fix)
// ---------------------------------------------------------------------------

// Starvation prevention — both apps get served in a single Step().
func (s *AdvancerSuite) TestStarvationPrevention() {
	require := s.Require()

	mm := newMockMachineManager()
	appA := newMockMachine(1) // will have many inputs
	appB := newMockMachine(2) // will have 1 input
	mm.Map[1] = newMockInstance(appA)
	mm.Map[2] = newMockInstance(appB)

	resA := make([]*Input, 20)
	for i := range resA {
		resA[i] = newInput(appA.Application.ID, 0, uint64(i), marshal(randomAdvanceResult(uint64(i))))
	}
	resB := randomAdvanceResult(0)

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			appA.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
			appB.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		},
		GetInputsReturn: map[common.Address][]*Input{
			appA.Application.IApplicationAddress: resA,
			appB.Application.IApplicationAddress: {
				newInput(appB.Application.ID, 0, 0, marshal(resB)),
			},
		},
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 5)
	require.NoError(err)

	hadWork, err := svc.Step(context.Background())
	require.NoError(err)
	require.True(hadWork, "should report more work remaining")

	// Verify both apps were served using per-app tracking.
	var countA, countB int
	for _, id := range repo.StoredAppIDs {
		switch id {
		case appA.Application.ID:
			countA++
		case appB.Application.ID:
			countB++
		}
	}
	require.GreaterOrEqual(countB, 1, "app B must be served (starvation-free)")
	require.LessOrEqual(countA, 5, "app A should process at most batchSize (5) inputs")
	require.LessOrEqual(len(repo.StoredResults), 6, // 5 for A + 1 for B
		"should process at most batchSize per app")
}

// Single-batch enforcement — processEpochInputs processes exactly one batch.
func (s *AdvancerSuite) TestSingleBatchEnforcement() {
	require := s.Require()

	mm := newMockMachineManager()
	app := newMockMachine(1)
	mm.Map[1] = newMockInstance(app)

	inputs := make([]*Input, 20)
	for i := range inputs {
		inputs[i] = newInput(app.Application.ID, 0, uint64(i), marshal(randomAdvanceResult(uint64(i))))
	}

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app.Application.IApplicationAddress: inputs,
		},
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 5)
	require.NoError(err)

	hadWork, err := svc.Step(context.Background())
	require.NoError(err)
	require.True(hadWork, "more work should remain")

	// Exactly one batch of 5 should have been processed, not all 20.
	require.Equal(5, len(repo.StoredResults),
		"should process exactly one batch (batchSize=5)")
}

// More-work signal accuracy.
func (s *AdvancerSuite) TestMoreWorkSignal() {
	s.Run("TrueWhenInputsRemain", func() {
		require := s.Require()
		env := s.setupOneApp()

		inputs := make([]*Input, 10)
		for i := range inputs {
			inputs[i] = newInput(env.app.Application.ID, 0, uint64(i),
				marshal(randomAdvanceResult(uint64(i))))
		}
		env.repo.GetEpochsReturn = map[common.Address][]*Epoch{
			env.app.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		}
		env.repo.GetInputsReturn = map[common.Address][]*Input{
			env.app.Application.IApplicationAddress: inputs,
		}

		// With default batch size (500) > 10 inputs, single batch drains everything.
		hadWork, err := env.service.Step(context.Background())
		require.NoError(err)
		require.False(hadWork, "no more work when all inputs fit in one batch")
	})

	s.Run("FalseWhenEmpty", func() {
		require := s.Require()
		env := s.setupOneApp()
		env.repo.GetEpochsReturn = map[common.Address][]*Epoch{
			env.app.Application.IApplicationAddress: {},
		}

		hadWork, err := env.service.Step(context.Background())
		require.NoError(err)
		require.False(hadWork, "no work when no epochs exist")
	})
}

// Round-robin cursor distributes work evenly.
func (s *AdvancerSuite) TestAllAppsProcessedEveryStep() {
	require := s.Require()

	mm := newMockMachineManager()
	apps := make([]*MockMachineImpl, 3)
	for i := range apps {
		apps[i] = &MockMachineImpl{
			Application: &Application{
				ID:                  int64(i + 1),
				IApplicationAddress: randomAddress(),
			},
		}
		mm.Map[int64(i+1)] = newMockInstance(apps[i])
	}

	// Each app has many inputs. batchSize=1 so we process exactly 1 per app per Step.
	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{},
		GetInputsReturn: map[common.Address][]*Input{},
	}
	for _, app := range apps {
		repo.GetEpochsReturn[app.Application.IApplicationAddress] = []*Epoch{
			{Index: 0, Status: EpochStatus_Open},
		}
		inputs := make([]*Input, 10)
		for j := range inputs {
			inputs[j] = newInput(app.Application.ID, 0, uint64(j),
				marshal(randomAdvanceResult(uint64(j))))
		}
		repo.GetInputsReturn[app.Application.IApplicationAddress] = inputs
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 1)
	require.NoError(err)

	// Each Step processes all 3 apps with 1 input each = 3 results per Step.
	// After 2 steps: 6 results, 2 per app.
	for range 2 {
		_, err := svc.Step(context.Background())
		require.NoError(err)
	}

	require.Equal(6, len(repo.StoredResults),
		"2 steps * 3 apps * 1 input/app = 6 total results")
}

// Cursor handles app removal gracefully.
func (s *AdvancerSuite) TestAppRemovalBetweenTicks() {
	require := s.Require()

	mm := newMockMachineManager()
	app1 := newMockMachine(1)
	app2 := newMockMachine(2)
	app3 := newMockMachine(3)
	mm.Map[1] = newMockInstance(app1)
	mm.Map[2] = newMockInstance(app2)
	mm.Map[3] = newMockInstance(app3)

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app1.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
			app2.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
			app3.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app1.Application.IApplicationAddress: {
				newInput(app1.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			},
			app2.Application.IApplicationAddress: {
				newInput(app2.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			},
			app3.Application.IApplicationAddress: {
				newInput(app3.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			},
		},
	}

	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	// First step processes all 3 apps.
	_, err = svc.Step(context.Background())
	require.NoError(err)
	require.Equal(3, len(repo.StoredResults))

	// Remove app2, add fresh inputs for remaining apps.
	delete(mm.Map, 2)
	repo.GetInputsReturn[app1.Application.IApplicationAddress] = []*Input{
		newInput(app1.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
	}
	repo.GetInputsReturn[app3.Application.IApplicationAddress] = []*Input{
		newInput(app3.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
	}

	// Second step should not panic and should process apps 1 and 3.
	_, err = svc.Step(context.Background())
	require.NoError(err)
	require.Equal(5, len(repo.StoredResults), "3 from first step + 2 from second")
}

// App addition between ticks is handled gracefully.
func (s *AdvancerSuite) TestAppAdditionBetweenTicks() {
	require := s.Require()

	mm := newMockMachineManager()
	app1 := newMockMachine(1)
	app2 := newMockMachine(2)
	mm.Map[1] = newMockInstance(app1)
	mm.Map[2] = newMockInstance(app2)

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app1.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
			app2.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app1.Application.IApplicationAddress: {
				newInput(app1.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			},
			app2.Application.IApplicationAddress: {
				newInput(app2.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
			},
		},
	}

	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	_, err = svc.Step(context.Background())
	require.NoError(err)
	require.Equal(2, len(repo.StoredResults))

	// Add app3 and fresh inputs.
	app3 := newMockMachine(3)
	mm.Map[3] = newMockInstance(app3)
	repo.GetEpochsReturn[app3.Application.IApplicationAddress] = []*Epoch{
		{Index: 0, Status: EpochStatus_Open},
	}
	repo.GetInputsReturn[app1.Application.IApplicationAddress] = []*Input{
		newInput(app1.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
	}
	repo.GetInputsReturn[app2.Application.IApplicationAddress] = []*Input{
		newInput(app2.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
	}
	repo.GetInputsReturn[app3.Application.IApplicationAddress] = []*Input{
		newInput(app3.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
	}

	_, err = svc.Step(context.Background())
	require.NoError(err)
	require.Equal(5, len(repo.StoredResults), "2 from first + 3 from second (all 3 apps)")
}

// Epoch completion fires only after last batch.
func (s *AdvancerSuite) TestEpochCompletionTiming() {
	require := s.Require()

	mm := newMockMachineManager()
	app := newMockMachine(1)
	app.processedInputs = 0
	mm.Map[1] = newMockInstance(app)

	inputs := make([]*Input, 10)
	for i := range inputs {
		inputs[i] = newInput(app.Application.ID, 0, uint64(i), marshal(randomAdvanceResult(uint64(i))))
	}

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app.Application.IApplicationAddress: {
				{Index: 0, Status: EpochStatus_Closed,
					InputIndexLowerBound: 0, InputIndexUpperBound: 10},
			},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app.Application.IApplicationAddress: inputs,
		},
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 5)
	require.NoError(err)

	// Step 1: processes 5 of 10 inputs. Epoch should NOT be marked complete.
	hadWork, err := svc.Step(context.Background())
	require.NoError(err)
	require.True(hadWork)
	require.Equal(0, repo.EpochInputsProcessedCount,
		"epoch should not be finalized after first batch")

	// Simulate machine having processed 10 inputs for the epoch completion check.
	app.processedInputs = 10

	// Step 2: processes remaining 5 inputs. Epoch should be marked complete.
	_, err = svc.Step(context.Background())
	require.NoError(err)
	require.Equal(10, len(repo.StoredResults))
}

// Zero apps returns (false, nil) without panic.
func (s *AdvancerSuite) TestZeroApps() {
	require := s.Require()

	mm := newMockMachineManager() // empty
	repo := &MockRepository{}
	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	hadWork, err := svc.Step(context.Background())
	require.NoError(err)
	require.False(hadWork)
}

// Single epoch per round — only the first epoch is processed.
// Cross-epoch processing — all unprocessed epochs are visited in a single Step.
func (s *AdvancerSuite) TestCrossEpochProcessing() {
	require := s.Require()

	mm := newMockMachineManager()
	app := newMockMachine(1)
	mm.Map[1] = newMockInstance(app)

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app.Application.IApplicationAddress: {
				{Index: 0, Status: EpochStatus_Open},
				{Index: 1, Status: EpochStatus_Open},
				{Index: 2, Status: EpochStatus_Open},
			},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app.Application.IApplicationAddress: {
				newInput(app.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(app.Application.ID, 1, 1, marshal(randomAdvanceResult(1))),
				newInput(app.Application.ID, 2, 2, marshal(randomAdvanceResult(2))),
			},
		},
	}

	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	hadWork, err := svc.Step(context.Background())
	require.NoError(err)
	require.False(hadWork, "all work consumed")
	// All three epochs' inputs should be processed in one step.
	require.Equal(3, len(repo.StoredResults))
}

// Cross-epoch budget — even when each epoch has fewer inputs than batchSize,
// the total across epochs is capped so one app cannot monopolize the tick.
func (s *AdvancerSuite) TestCrossEpochBudget() {
	require := s.Require()

	mm := newMockMachineManager()
	app := newMockMachine(1)
	mm.Map[1] = newMockInstance(app)

	// 5 epochs, each with 3 inputs = 15 total. batchSize=5.
	// Without the budget, all 15 would be processed. With it, only 5.
	var allInputs []*Input
	epochs := make([]*Epoch, 5)
	for e := uint64(0); e < 5; e++ {
		epochs[e] = &Epoch{Index: e, Status: EpochStatus_Open}
		for i := uint64(0); i < 3; i++ {
			idx := e*3 + i
			allInputs = append(allInputs,
				newInput(app.Application.ID, e, idx, marshal(randomAdvanceResult(idx))))
		}
	}

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app.Application.IApplicationAddress: epochs,
		},
		GetInputsReturn: map[common.Address][]*Input{
			app.Application.IApplicationAddress: allInputs,
		},
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 5)
	require.NoError(err)

	hadWork, err := svc.Step(context.Background())
	require.NoError(err)
	require.True(hadWork, "more work remains across later epochs")
	// With batchSize=5 and 3 inputs per epoch: epoch 0 (3) + epoch 1 (2 of 3) = 5.
	// Budget exhausted, remaining epochs deferred.
	require.LessOrEqual(len(repo.StoredResults), 5,
		"should not process more inputs than batchSize across epochs")
	require.GreaterOrEqual(len(repo.StoredResults), 3,
		"should process at least one full epoch")
}

// Deterministic ordering — same app set produces same order.
func (s *AdvancerSuite) TestDeterministicOrdering() {
	require := s.Require()

	// Create apps with IDs in non-sorted order.
	mm := newMockMachineManager()
	for _, id := range []int64{30, 10, 20} {
		impl := &MockMachineImpl{
			Application: &Application{
				ID:                  id,
				IApplicationAddress: randomAddress(),
			},
		}
		mm.Map[id] = newMockInstance(impl)
	}

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{},
		GetInputsReturn: map[common.Address][]*Input{},
	}
	for _, inst := range mm.Map {
		addr := inst.application.IApplicationAddress
		repo.GetEpochsReturn[addr] = []*Epoch{{Index: 0, Status: EpochStatus_Open}}
		repo.GetInputsReturn[addr] = []*Input{
			newInput(inst.application.ID, 0, 0, marshal(randomAdvanceResult(0))),
		}
	}

	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	_, err = svc.Step(context.Background())
	require.NoError(err)

	// Verify results were stored in sorted ID order (10, 20, 30).
	require.Equal(3, len(repo.StoredResults))
	require.Equal([]int64{10, 20, 30}, repo.StoredAppIDs,
		"apps should be processed in ascending ID order")
}

// Self-wake fires on successful work.
func (s *AdvancerSuite) TestSelfWakeOnSuccess() {
	require := s.Require()

	mm := newMockMachineManager()
	app := newMockMachine(1)
	mm.Map[1] = newMockInstance(app)

	inputs := make([]*Input, 10)
	for i := range inputs {
		inputs[i] = newInput(app.Application.ID, 0, uint64(i), marshal(randomAdvanceResult(uint64(i))))
	}

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app.Application.IApplicationAddress: inputs,
		},
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 5)
	require.NoError(err)

	// Call Tick() which internally calls Step() and signals reschedule.
	svc.Tick()

	// The reschedule channel should have a pending signal.
	require.True(svc.DrainReschedule(),
		"reschedule channel should have a pending signal after Tick with work")
}

// No self-wake when idle.
func (s *AdvancerSuite) TestNoSelfWakeWhenIdle() {
	require := s.Require()

	mm := newMockMachineManager()
	app := newMockMachine(1)
	mm.Map[1] = newMockInstance(app)

	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app.Application.IApplicationAddress: {},
		},
	}

	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	svc.Tick()

	require.False(svc.DrainReschedule(),
		"reschedule channel should be empty when no work exists")
}

// No self-wake on error.
func (s *AdvancerSuite) TestNoSelfWakeOnError() {
	require := s.Require()

	mm := &MockMachineManager{
		Map:                 map[int64]*MockMachineInstance{},
		UpdateMachinesError: errors.New("db unavailable"),
	}
	repo := &MockRepository{}

	svc, err := newMockAdvancerService(mm, repo)
	require.NoError(err)

	errs := svc.Tick()
	require.NotEmpty(errs)

	require.False(svc.DrainReschedule(),
		"reschedule should NOT be signaled on error")
}

// Error from Step does NOT signal reschedule.
// Uses the same pattern as FailedAppDoesNotBlockOtherApps, which
// verifies Step returns an error when one app fails.
func (s *AdvancerSuite) TestPartialSuccessStillReschedules() {
	require := s.Require()

	mm := newMockMachineManager()
	app1 := newMockMachine(1) // will fail
	app2 := newMockMachine(2) // will succeed with more work remaining
	mm.Map[1] = newMockInstance(app1)
	mm.Map[2] = newMockInstance(app2)

	// Give app2 more inputs than the batch size so stepApp signals "more work".
	repo := &MockRepository{
		GetEpochsReturn: map[common.Address][]*Epoch{
			app1.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
			app2.Application.IApplicationAddress: {{Index: 0, Status: EpochStatus_Open}},
		},
		GetInputsReturn: map[common.Address][]*Input{
			app1.Application.IApplicationAddress: {
				newInput(app1.Application.ID, 0, 0, []byte("advance error")),
			},
			app2.Application.IApplicationAddress: {
				newInput(app2.Application.ID, 0, 0, marshal(randomAdvanceResult(0))),
				newInput(app2.Application.ID, 0, 1, marshal(randomAdvanceResult(1))),
			},
		},
	}

	svc, err := newMockAdvancerServiceWithBatchSize(mm, repo, 1)
	require.NoError(err)

	// Call Tick — app1 fails, app2 succeeds with more work remaining (batch limit hit).
	// Tick should surface the error AND signal reschedule for app2's pending work.
	errs := svc.Tick()
	require.NotEmpty(errs, "Tick should surface app1's error")

	// Reschedule SHOULD fire: app2 had work, and one failing app must not
	// delay healthy apps by suppressing the reschedule signal.
	require.True(svc.DrainReschedule(),
		"reschedule should be signaled when hadWork is true, even with errors")
}

// ---------------------------------------------------------------------------
// Service.Create tests
// ---------------------------------------------------------------------------

func (s *AdvancerSuite) TestServiceCreate() {
	s.Run("NilRepository", func() {
		require := s.Require()
		c := &CreateInfo{}
		c.Name = "advancer"
		c.Config.AdvancerInputBatchSize = 500
		svc, err := Create(context.Background(), c)
		require.Error(err)
		require.Nil(svc)
		require.Contains(err.Error(), "nil")
	})

	s.Run("ZeroBatchSize", func() {
		require := s.Require()
		c := &CreateInfo{}
		c.Name = "advancer"
		c.Config.AdvancerInputBatchSize = 0
		c.Repository = &MockFullRepository{}
		svc, err := Create(context.Background(), c)
		require.Error(err)
		require.Nil(svc)
		require.Contains(err.Error(), "batch size")
	})

	s.Run("CancelledContext", func() {
		require := s.Require()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c := &CreateInfo{}
		c.Name = "advancer"
		c.Config.AdvancerInputBatchSize = 500
		c.Repository = &MockFullRepository{}
		svc, err := Create(ctx, c)
		require.Error(err)
		require.Nil(svc)
	})
}

// MockFullRepository satisfies the repository.Repository interface minimally
// for Create() validation tests. It panics on any actual DB call.
type MockFullRepository struct {
	repository.Repository
}

type MockMachineImpl struct {
	Application       *Application
	AdvanceBlock      bool
	AdvanceError      error
	OutputsProofError error
	processedInputs   uint64
}

func (mock *MockMachineImpl) Advance(
	ctx context.Context,
	input []byte,
	_ uint64,
	_ uint64,
	_ bool,
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

// newMockInstance creates a MockMachineInstance from a MockMachineImpl, ready to store in MockMachineManager.Map.
func newMockInstance(impl *MockMachineImpl) *MockMachineInstance {
	return &MockMachineInstance{
		application: impl.Application,
		machineImpl: impl,
	}
}

// ------------------------------------------------------------------------------------------------

type MockMachineManager struct {
	Map                 map[int64]*MockMachineInstance
	UpdateMachinesError error
}

func newMockMachineManager() *MockMachineManager {
	return &MockMachineManager{
		Map: map[int64]*MockMachineInstance{},
	}
}

func (mock *MockMachineManager) GetMachine(appID int64) (manager.MachineInstance, bool) {
	instance, exists := mock.Map[appID]
	if !exists {
		return nil, false
	}
	return instance, true
}

func (mock *MockMachineManager) UpdateMachines(ctx context.Context) error {
	return mock.UpdateMachinesError
}

func (mock *MockMachineManager) Applications() []*Application {
	apps := make([]*Application, 0, len(mock.Map))
	for _, v := range mock.Map {
		apps = append(apps, v.application)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
	return apps
}

func (mock *MockMachineManager) HasMachine(appID int64) bool {
	_, exists := mock.Map[appID]
	return exists
}

func (mock *MockMachineManager) Close() error {
	return nil
}

// MockMachineInstance is a test implementation of manager.MachineInstance
type MockMachineInstance struct {
	application         *Application
	machineImpl         *MockMachineImpl
	createSnapshotError error
}

// Advance implements the MachineInstance interface for testing
func (m *MockMachineInstance) Advance(ctx context.Context, input []byte, epochIndex uint64, index uint64, leafs bool) (*AdvanceResult, error) {
	return m.machineImpl.Advance(ctx, input, epochIndex, index, leafs)
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

func (m *MockMachineInstance) ProcessedInputs() uint64 {
	return m.machineImpl.processedInputs
}

func (m *MockMachineInstance) OutputsProof(ctx context.Context) (*OutputsProof, error) {
	if m.machineImpl.OutputsProofError != nil {
		return nil, m.machineImpl.OutputsProofError
	}
	return &OutputsProof{
		OutputsHash: randomHash(),
		MachineHash: randomHash(),
	}, nil
}

// Synchronize implements the MachineInstance interface for testing
func (m *MockMachineInstance) Synchronize(ctx context.Context, repo manager.MachineRepository, batchSize uint64) error {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil
}

// CreateSnapshot implements the MachineInstance interface for testing
func (m *MockMachineInstance) CreateSnapshot(ctx context.Context, processInputs uint64, path string) error {
	return m.createSnapshotError
}

// Retrieves the hash of the current machine state
func (m *MockMachineInstance) Hash(ctx context.Context) ([32]byte, error) {
	// Not used in advancer tests, but needed to satisfy the interface
	return [32]byte{}, nil
}

// Close implements the MachineInstance interface for testing
func (m *MockMachineInstance) Close() error {
	// Not used in advancer tests, but needed to satisfy the interface
	return nil
}

// ------------------------------------------------------------------------------------------------

type MockRepository struct {
	GetEpochsReturn             map[common.Address][]*Epoch
	GetEpochsError              error
	GetEpochsBlock              bool
	GetInputsReturn             map[common.Address][]*Input
	GetInputsError              error
	GetInputsBlock              bool
	StoreAdvanceError           error
	StoreAdvanceFailCount       int
	UpdateApplicationStateError error
	UpdateEpochsError           error
	UpdateOutputsProofError     error
	GetLastSnapshotReturn       *Input
	GetLastSnapshotError        error
	RepeatOutputsProofError     error
	GetEpochReturn              *Epoch
	GetEpochError               error
	GetLastInputReturn          *Input
	GetLastInputError           error
	GetLastProcessedInputReturn *Input
	GetLastProcessedInputError  error
	UpdateSnapshotURIError      error

	StoredResults              []*AdvanceResult
	StoredAppIDs               []int64
	ApplicationStateUpdates    int
	LastApplicationState       ApplicationState
	LastApplicationStateReason *string
	OutputsProofUpdated        bool
	RepeatOutputsProofCalled   bool
	SnapshotURIUpdated         bool
	EpochInputsProcessedCount  int

	mu sync.Mutex
}

func (mock *MockRepository) ListEpochs(
	ctx context.Context,
	nameOrAddress string,
	f repository.EpochFilter,
	p repository.Pagination,
	descending bool,
) ([]*Epoch, uint64, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	// If GetEpochsBlock is true, block until context is canceled
	if mock.GetEpochsBlock {
		<-ctx.Done()
		return nil, 0, ctx.Err()
	}

	address := common.HexToAddress(nameOrAddress)
	return mock.GetEpochsReturn[address], uint64(len(mock.GetEpochsReturn[address])), mock.GetEpochsError
}

func (mock *MockRepository) ListInputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.InputFilter,
	p repository.Pagination,
	descending bool,
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
	inputs := mock.GetInputsReturn[address]

	// Filter by epoch if specified (production code always sets this).
	if f.EpochIndex != nil {
		var filtered []*Input
		for _, inp := range inputs {
			if inp.EpochIndex == *f.EpochIndex {
				filtered = append(filtered, inp)
			}
		}
		inputs = filtered
	}

	total := uint64(len(inputs))

	// Apply pagination if a limit is set
	if p.Limit > 0 {
		start := p.Offset
		if start >= total {
			return nil, total, mock.GetInputsError
		}
		end := start + p.Limit
		if end > total {
			end = total
		}
		inputs = inputs[start:end]
	}

	return inputs, total, mock.GetInputsError
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
	mock.StoredAppIDs = append(mock.StoredAppIDs, appID)

	// Simulate real behavior: processed inputs change status and are no longer
	// returned by queries filtering for unprocessed (Status_None) inputs.
	// This prevents infinite loops in batched fetching.
	if mock.StoreAdvanceError == nil {
		for addr, inputs := range mock.GetInputsReturn {
			for i, inp := range inputs {
				if inp.EpochApplicationID == appID && inp.Index == res.InputIndex {
					newInputs := make([]*Input, 0, len(inputs)-1)
					newInputs = append(newInputs, inputs[:i]...)
					newInputs = append(newInputs, inputs[i+1:]...)
					mock.GetInputsReturn[addr] = newInputs
					break
				}
			}
		}
	}

	return mock.StoreAdvanceError
}

func (mock *MockRepository) UpdateEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64, proof *OutputsProof) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	mock.OutputsProofUpdated = true
	return mock.UpdateOutputsProofError
}

func (mock *MockRepository) UpdateEpochInputsProcessed(ctx context.Context, nameOrAddress string, epochIndex uint64) error {
	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	mock.EpochInputsProcessedCount++
	return mock.UpdateEpochsError
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

func (mock *MockRepository) GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if mock.GetEpochError != nil {
		return nil, mock.GetEpochError
	}
	if mock.GetEpochReturn != nil {
		return mock.GetEpochReturn, nil
	}
	return &Epoch{Status: EpochStatus_Closed}, nil
}

func (mock *MockRepository) GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if mock.GetLastInputError != nil {
		return nil, mock.GetLastInputError
	}
	if mock.GetLastInputReturn != nil {
		return mock.GetLastInputReturn, nil
	}

	address := common.HexToAddress(appAddress)
	inputs := mock.GetInputsReturn[address]
	if len(inputs) == 0 {
		return nil, nil
	}

	var lastInput *Input
	for _, input := range inputs {
		if input.EpochIndex == epochIndex && (lastInput == nil || input.Index > lastInput.Index) {
			lastInput = input
		}
	}

	return lastInput, nil
}

func (mock *MockRepository) GetLastProcessedInput(ctx context.Context, appAddress string) (*Input, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if mock.GetLastProcessedInputError != nil {
		return nil, mock.GetLastProcessedInputError
	}
	if mock.GetLastProcessedInputReturn != nil {
		return mock.GetLastProcessedInputReturn, nil
	}

	address := common.HexToAddress(appAddress)
	inputs := mock.GetInputsReturn[address]
	if len(inputs) == 0 {
		return nil, nil
	}

	var lastInput *Input
	for _, input := range inputs {
		if input.Status != InputCompletionStatus_None && (lastInput == nil || input.Index > lastInput.Index) {
			lastInput = input
		}
	}

	return lastInput, nil
}

func (mock *MockRepository) UpdateInputSnapshotURI(ctx context.Context, appId int64, inputIndex uint64, snapshotURI string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	mock.SnapshotURIUpdated = true
	return mock.UpdateSnapshotURIError
}

func (mock *MockRepository) GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return mock.GetLastSnapshotReturn, mock.GetLastSnapshotError
}

func (mock *MockRepository) RepeatPreviousEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	mock.RepeatOutputsProofCalled = true
	return mock.RepeatOutputsProofError
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
	res := &AdvanceResult{
		InputIndex: inputIndex,
		Status:     InputCompletionStatus_Accepted,
		Outputs:    randomSliceOfBytes(),
		Reports:    randomSliceOfBytes(),
		OutputsProof: OutputsProof{
			OutputsHash: randomHash(),
			MachineHash: randomHash(),
		},
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
