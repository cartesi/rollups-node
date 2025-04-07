// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager/pmutex"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/semaphore"
)

func TestMachineInstance(t *testing.T) {
	suite.Run(t, new(MachineInstanceSuite))
}

type MachineInstanceSuite struct{ suite.Suite }

func (s *MachineInstanceSuite) TestNewMachineInstance() {
	s.Run("Ok", func() {
		require := s.Require()
		app := &model.Application{
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}

		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		// Mock the createMachineRuntime function to avoid actual server creation
		origCreateMachineRuntime := createMachineRuntime
		defer func() { createMachineRuntime = origCreateMachineRuntime }()

		mockRuntime := &MockRollupsMachine{}
		createMachineRuntime = func(ctx context.Context, verbosity MachineLogLevel, app *model.Application,
			logger *slog.Logger, checkHash bool) (rollupsmachine.RollupsMachine, error) {
			return mockRuntime, nil
		}

		machine, err := NewMachineInstance(context.Background(), cartesimachine.MachineLogLevelInfo, app, testLogger, false)
		require.Nil(err)
		require.NotNil(machine)

		// Clean up
		machine.Close()
	})

	s.Run("ErrInvalidAdvanceTimeout", func() {
		require := s.Require()
		app := &model.Application{
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    -1,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}
		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		machine, err := NewMachineInstance(context.Background(), cartesimachine.MachineLogLevelInfo, app, testLogger, false)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidAdvanceTimeout, err)
	})

	s.Run("ErrInvalidInspectTimeout", func() {
		require := s.Require()
		app := &model.Application{
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    -500,
				MaxConcurrentInspects: 3,
			},
		}
		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		machine, err := NewMachineInstance(context.Background(), cartesimachine.MachineLogLevelInfo, app, testLogger, false)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidInspectTimeout, err)
	})

	s.Run("ErrInvalidConcurrentLimit", func() {
		require := s.Require()
		app := &model.Application{
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 0,
			},
		}
		// Create a test logger
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		machine, err := NewMachineInstance(context.Background(), cartesimachine.MachineLogLevelInfo, app, testLogger, false)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidConcurrentLimit, err)
	})

	s.Run("ErrInvalidLogger", func() {
		require := s.Require()
		app := &model.Application{
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}

		machine, err := NewMachineInstance(context.Background(), cartesimachine.MachineLogLevelInfo, app, nil, false)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidLogger, err)
	})
}

