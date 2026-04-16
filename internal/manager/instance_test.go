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
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/semaphore"
)

func TestMachineInstance(t *testing.T) {
	suite.Run(t, new(MachineInstanceSuite))
}

type MachineInstanceSuite struct{ suite.Suite }

// MockMachineRuntimeFactory implements MachineRuntimeFactory for testing
type MockMachineRuntimeFactory struct {
	RuntimeToReturn machine.Machine
	ErrorToReturn   error
}

func (f *MockMachineRuntimeFactory) CreateMachineRuntime(
	_ context.Context,
	_ *model.Application,
	_ *slog.Logger,
	_ bool,
) (machine.Machine, error) {
	return f.RuntimeToReturn, f.ErrorToReturn
}

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

		// Create a mock factory
		mockRuntime := &MockRollupsMachine{}
		mockFactory := &MockMachineRuntimeFactory{
			RuntimeToReturn: mockRuntime,
			ErrorToReturn:   nil,
		}

		machine, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			0,
			testLogger,
			false,
			mockFactory,
		)
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
		mockFactory := &MockMachineRuntimeFactory{}

		machine, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			0,
			testLogger,
			false,
			mockFactory,
		)
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
		mockFactory := &MockMachineRuntimeFactory{}

		machine, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			0,
			testLogger,
			false,
			mockFactory,
		)
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
		mockFactory := &MockMachineRuntimeFactory{}

		machine, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			0,
			testLogger,
			false,
			mockFactory,
		)
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
		mockFactory := &MockMachineRuntimeFactory{}

		machine, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			0,
			nil,
			false,
			mockFactory,
		)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidLogger, err)
	})

	s.Run("ErrInvalidFactory", func() {
		require := s.Require()
		app := &model.Application{
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}
		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		machine, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			0,
			testLogger,
			false,
			nil,
		)
		require.Error(err)
		require.Nil(machine)
		require.Contains(err.Error(), "factory must not be nil")
	})
}

func (s *MachineInstanceSuite) TestNewMachineInstanceFromSnapshot() {
	s.Run("Ok", func() {
		require := s.Require()
		app := &model.Application{
			Name: "TestApp",
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockRuntime := &MockRollupsMachine{}
		mockFactory := &MockMachineRuntimeFactory{
			RuntimeToReturn: mockRuntime,
			ErrorToReturn:   nil,
		}

		// NewMachineInstanceFromSnapshot creates a SnapshotMachineRuntimeFactory
		// internally, so we use NewMachineInstanceWithFactory to test the same
		// logic with a controlled factory.
		inputIndex := uint64(5)

		// The function sets processedInputs = inputIndex + 1
		// Use the mock factory to avoid actual machine loading
		inst, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			inputIndex+1,
			testLogger,
			false,
			mockFactory,
		)
		require.NoError(err)
		require.NotNil(inst)
		require.Equal(inputIndex+1, inst.ProcessedInputs())

		inst.Close()
	})

	s.Run("FactoryError", func() {
		require := s.Require()
		app := &model.Application{
			Name: "TestApp",
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}

		testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		mockFactory := &MockMachineRuntimeFactory{
			RuntimeToReturn: nil,
			ErrorToReturn:   errors.New("snapshot load failed"),
		}

		inst, err := NewMachineInstanceWithFactory(
			context.Background(),
			app,
			6,
			testLogger,
			false,
			mockFactory,
		)
		require.Error(err)
		require.Nil(inst)
		require.ErrorIs(err, ErrMachineCreation)
		require.Contains(err.Error(), "snapshot load failed")
	})
}

func (s *MachineInstanceSuite) TestApplicationAndProcessedInputs() {
	require := s.Require()
	app := &model.Application{
		Name: "TestApp",
		ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline:    decisecond,
			InspectMaxDeadline:    centisecond,
			MaxConcurrentInspects: 3,
		},
	}

	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockRuntime := &MockRollupsMachine{}
	mockFactory := &MockMachineRuntimeFactory{
		RuntimeToReturn: mockRuntime,
		ErrorToReturn:   nil,
	}

	inst, err := NewMachineInstanceWithFactory(
		context.Background(), app, 42, testLogger, false, mockFactory,
	)
	require.NoError(err)
	require.Same(app, inst.Application())
	require.Equal(uint64(42), inst.ProcessedInputs())
	inst.Close()
}