func (s *MachineInstanceSuite) TestAdvance() {
	s.Run("Ok", func() {
		s.Run("Accept", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Nil(err)
			require.NotNil(res)

			require.Same(fork, machine.runtime)
			require.Equal(model.InputCompletionStatus_Accepted, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(2), *res.MachineHash)
			require.Equal(uint64(6), machine.processedInputs)
		})

		s.Run("Reject", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			fork.AdvanceAcceptedReturn = false
			fork.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Nil(err)
			require.NotNil(res)

			require.Same(inner, machine.runtime)
			require.Equal(model.InputCompletionStatus_Rejected, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(2), *res.MachineHash)
			require.Equal(uint64(6), machine.processedInputs)
		})

		testSoftError := func(name string, err error, status model.InputCompletionStatus) {
			s.Run(name, func() {
				require := s.Require()
				inner, fork, machine := s.setupAdvance()
				fork.AdvanceError = err
				fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

				res, err := machine.Advance(context.Background(), []byte{}, 5)
				require.Nil(err)
				require.NotNil(res)

				require.Equal(status, res.Status)
				require.Equal(expectedOutputs, res.Outputs)
				require.Equal(expectedReports1, res.Reports)
				require.Equal(newHash(1), res.OutputsHash)
				require.Equal(newHash(2), *res.MachineHash)
				require.Equal(uint64(6), machine.processedInputs)
			})
		}

		testSoftError("Exception",
			rollupsmachine.ErrException,
			model.InputCompletionStatus_Exception)

		testSoftError("Halted",
			rollupsmachine.ErrHalted,
			model.InputCompletionStatus_MachineHalted)

		testSoftError("OutputsLimit",
			rollupsmachine.ErrOutputsLimitExceeded,
			model.InputCompletionStatus_OutputsLimitExceeded)

		testSoftError("CycleLimit",
			rollupsmachine.ErrCycleLimitExceeded,
			model.InputCompletionStatus_CycleLimitExceeded)

		testSoftError("TimeLimit",
			cartesimachine.ErrTimedOut,
			model.InputCompletionStatus_TimeLimitExceeded)

		testSoftError("PayloadLengthLimit",
			rollupsmachine.ErrPayloadLengthLimitExceeded,
			model.InputCompletionStatus_PayloadLengthLimitExceeded)
	})

	s.Run("Error", func() {
		s.Run("Fork", func() {
			require := s.Require()
			inner, _, machine := s.setupAdvance()
			errFork := errors.New("Fork error")
			inner.ForkError = errFork

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Error(err)
			require.Nil(res)
			require.Equal(errFork, err)
			require.Equal(uint64(5), machine.processedInputs)
		})

		s.Run("Advance", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errAdvance := errors.New("Advance error")
			fork.AdvanceError = errAdvance
			fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errAdvance)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs)
		})

		s.Run("AdvanceAndClose", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errAdvance := errors.New("Advance error")
			errClose := errors.New("Close error")
			fork.AdvanceError = errAdvance
			fork.CloseError = errClose
			inner.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errAdvance)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs)
		})

		s.Run("Hash", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errHash := errors.New("Hash error")
			fork.HashError = errHash
			fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errHash)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs)
		})

		s.Run("HashAndClose", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errHash := errors.New("Hash error")
			errClose := errors.New("Close error")
			fork.HashError = errHash
			fork.CloseError = errClose
			inner.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errHash)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs)
		})

		s.Run("Close", func() {
			s.Run("Inner", func() {
				require := s.Require()
				inner, _, machine := s.setupAdvance()
				errClose := errors.New("Close error")
				inner.CloseError = errClose

				res, err := machine.Advance(context.Background(), []byte{}, 5)
				require.Error(err)
				require.Nil(res)
				require.ErrorIs(err, errClose)
				require.NotErrorIs(err, errUnreachable)
				require.Equal(uint64(5), machine.processedInputs)
			})

			s.Run("Fork", func() {
				require := s.Require()
				_, fork, machine := s.setupAdvance()
				errClose := errors.New("Close error")
				fork.AdvanceError = rollupsmachine.ErrException
				fork.CloseError = errClose

				res, err := machine.Advance(context.Background(), []byte{}, 5)
				require.Error(err)
				require.NotNil(res)
				require.ErrorIs(err, errClose)
				require.NotErrorIs(err, errUnreachable)
				require.Equal(uint64(6), machine.processedInputs)
			})
		})
	})

	s.Run("Concurrency", func() {
		// Two Advances cannot be concurrently active.
		s.T().Skip("TODO")
	})
}

func (s *MachineInstanceSuite) TestInspect() {
	s.Run("Ok", func() {
		s.Run("Accept", func() {
			require := s.Require()
			_, fork, machine := s.setupInspect()

			res, err := machine.Inspect(context.Background(), []byte{})
			require.Nil(err)
			require.NotNil(res)

			require.NotSame(fork, machine.runtime)
			require.Equal(uint64(55), res.ProcessedInputs)
			require.True(res.Accepted)
			require.Equal(expectedReports2, res.Reports)
			require.Nil(res.Error)
		})

		s.Run("Reject", func() {
			require := s.Require()
			_, fork, machine := s.setupInspect()
			fork.InspectAcceptedReturn = false

			res, err := machine.Inspect(context.Background(), []byte{})
			require.Nil(err)
			require.NotNil(res)

			require.NotSame(fork, machine.runtime)
			require.Equal(uint64(55), res.ProcessedInputs)
			require.False(res.Accepted)
			require.Equal(expectedReports2, res.Reports)
			require.Nil(res.Error)
		})
	})

	s.Run("Error", func() {
		s.Run("Acquire", func() {
			require := s.Require()
			_, _, machine := s.setupInspect()

			// Set semaphore to 0 to force acquisition failure
			machine.inspectSemaphore.TryAcquire(int64(machine.maxConcurrentInspects))

			ctx, cancel := context.WithTimeout(context.Background(), centisecond)
			defer cancel()

			res, err := machine.Inspect(ctx, []byte{})
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, context.DeadlineExceeded)

			// Release the semaphore for cleanup
			machine.inspectSemaphore.Release(int64(machine.maxConcurrentInspects))
		})

		s.Run("Fork", func() {
			require := s.Require()
			inner, _, machine := s.setupInspect()
			errFork := errors.New("Fork error")
			inner.ForkError = errFork

			res, err := machine.Inspect(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.Equal(errFork, err)
		})

		s.Run("Inspect", func() {
			require := s.Require()
			_, fork, machine := s.setupInspect()
			errInspect := errors.New("Inspect error")
			fork.InspectError = errInspect

			res, err := machine.Inspect(context.Background(), []byte{})
			require.Nil(err)
			require.NotNil(res)
			require.Equal(errInspect, res.Error)
		})

		s.Run("Close", func() {
			require := s.Require()
			_, fork, machine := s.setupInspect()
			errClose := errors.New("Close error")
			fork.CloseError = errClose

			res, err := machine.Inspect(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.Equal(errClose, err)
		})
	})

	s.Run("Concurrency", func() {
		require := s.Require()
		_, _, machine := s.setupInspect()

		// Test that we can run maxConcurrentInspects inspects concurrently
		var wg sync.WaitGroup
		errors := make(chan error, machine.maxConcurrentInspects)

		for range int(machine.maxConcurrentInspects) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := machine.Inspect(context.Background(), []byte{})
				if err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Check if any errors occurred
		for err := range errors {
			require.Nil(err, "Concurrent inspect failed: %v", err)
		}
	})
}

func (s *MachineInstanceSuite) TestClose() {
	s.Run("Ok", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		inner.CloseError = nil

		err := machine.Close()
		require.Nil(err)
		require.Nil(machine.runtime)
	})

	s.Run("Error", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		errClose := errors.New("Close error")
		inner.CloseError = errClose

		err := machine.Close()
		require.Error(err)
		require.Equal(errClose, err)
	})

	s.Run("Concurrency", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		inner.CloseError = nil

		// Start a goroutine that tries to advance while we're closing
		done := make(chan struct{})
		go func() {
			defer close(done)

			// Small delay to ensure Close has a chance to start
			time.Sleep(centisecond / 2)

			// This should block until Close is done
			_, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Error(err)
			require.Equal(ErrMachineClosed, err)
		}()

		// Close the machine
		err := machine.Close()
		require.Nil(err)

		// Wait for the advance goroutine to finish
		select {
		case <-done:
			// Good, it completed
		case <-time.After(decisecond):
			require.Fail("Advance did not complete after Close")
		}
	})
}

// ------------------------------------------------------------------------------------------------

// MockMachineInstance implements the MachineInstance interface for testing
type MockMachineInstance struct {
	application *model.Application
}

func (m *MockMachineInstance) Application() *model.Application {
	return m.application
}

func (m *MockMachineInstance) Advance(ctx context.Context, input []byte, index uint64) (*model.AdvanceResult, error) {
	return nil, nil
}

func (m *MockMachineInstance) Inspect(ctx context.Context, query []byte) (*model.InspectResult, error) {
	return nil, nil
}

func (m *MockMachineInstance) Synchronize(ctx context.Context, repo MachineRepository) error {
	return nil
}

func (m *MockMachineInstance) Close() error {
	return nil
}

// ------------------------------------------------------------------------------------------------

var (
	errUnreachable  = errors.New("unreachable")
	expectedOutputs = []rollupsmachine.Output{
		newBytes(11, 100),
		newBytes(12, 100),
		newBytes(13, 100),
	}
	expectedReports1 = []rollupsmachine.Report{
		newBytes(21, 200),
		newBytes(22, 200),
	}
	expectedReports2 = []rollupsmachine.Report{
		newBytes(31, 300),
		newBytes(32, 300),
		newBytes(33, 300),
		newBytes(34, 300),
	}
)