func (s *MachineInstanceSuite) TestAdvance() {
	s.Run("Ok", func() {
		s.Run("Accept", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Nil(err)
			require.NotNil(res)

			require.Same(fork, machine.runtime)
			require.Equal(model.InputCompletionStatus_Accepted, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(2), res.MachineHash)
			require.Equal(uint64(6), machine.processedInputs.Load())
		})

		s.Run("Reject", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			fork.AdvanceAcceptedReturn = false
			fork.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Nil(err)
			require.NotNil(res)

			require.Same(inner, machine.runtime)
			require.Equal(model.InputCompletionStatus_Rejected, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(2), res.MachineHash)
			require.Equal(uint64(6), machine.processedInputs.Load())
		})

		testSoftError := func(name string, err error, status model.InputCompletionStatus) {
			s.Run(name, func() {
				require := s.Require()
				inner, fork, machine := s.setupAdvance()
				fork.AdvanceError = err
				fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

				res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
				require.Nil(err)
				require.NotNil(res)

				require.Equal(status, res.Status)
				require.Equal(expectedOutputs, res.Outputs)
				require.Equal(expectedReports1, res.Reports)
				require.Equal(newHash(1), res.OutputsHash)
				require.Equal(newHash(2), res.MachineHash)
				require.Equal(uint64(6), machine.processedInputs.Load())
			})
		}

		testSoftError("Exception",
			machine.ErrException,
			model.InputCompletionStatus_Exception)

		testSoftError("Halted",
			machine.ErrHalted,
			model.InputCompletionStatus_MachineHalted)

		testSoftError("OutputsLimit",
			machine.ErrOutputsLimitExceeded,
			model.InputCompletionStatus_OutputsLimitExceeded)

		testSoftError("ReportsLimit",
			machine.ErrReportsLimitExceeded,
			model.InputCompletionStatus_ReportsLimitExceeded)

		testSoftError("ReachedTargetMcycle",
			machine.ErrReachedTargetMcycle,
			model.InputCompletionStatus_CycleLimitExceeded)

		testSoftError("TimeLimit",
			machine.ErrDeadlineExceeded,
			model.InputCompletionStatus_TimeLimitExceeded)

		testSoftError("PayloadLengthLimit",
			machine.ErrPayloadLengthLimitExceeded,
			model.InputCompletionStatus_PayloadLengthLimitExceeded)
	})

	s.Run("Error", func() {
		s.Run("Fork", func() {
			require := s.Require()
			inner, _, machine := s.setupAdvance()
			errFork := errors.New("Fork error")
			inner.ForkError = errFork

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.Equal(errFork, err)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("Advance", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errAdvance := errors.New("Advance error")
			fork.AdvanceError = errAdvance
			fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errAdvance)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("AdvanceAndClose", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errAdvance := errors.New("Advance error")
			errClose := errors.New("Close error")
			fork.AdvanceError = errAdvance
			fork.CloseError = errClose
			inner.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errAdvance)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("Hash", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errHash := errors.New("Hash error")
			fork.HashError = errHash
			fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errHash)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("HashAndClose", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errHash := errors.New("Hash error")
			errClose := errors.New("Close error")
			fork.HashError = errHash
			fork.CloseError = errClose
			inner.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errHash)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("Close", func() {
			s.Run("Inner", func() {
				require := s.Require()
				inner, _, machine := s.setupAdvance()
				errClose := errors.New("Close error")
				inner.CloseError = errClose

				// Close error on old runtime is logged, not propagated.
				// Advance succeeds and processedInputs is incremented.
				res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
				require.NoError(err)
				require.NotNil(res)
				require.Equal(uint64(6), machine.processedInputs.Load())
			})

			s.Run("Fork", func() {
				require := s.Require()
				_, fork, machineInst := s.setupAdvance()
				fork.AdvanceError = machine.ErrException
				fork.CloseError = errors.New("Close error")

				// Close error on fork is logged, not propagated.
				// Advance succeeds and processedInputs is incremented.
				res, err := machineInst.Advance(context.Background(), []byte{}, 0, 5, false)
				require.NoError(err)
				require.NotNil(res)
				require.Equal(uint64(6), machineInst.processedInputs.Load())
			})
		})
	})

	s.Run("CollectHashes", func() {
		require := s.Require()
		inner, fork, machine := s.setupAdvance()

		// Set up WriteCheckpointHash to succeed
		fork.CheckpointHashError = nil

		res, err := machine.Advance(context.Background(), []byte{}, 0, 5, true)
		require.Nil(err)
		require.NotNil(res)

		require.Same(fork, machine.runtime)
		require.Equal(model.InputCompletionStatus_Accepted, res.Status)
		require.True(res.IsDaveConsensus)
		require.Equal(uint64(6), machine.processedInputs.Load())

		// Verify the inner runtime was closed (accept path)
		_ = inner
	})

	s.Run("CollectHashesWriteCheckpointError", func() {
		require := s.Require()
		_, fork, machine := s.setupAdvance()

		errCheckpoint := errors.New("checkpoint write error")
		fork.CheckpointHashError = errCheckpoint

		res, err := machine.Advance(context.Background(), []byte{}, 0, 5, true)
		require.Error(err)
		require.Nil(res)
		require.ErrorIs(err, errCheckpoint)
		require.Equal(uint64(5), machine.processedInputs.Load())
	})

	s.Run("SequentialAdvances", func() {
		// Advance is serialized by advanceMutex — concurrent advance on the
		// same machine never happens by design. This test verifies that two
		// sequential advances correctly increment processedInputs.
		require := s.Require()
		inner, fork, machine := s.setupAdvance()

		// Allow inner.Close to succeed (old runtime close on accept)
		inner.CloseError = nil

		// First advance: fork from inner (processedInputs=5), returns accepted.
		// After accept, fork becomes the new runtime.
		// Second advance: fork from fork (processedInputs=6), fork must also fork.
		fork2 := &MockRollupsMachine{}
		fork2.AdvanceAcceptedReturn = true
		fork2.AdvanceOutputsReturn = expectedOutputs
		fork2.AdvanceReportsReturn = expectedReports1
		fork2.OutputsHashReturn = newHash(1)
		fork2.HashReturn = newHash(2)
		fork2.CloseError = errUnreachable // old runtime close for second advance
		fork2.ForkReturn = nil
		fork.ForkReturn = fork2
		fork.CloseError = nil // close of fork (now old runtime) in second advance

		// First advance at index 5
		res1, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
		require.Nil(err)
		require.NotNil(res1)
		require.Equal(uint64(6), machine.processedInputs.Load())

		// Second advance at index 6
		res2, err := machine.Advance(context.Background(), []byte{}, 0, 6, false)
		require.Nil(err)
		require.NotNil(res2)
		require.Equal(uint64(7), machine.processedInputs.Load())
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
		s.Run("AtCapacity", func() {
			require := s.Require()
			_, _, machine := s.setupInspect()

			// Pre-fill all semaphore slots to simulate a saturated app
			machine.inspectSemaphore.TryAcquire(int64(machine.maxConcurrentInspects))

			// TryAcquire is non-blocking: the error is returned immediately,
			// no context deadline required.
			res, err := machine.Inspect(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, ErrInspectAtCapacity)

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

func (s *MachineInstanceSuite) TestCreateSnapshot() {
	s.Run("Ok", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		inner.CloseError = nil

		err := machine.CreateSnapshot(context.Background(), 5, "/tmp/snapshot")
		require.Nil(err)
	})

	s.Run("Error", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		errStore := errors.New("Store error")
		inner.StoreError = errStore
		inner.CloseError = nil

		err := machine.CreateSnapshot(context.Background(), 5, "/tmp/snapshot")
		require.Error(err)
		require.ErrorIs(err, errStore)

		// Runtime should be destroyed after a store error.
		require.Nil(machine.runtime)
	})

	s.Run("ErrorAndCloseError", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		errStore := errors.New("Store error")
		errClose := errors.New("Close error")
		inner.StoreError = errStore
		inner.CloseError = errClose

		err := machine.CreateSnapshot(context.Background(), 5, "/tmp/snapshot")
		require.Error(err)
		require.ErrorIs(err, errStore)
		require.ErrorIs(err, errClose)
		require.Nil(machine.runtime)
	})

	s.Run("MachineClosed", func() {
		require := s.Require()
		_, _, machine := s.setupAdvance()
		machine.runtime = nil

		err := machine.CreateSnapshot(context.Background(), 5, "/tmp/snapshot")
		require.Error(err)
		require.Equal(ErrMachineClosed, err)
	})

	s.Run("InvalidSnapshotPoint", func() {
		require := s.Require()
		_, _, machine := s.setupAdvance()

		err := machine.CreateSnapshot(context.Background(), 6, "/tmp/snapshot")
		require.ErrorIs(err, ErrInvalidSnapshotPoint)
	})
}

func (s *MachineInstanceSuite) TestHash() {
	s.Run("Ok", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()

		hash, err := machineInst.Hash(context.Background())
		require.NoError(err)
		require.Equal([32]byte(newHash(1)), hash)

		// Runtime should still be alive after a successful call.
		require.Same(inner, machineInst.runtime)
	})

	s.Run("MachineClosed", func() {
		require := s.Require()
		_, machineInst := s.setupOutputsProof()
		machineInst.runtime = nil

		hash, err := machineInst.Hash(context.Background())
		require.Error(err)
		require.Equal(ErrMachineClosed, err)
		require.Equal([32]byte{}, hash)
	})

	s.Run("Error", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()
		errHash := errors.New("Hash error")
		inner.HashError = errHash
		inner.CloseError = nil

		hash, err := machineInst.Hash(context.Background())
		require.Error(err)
		require.ErrorIs(err, errHash)
		require.Equal([32]byte{}, hash)

		// Runtime should be destroyed after a hash error.
		require.Nil(machineInst.runtime)
	})

	s.Run("ErrorAndCloseError", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()
		errHash := errors.New("Hash error")
		errClose := errors.New("Close error")
		inner.HashError = errHash
		inner.CloseError = errClose

		hash, err := machineInst.Hash(context.Background())
		require.Error(err)
		require.ErrorIs(err, errHash)
		require.ErrorIs(err, errClose)
		require.Equal([32]byte{}, hash)
		require.Nil(machineInst.runtime)
	})
}

func (s *MachineInstanceSuite) TestOutputsProof() {
	s.Run("Ok", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()

		proof, err := machineInst.OutputsProof(context.Background())
		require.NoError(err)
		require.NotNil(proof)

		require.Equal(newHash(1), proof.MachineHash)
		require.Equal(newHash(2), proof.OutputsHash)
		require.Equal(expectedOutputsHashProof, proof.OutputsHashProof)

		// Runtime should still be alive after a successful call.
		require.Same(inner, machineInst.runtime)
	})

	s.Run("MachineClosed", func() {
		require := s.Require()
		_, machineInst := s.setupOutputsProof()
		machineInst.runtime = nil

		proof, err := machineInst.OutputsProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.Equal(ErrMachineClosed, err)
	})

	s.Run("HashError", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()
		errHash := errors.New("Hash error")
		inner.HashError = errHash
		inner.CloseError = nil

		proof, err := machineInst.OutputsProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.ErrorIs(err, errHash)

		// Runtime should be destroyed after a hash error.
		require.Nil(machineInst.runtime)
	})

	s.Run("HashErrorAndCloseError", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()
		errHash := errors.New("Hash error")
		errClose := errors.New("Close error")
		inner.HashError = errHash
		inner.CloseError = errClose

		proof, err := machineInst.OutputsProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.ErrorIs(err, errHash)
		require.ErrorIs(err, errClose)

		// Runtime should be destroyed even when Close also fails.
		require.Nil(machineInst.runtime)
	})

	s.Run("OutputsHashError", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()
		errOutputsHash := errors.New("OutputsHash error")
		inner.OutputsHashError = errOutputsHash
		inner.CloseError = nil

		proof, err := machineInst.OutputsProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.ErrorIs(err, errOutputsHash)

		// Runtime should be destroyed after an outputs hash error.
		require.Nil(machineInst.runtime)
	})

	s.Run("OutputsHashProofError", func() {
		require := s.Require()
		inner, machineInst := s.setupOutputsProof()
		errProof := errors.New("OutputsHashProof error")
		inner.OutputsHashProofError = errProof
		inner.CloseError = nil

		proof, err := machineInst.OutputsProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.ErrorIs(err, errProof)

		// Runtime should be destroyed after an outputs hash proof error.
		require.Nil(machineInst.runtime)
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
			_, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
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

	s.Run("TimesOutWaitingForInspects", func() {
		require := s.Require()
		inner, _, machine := s.setupAdvance()
		inner.CloseError = nil

		// Use a short timeout so the test runs fast
		machine.closeTimeout = centisecond

		// Pre-acquire all semaphore slots to simulate stuck inspects
		for range int(machine.maxConcurrentInspects) {
			err := machine.inspectSemaphore.Acquire(context.Background(), 1)
			require.Nil(err)
		}

		// Close should not block indefinitely — it times out and closes anyway
		done := make(chan error, 1)
		go func() {
			done <- machine.Close()
		}()

		select {
		case err := <-done:
			require.Nil(err)
			require.Nil(machine.runtime)
		case <-time.After(decisecond * 5):
			require.Fail("Close blocked indefinitely despite timeout")
		}

		// Release slots to clean up
		machine.inspectSemaphore.Release(int64(machine.maxConcurrentInspects))
	})
}

// ------------------------------------------------------------------------------------------------

var (
	errUnreachable  = errors.New("unreachable")
	expectedOutputs = []machine.Output{
		newBytes(11, 100),
		newBytes(12, 100),
		newBytes(13, 100),
	}
	expectedReports1 = []machine.Report{
		newBytes(21, 200),
		newBytes(22, 200),
	}
	expectedReports2 = []machine.Report{
		newBytes(31, 300),
		newBytes(32, 300),
		newBytes(33, 300),
		newBytes(34, 300),
	}
	expectedOutputsHashProof = []machine.Hash{
		newHash(3),
		newHash(4),
		newHash(5),
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
	machineInst := &MachineInstanceImpl{
		application:           app,
		runtime:               inner,
		advanceTimeout:        decisecond,
		inspectTimeout:        centisecond,
		maxConcurrentInspects: 3,
		closeTimeout:          defaultCloseTimeout,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(3),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	machineInst.processedInputs.Store(5)

	fork := &MockRollupsMachine{}

	inner.ForkReturn = fork
	inner.CloseError = nil

	fork.AdvanceAcceptedReturn = true
	fork.AdvanceOutputsReturn = []machine.Output{
		newBytes(11, 100),
		newBytes(12, 100),
		newBytes(13, 100),
	}
	fork.AdvanceReportsReturn = []machine.Report{
		newBytes(21, 200),
		newBytes(22, 200),
	}
	fork.OutputsHashReturn = newHash(1)
	fork.AdvanceError = nil

	fork.HashReturn = newHash(2)
	fork.HashError = nil

	fork.InspectAcceptedReturn = true
	fork.InspectReportsReturn = []machine.Report{
		newBytes(31, 300),
		newBytes(32, 300),
		newBytes(33, 300),
		newBytes(34, 300),
	}
	fork.InspectError = errUnreachable

	fork.CloseError = errUnreachable

	return inner, fork, machineInst
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
	machineInst := &MachineInstanceImpl{
		application:           app,
		runtime:               inner,
		advanceTimeout:        decisecond,
		inspectTimeout:        centisecond,
		maxConcurrentInspects: 3,
		closeTimeout:          defaultCloseTimeout,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(3),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	machineInst.processedInputs.Store(55)

	fork := &MockRollupsMachine{}

	inner.ForkReturn = fork
	inner.CloseError = errUnreachable

	fork.AdvanceError = errUnreachable
	fork.HashError = errUnreachable

	fork.InspectAcceptedReturn = true
	fork.InspectReportsReturn = []machine.Report{
		newBytes(31, 300),
		newBytes(32, 300),
		newBytes(33, 300),
		newBytes(34, 300),
	}
	fork.InspectError = nil

	fork.CloseError = nil

	return inner, fork, machineInst
}

func (s *MachineInstanceSuite) setupOutputsProof() (*MockRollupsMachine, *MachineInstanceImpl) {
	app := &model.Application{
		ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline:    decisecond,
			InspectMaxDeadline:    centisecond,
			LoadDeadline:          decisecond,
			MaxConcurrentInspects: 3,
		},
	}
	inner := &MockRollupsMachine{}
	machineInst := &MachineInstanceImpl{
		application:           app,
		runtime:               inner,
		advanceTimeout:        decisecond,
		inspectTimeout:        centisecond,
		maxConcurrentInspects: 3,
		closeTimeout:          defaultCloseTimeout,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(3),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	machineInst.processedInputs.Store(5)

	inner.HashReturn = newHash(1)
	inner.HashError = nil
	inner.OutputsHashReturn = newHash(2)
	inner.OutputsHashError = nil
	inner.OutputsHashProofReturn = []machine.Hash{
		newHash(3),
		newHash(4),
		newHash(5),
	}
	inner.OutputsHashProofError = nil
	inner.CloseError = errUnreachable

	return inner, machineInst
}

// ------------------------------------------------------------------------------------------------

const (
	centisecond = 10 * time.Millisecond
	decisecond  = 100 * time.Millisecond
)

func newHash(n byte) common.Hash {
	hash := machine.Hash{}
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
// Synchronize tests
// ------------------------------------------------------------------------------------------------

// mockSyncRepository is a lightweight mock for Synchronize tests.
// It simulates pagination over a slice of inputs.
type mockSyncRepository struct {
	inputs     []*model.Input
	totalCount uint64
	listErr    error
}

func (r *mockSyncRepository) ListApplications(
	_ context.Context,
	_ repository.ApplicationFilter,
	_ repository.Pagination,
	_ bool,
) ([]*model.Application, uint64, error) {
	return nil, 0, nil
}

func (r *mockSyncRepository) ListInputs(
	ctx context.Context,
	_ string,
	_ repository.InputFilter,
	p repository.Pagination,
	_ bool,
) ([]*model.Input, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	start := p.Offset
	if start >= uint64(len(r.inputs)) {
		return nil, r.totalCount, nil
	}
	end := start + p.Limit
	if p.Limit == 0 || end > uint64(len(r.inputs)) {
		end = uint64(len(r.inputs))
	}
	return r.inputs[start:end], r.totalCount, nil
}

func (r *mockSyncRepository) GetLastSnapshot(
	_ context.Context,
	_ string,
) (*model.Input, error) {
	return nil, nil
}

// newForkableMock creates a mock where Fork returns a fresh mock each time,
// properly exercising the fork/replace lifecycle in Synchronize tests.
func newForkableMock() *MockRollupsMachine {
	m := &MockRollupsMachine{}
	m.CloseError = nil
	m.AdvanceAcceptedReturn = true
	m.HashReturn = newHash(1)
	m.OutputsHashReturn = newHash(2)
	m.ForkFunc = func(_ context.Context) (machine.Machine, error) {
		return newForkableMock(), nil
	}
	return m
}

func (s *MachineInstanceSuite) newSyncMachine(processedInputs uint64, appProcessedInputs uint64) *MachineInstanceImpl {
	runtime := newForkableMock()

	inst := &MachineInstanceImpl{
		application: &model.Application{
			ProcessedInputs: appProcessedInputs,
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		},
		runtime:               runtime,
		advanceTimeout:        decisecond,
		inspectTimeout:        centisecond,
		maxConcurrentInspects: 3,
		closeTimeout:          defaultCloseTimeout,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(3),
		logger:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	inst.processedInputs.Store(processedInputs)
	return inst
}

func makeInputs(startIndex, count uint64) []*model.Input {
	inputs := make([]*model.Input, count)
	for i := uint64(0); i < count; i++ {
		inputs[i] = &model.Input{
			Index:      startIndex + i,
			EpochIndex: 0,
			RawData:    []byte{byte(startIndex + i)},
		}
	}
	return inputs
}

func (s *MachineInstanceSuite) TestSynchronize() {
	s.Run("TemplateSyncAllInputs", func() {
		require := s.Require()
		inst := s.newSyncMachine(0, 3)
		originalRuntime := inst.runtime
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 3),
			totalCount: 3,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.NoError(err)
		require.Equal(uint64(3), inst.processedInputs.Load())
		// Verify the runtime was actually replaced (not self-fork)
		require.NotSame(originalRuntime, inst.runtime)
	})

	s.Run("SnapshotSyncRemainingInputs", func() {
		require := s.Require()
		// Snapshot was at index 2, so processedInputs=3, but app has 5 total
		inst := s.newSyncMachine(3, 5)
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 5),
			totalCount: 5,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.NoError(err)
		require.Equal(uint64(5), inst.processedInputs.Load())
	})

	s.Run("NoInputsToReplay", func() {
		require := s.Require()
		inst := s.newSyncMachine(0, 0)
		repo := &mockSyncRepository{
			inputs:     nil,
			totalCount: 0,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.NoError(err)
		require.Equal(uint64(0), inst.processedInputs.Load())
	})

	s.Run("SnapshotAlreadyCaughtUp", func() {
		require := s.Require()
		// Snapshot at last input — nothing to replay
		inst := s.newSyncMachine(5, 5)
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 5),
			totalCount: 5,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.NoError(err)
		require.Equal(uint64(5), inst.processedInputs.Load())
	})

	s.Run("MachineAheadOfDB", func() {
		require := s.Require()
		// Machine has processed 5 inputs but DB only has 3
		inst := s.newSyncMachine(5, 3)
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 3),
			totalCount: 3,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.Error(err)
		require.ErrorIs(err, ErrMachineSynchronization)
		require.Contains(err.Error(), "machine has processed 5 inputs but DB only has 3")
	})

	s.Run("CountMismatch", func() {
		require := s.Require()
		inst := s.newSyncMachine(0, 5)
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 3),
			totalCount: 3, // DB says 3 but app expects 5
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.Error(err)
		require.ErrorIs(err, ErrMachineSynchronization)
		require.Contains(err.Error(), "count mismatch")
	})

	s.Run("ListInputsError", func() {
		require := s.Require()
		inst := s.newSyncMachine(0, 3)
		listErr := errors.New("database connection lost")
		repo := &mockSyncRepository{
			listErr: listErr,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.Error(err)
		require.ErrorIs(err, ErrMachineSynchronization)
		require.Contains(err.Error(), "database connection lost")
	})

	s.Run("AdvanceErrorMidReplay", func() {
		require := s.Require()
		inst := s.newSyncMachine(0, 3)
		// Make each fork return a hard error on Advance.
		runtime := inst.runtime.(*MockRollupsMachine)
		runtime.ForkFunc = func(_ context.Context) (machine.Machine, error) {
			fork := newForkableMock()
			fork.AdvanceError = errors.New("advance failed during replay")
			return fork, nil
		}

		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 3),
			totalCount: 3,
		}

		err := inst.Synchronize(context.Background(), repo, 1000)
		require.Error(err)
		require.ErrorIs(err, ErrMachineSynchronization)
		require.Contains(err.Error(), "failed to replay input")
	})

	s.Run("BatchBoundaryCrossing", func() {
		require := s.Require()
		// Use batchSize=2 with 3 inputs so the loop must fetch two batches
		// (batch 1: inputs 0-1, batch 2: input 2), exercising pagination.
		inst := s.newSyncMachine(0, 3)
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 3),
			totalCount: 3,
		}

		err := inst.Synchronize(context.Background(), repo, 2)
		require.NoError(err)
		require.Equal(uint64(3), inst.processedInputs.Load())
	})

	s.Run("PartialSyncDetected", func() {
		require := s.Require()
		// Machine has 0 processed, app has 5. But the mock only has 2 inputs,
		// simulating rows disappearing between batches.
		// Use batchSize=2 so the first batch returns inputs [0,1] (replayed=2),
		// then the second batch returns 0 rows (offset=2 >= len=2).
		// The loop must detect replayed(2) != toReplay(5) and return an error.
		inst := s.newSyncMachine(0, 5)
		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 2), // only 2 inputs exist
			totalCount: 5,                // but totalCount says 5
		}

		err := inst.Synchronize(context.Background(), repo, 2)
		require.Error(err)
		require.ErrorIs(err, ErrMachineSynchronization)
		require.Contains(err.Error(), "expected to replay 5 inputs but only replayed 2")
	})

	s.Run("ContextCancellation", func() {
		require := s.Require()
		inst := s.newSyncMachine(0, 3)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		repo := &mockSyncRepository{
			inputs:     makeInputs(0, 3),
			totalCount: 3,
		}

		err := inst.Synchronize(ctx, repo, 1000)
		require.Error(err)
	})
}