func (s *MachineInstanceSuite) setupAdvance() (*MockRollupsMachine, *MockRollupsMachine, *MachineInstanceImpl) {
	app := &model.Application{
		ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline:    decisecond,
			InspectMaxDeadline:    centisecond,
			MaxConcurrentInspects: 3,
		},
	}
	inner := &MockRollupsMachine{}
	machine := &MachineInstanceImpl{
		application:           app,
		runtime:               inner,
		processedInputs:       5,
		advanceTimeout:        decisecond,
		inspectTimeout:        centisecond,
		maxConcurrentInspects: 3,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(3),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	fork := &MockRollupsMachine{}

	inner.ForkReturn = fork
	inner.CloseError = nil

	fork.AdvanceAcceptedReturn = true
	fork.AdvanceOutputsReturn = []rollupsmachine.Output{
		newBytes(11, 100),
		newBytes(12, 100),
		newBytes(13, 100),
	}
	fork.AdvanceReportsReturn = []rollupsmachine.Report{
		newBytes(21, 200),
		newBytes(22, 200),
	}
	fork.AdvanceHashReturn = newHash(1)
	fork.AdvanceError = nil

	fork.HashReturn = newHash(2)
	fork.HashError = nil

	fork.InspectAcceptedReturn = true
	fork.InspectReportsReturn = []rollupsmachine.Report{
		newBytes(31, 300),
		newBytes(32, 300),
		newBytes(33, 300),
		newBytes(34, 300),
	}
	fork.InspectError = errUnreachable

	fork.CloseError = errUnreachable

	return inner, fork, machine
}

func (s *MachineInstanceSuite) setupInspect() (*MockRollupsMachine, *MockRollupsMachine, *MachineInstanceImpl) {
	app := &model.Application{
		ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline:    decisecond,
			InspectMaxDeadline:    centisecond,
			MaxConcurrentInspects: 3,
		},
	}
	inner := &MockRollupsMachine{}
	machine := &MachineInstanceImpl{
		application:           app,
		runtime:               inner,
		processedInputs:       55,
		advanceTimeout:        decisecond,
		inspectTimeout:        centisecond,
		maxConcurrentInspects: 3,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(3),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	fork := &MockRollupsMachine{}

	inner.ForkReturn = fork
	inner.CloseError = errUnreachable

	fork.AdvanceError = errUnreachable
	fork.HashError = errUnreachable

	fork.InspectAcceptedReturn = true
	fork.InspectReportsReturn = []rollupsmachine.Report{
		newBytes(31, 300),
		newBytes(32, 300),
		newBytes(33, 300),
		newBytes(34, 300),
	}
	fork.InspectError = nil

	fork.CloseError = nil

	return inner, fork, machine
}

// ------------------------------------------------------------------------------------------------

const (
	centisecond = 10 * time.Millisecond
	decisecond  = 100 * time.Millisecond
)

func newHash(n byte) common.Hash {
	hash := rollupsmachine.Hash{}
	for i := range 32 {
		hash[i] = n
	}
	return hash
}

func newBytes(n byte, size int) []byte {
	bytes := make([]byte, size)
	for i := range size {
		bytes[i] = n
	}
	return bytes
}

// ------------------------------------------------------------------------------------------------

type MockRollupsMachine struct {
	ForkReturn rollupsmachine.RollupsMachine
	ForkError  error

	HashReturn rollupsmachine.Hash
	HashError  error

	AdvanceAcceptedReturn bool
	AdvanceOutputsReturn  []rollupsmachine.Output
	AdvanceReportsReturn  []rollupsmachine.Report
	AdvanceHashReturn     rollupsmachine.Hash
	AdvanceError          error

	InspectAcceptedReturn bool
	InspectReportsReturn  []rollupsmachine.Report
	InspectError          error

	CloseError error
}

func (machine *MockRollupsMachine) Fork(_ context.Context) (rollupsmachine.RollupsMachine, error) {
	return machine.ForkReturn, machine.ForkError
}

func (machine *MockRollupsMachine) Hash(_ context.Context) (rollupsmachine.Hash, error) {
	return machine.HashReturn, machine.HashError
}

func (machine *MockRollupsMachine) OutputsHash(_ context.Context) (rollupsmachine.Hash, error) {
	return machine.AdvanceHashReturn, machine.HashError
}

func (machine *MockRollupsMachine) Advance(_ context.Context, input []byte) (
	bool, []rollupsmachine.Output, []rollupsmachine.Report, rollupsmachine.Hash, error,
) {
	return machine.AdvanceAcceptedReturn,
		machine.AdvanceOutputsReturn,
		machine.AdvanceReportsReturn,
		machine.AdvanceHashReturn,
		machine.AdvanceError
}

func (machine *MockRollupsMachine) Inspect(_ context.Context,
	query []byte,
) (bool, []rollupsmachine.Report, error) {
	return machine.InspectAcceptedReturn, machine.InspectReportsReturn, machine.InspectError
}

func (machine *MockRollupsMachine) Close(_ context.Context) error {
	return machine.CloseError
}