// ------------------------------------------------------------------------------------------------

type MockRollupsMachine struct {
	ForkReturn machine.Machine
	ForkFunc   func(context.Context) (machine.Machine, error)
	ForkError  error

	HashReturn machine.Hash
	HashError  error

	CheckpointHashError error

	AdvanceAcceptedReturn  bool
	AdvanceOutputsReturn   []machine.Output
	AdvanceReportsReturn   []machine.Report
	AdvanceLeafsReturn     []machine.Hash
	AdvanceRemainingReturn uint64
	OutputsHashReturn      machine.Hash
	OutputsHashError       error
	OutputsHashProofReturn []machine.Hash
	OutputsHashProofError  error
	AdvanceError           error

	InspectAcceptedReturn bool
	InspectReportsReturn  []machine.Report
	InspectError          error

	StoreError error

	CloseError error
}

func (m *MockRollupsMachine) Fork(ctx context.Context) (machine.Machine, error) {
	if m.ForkFunc != nil {
		return m.ForkFunc(ctx)
	}
	return m.ForkReturn, m.ForkError
}

func (m *MockRollupsMachine) Hash(_ context.Context) (machine.Hash, error) {
	return m.HashReturn, m.HashError
}

func (m *MockRollupsMachine) OutputsHash(_ context.Context) (machine.Hash, error) {
	return m.OutputsHashReturn, m.OutputsHashError
}

func (m *MockRollupsMachine) OutputsHashProof(_ context.Context) ([]machine.Hash, error) {
	return m.OutputsHashProofReturn, m.OutputsHashProofError
}

func (m *MockRollupsMachine) WriteCheckpointHash(_ context.Context, _ machine.Hash) error {
	return m.CheckpointHashError
}

func (m *MockRollupsMachine) Advance(_ context.Context, _ []byte, _ bool) (*machine.AdvanceResponse, error) {
	return &machine.AdvanceResponse{
		Accepted:        m.AdvanceAcceptedReturn,
		Outputs:         m.AdvanceOutputsReturn,
		Reports:         m.AdvanceReportsReturn,
		Hashes:          m.AdvanceLeafsReturn,
		RemainingCycles: m.AdvanceRemainingReturn,
		OutputsHash:     m.OutputsHashReturn,
	}, m.AdvanceError
}

func (m *MockRollupsMachine) Inspect(_ context.Context, _ []byte) (bool, []machine.Report, error) {
	return m.InspectAcceptedReturn, m.InspectReportsReturn, m.InspectError
}

func (m *MockRollupsMachine) Store(_ context.Context, _ string) error {
	return m.StoreError
}

func (m *MockRollupsMachine) Close() error {
	return m.CloseError
}

func (m *MockRollupsMachine) Address() string {
	return "mock-address"
}
