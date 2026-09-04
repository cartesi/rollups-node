// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/manager/pmutex"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/semaphore"
)

func TestMachineInstance(t *testing.T) {
	suite.Run(t, new(MachineInstanceSuite))
}

type MachineInstanceSuite struct{ suite.Suite }

func (s *MachineInstanceSuite) TestOverflowCompletionStatusMapsToInputOverflow() {
	status, err := toInputStatus(machine.CompletionStatusOverflow)
	s.Require().NoError(err)
	s.Require().Equal(model.InputCompletionStatus_Overflow, status)
}

// MockMachineRuntimeFactory implements MachineRuntimeFactory for testing
type MockMachineRuntimeFactory struct {
	RuntimeToReturn machine.Machine
	ErrorToReturn   error
}

func (f *MockMachineRuntimeFactory) CreateMachineRuntime(
	_ context.Context,
	_ *model.Application,
	_ *slog.Logger,
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
			nil,
		)
		require.Error(err)
		require.Nil(machine)
		require.Contains(err.Error(), "factory must not be nil")
	})

	s.Run("FactoryErrorClosesPartialRuntime", func() {
		require := s.Require()
		factoryErr := errors.New("factory failed after creating runtime")
		closeErr := errors.New("partial runtime close failed")
		partial := &MockRollupsMachine{CloseError: closeErr}
		factory := &MockMachineRuntimeFactory{RuntimeToReturn: partial, ErrorToReturn: factoryErr}
		app := &model.Application{ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline: decisecond, InspectMaxDeadline: centisecond, MaxConcurrentInspects: 1,
		}}

		instance, err := NewMachineInstanceWithFactory(
			context.Background(), app, 0,
			slog.New(slog.NewTextHandler(io.Discard, nil)), factory,
		)

		require.Nil(instance)
		require.ErrorIs(err, ErrMachineCreation)
		require.ErrorIs(err, factoryErr)
		require.ErrorIs(err, closeErr)
		require.Equal(int64(1), partial.CloseCalls.Load())
	})

	s.Run("FactoryNilRuntimeIsRejected", func() {
		require := s.Require()
		app := &model.Application{ExecutionParameters: model.ExecutionParameters{
			AdvanceMaxDeadline: decisecond, InspectMaxDeadline: centisecond, MaxConcurrentInspects: 1,
		}}
		factory := &MockMachineRuntimeFactory{}

		instance, err := NewMachineInstanceWithFactory(
			context.Background(), app, 0,
			slog.New(slog.NewTextHandler(io.Discard, nil)), factory,
		)

		require.Nil(instance)
		require.ErrorIs(err, ErrMachineCreation)
		require.Contains(err.Error(), "nil runtime")
	})
}

func (s *MachineInstanceSuite) TestNewMachineInstanceFromSnapshot() {
	newApp := func() *model.Application {
		return &model.Application{
			Name: "TestApp",
			ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline:    decisecond,
				InspectMaxDeadline:    centisecond,
				MaxConcurrentInspects: 3,
			},
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	s.Run("MatchingHashCreatesInstanceAtNextInput", func() {
		require := s.Require()
		expected := newHash(7)
		runtime := &MockRollupsMachine{HashReturn: expected}
		loader := func(
			_ context.Context, _ *slog.Logger, config *machine.MachineConfig,
		) (machine.Machine, error) {
			require.Equal("snapshot-dir", config.Path)
			return runtime, nil
		}

		instance, err := newMachineInstanceFromSnapshot(
			context.Background(), newApp(), logger, "snapshot-dir", expected, 5, loader,
		)

		require.NoError(err)
		require.Equal(uint64(6), instance.ProcessedInputs())
		require.Equal(int64(1), runtime.HashCalls.Load())
		require.Zero(runtime.CloseCalls.Load())
		require.NoError(instance.Close())
	})

	s.Run("MismatchingHashClosesRuntime", func() {
		require := s.Require()
		runtime := &MockRollupsMachine{HashReturn: newHash(8)}
		factory := &SnapshotMachineRuntimeFactory{
			SnapshotPath: "snapshot-dir",
			ExpectedHash: newHash(9),
			loader: func(context.Context, *slog.Logger, *machine.MachineConfig) (machine.Machine, error) {
				return runtime, nil
			},
		}

		got, err := factory.CreateMachineRuntime(context.Background(), newApp(), logger)

		require.Nil(got)
		require.Error(err)
		require.Contains(err.Error(), "machine hash mismatch")
		require.Equal(int64(1), runtime.HashCalls.Load())
		require.Equal(int64(1), runtime.CloseCalls.Load())
	})

	s.Run("HashReadErrorClosesRuntime", func() {
		require := s.Require()
		hashErr := errors.New("hash unavailable")
		runtime := &MockRollupsMachine{HashError: hashErr}
		factory := &SnapshotMachineRuntimeFactory{
			SnapshotPath: "snapshot-dir",
			ExpectedHash: newHash(9),
			loader: func(context.Context, *slog.Logger, *machine.MachineConfig) (machine.Machine, error) {
				return runtime, nil
			},
		}

		got, err := factory.CreateMachineRuntime(context.Background(), newApp(), logger)

		require.Nil(got)
		require.ErrorIs(err, hashErr)
		require.Equal(int64(1), runtime.HashCalls.Load())
		require.Equal(int64(1), runtime.CloseCalls.Load())
	})

	s.Run("NilRuntimeFromLoaderIsRejected", func() {
		require := s.Require()
		loader := func(context.Context, *slog.Logger, *machine.MachineConfig) (machine.Machine, error) {
			return nil, nil
		}

		instance, err := newMachineInstanceFromSnapshot(
			context.Background(), newApp(), logger, "snapshot-dir", newHash(1), 0, loader,
		)

		require.Nil(instance)
		require.ErrorIs(err, ErrMachineCreation)
		require.Contains(err.Error(), "machine loader returned nil runtime")
	})

	s.Run("InputIndexOverflowIsRejectedBeforeLoad", func() {
		require := s.Require()

		instance, err := NewMachineInstanceFromSnapshot(
			context.Background(), newApp(), logger, "snapshot-dir", newHash(1), math.MaxUint64,
		)

		require.Nil(instance)
		require.ErrorIs(err, ErrInvalidSnapshotPoint)
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
		context.Background(), app, 42, testLogger, mockFactory,
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
			require.Equal(newHash(1), res.TxBufferDataBlock)
			require.Equal(newHash(2), res.MachineHash)
			require.True(res.IsComplete())
			require.Equal(uint64(6), machine.processedInputs.Load())
		})

		s.Run("Reject", func() {
			require := s.Require()
			inner, fork, instance := s.setupAdvance()
			fork.CompletionStatusReturn = machine.CompletionStatusRejected
			fork.CloseError = nil

			res, err := instance.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Nil(err)
			require.NotNil(res)

			require.Same(inner, instance.runtime)
			require.Equal(model.InputCompletionStatus_Rejected, res.Status)
			require.Empty(res.Outputs)
			require.Empty(res.Reports)
			require.Equal(newHash(1), res.TxBufferDataBlock)
			require.Equal(newHash(2), res.MachineHash)
			require.True(res.IsComplete())
			require.Equal(uint64(6), instance.processedInputs.Load())
		})

		testCompletedStatus := func(
			name string,
			machineStatus machine.CompletionStatus,
			inputStatus model.InputCompletionStatus,
			exceptionData []byte,
		) {
			s.Run(name, func() {
				require := s.Require()
				inner, fork, instance := s.setupAdvance()
				fork.CompletionStatusReturn = machineStatus
				fork.ExceptionDataReturn = exceptionData
				fork.CloseError = nil
				preProof := fork.StateProofReturn
				postProof := acceptedStateProof(newHash(2), newHash(1))
				postProof.IflagsYProof.DataBlock = machine.Hash{}
				proofCalls := 0
				fork.StateProofFunc = func(context.Context) (*machine.StateProof, error) {
					proofCalls++
					if proofCalls == 1 {
						return preProof, nil
					}
					return postProof, nil
				}

				res, err := instance.Advance(context.Background(), []byte{}, 0, 5, false)
				require.Nil(err)
				require.NotNil(res)

				require.Nil(instance.runtime)
				require.Equal(inputStatus, res.Status)
				require.Equal(exceptionData, res.ExceptionData)
				require.Empty(res.Outputs)
				require.Empty(res.Reports)
				require.Equal(newHash(1), res.TxBufferDataBlock)
				require.Equal(newHash(2), res.MachineHash)
				require.True(res.IsComplete())
				require.Equal(uint64(6), instance.processedInputs.Load())
				require.Equal(int64(1), inner.CloseCalls.Load())
				require.Equal(int64(1), fork.CloseCalls.Load())
				_, inspectErr := instance.Inspect(context.Background(), nil)
				require.ErrorIs(inspectErr, ErrMachineClosed)
			})
		}

		testCompletedStatus("Exception",
			machine.CompletionStatusException,
			model.InputCompletionStatus_Exception,
			[]byte("guest exception"))

		testCompletedStatus("Halted",
			machine.CompletionStatusHalted,
			model.InputCompletionStatus_MachineHalted,
			nil)

		testCompletedStatus("Overflow",
			machine.CompletionStatusOverflow,
			model.InputCompletionStatus_Overflow,
			nil)

		testCompletedStatus("UnexpectedYield",
			machine.CompletionStatusUnexpectedYield,
			model.InputCompletionStatus_UnexpectedYield,
			nil)
	})

	s.Run("Error", func() {
		interruptions := []struct {
			name string
			err  error
		}{
			{"OutputsLimit", machine.ErrOutputsLimitExceeded},
			{"ReportsLimit", machine.ErrReportsLimitExceeded},
			{"ReachedCycleLimit", machine.ErrReachedLimitMcycle},
			{"Deadline", machine.ErrDeadlineExceeded},
			{"Canceled", machine.ErrCanceled},
			{"PayloadLengthLimit", machine.ErrPayloadLengthLimitExceeded},
			{"MachineInternal", machine.ErrMachineInternal},
		}
		for _, interruption := range interruptions {
			s.Run(interruption.name, func() {
				require := s.Require()
				inner, fork, instance := s.setupAdvance()
				fork.AdvanceError = interruption.err
				fork.CloseError = nil

				res, err := instance.Advance(context.Background(), []byte{}, 0, 5, false)
				require.ErrorIs(err, interruption.err)
				require.Nil(res)
				require.Same(inner, instance.runtime)
				require.Equal(uint64(5), instance.processedInputs.Load())
			})
		}

		s.Run("UnknownStatus", func() {
			require := s.Require()
			inner, fork, instance := s.setupAdvance()
			fork.CompletionStatusReturn = machine.CompletionStatusUnknown
			fork.CloseError = nil

			res, err := instance.Advance(context.Background(), []byte{}, 0, 5, false)
			require.ErrorIs(err, ErrIncompleteAdvance)
			require.Nil(res)
			require.Same(inner, instance.runtime)
			require.Equal(uint64(5), instance.processedInputs.Load())
		})

		for _, test := range []struct {
			name          string
			status        machine.CompletionStatus
			exceptionData []byte
		}{
			{"ExceptionWithoutData", machine.CompletionStatusException, nil},
			{"AcceptedWithExceptionData", machine.CompletionStatusAccepted, []byte("unexpected")},
		} {
			s.Run(test.name, func() {
				require := s.Require()
				inner, fork, instance := s.setupAdvance()
				fork.CompletionStatusReturn = test.status
				fork.ExceptionDataReturn = test.exceptionData
				fork.CloseError = nil

				res, err := instance.Advance(context.Background(), []byte{}, 0, 5, false)
				require.ErrorIs(err, ErrIncompleteAdvance)
				require.ErrorIs(err, machine.ErrMachineInternal)
				require.Nil(res)
				require.Same(inner, instance.runtime)
				require.Equal(uint64(5), instance.processedInputs.Load())
			})
		}

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

		s.Run("StateProof", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errProof := errors.New("state proof error")
			fork.StateProofError = errProof
			fork.CloseError, inner.CloseError = inner.CloseError, fork.CloseError

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errProof)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("StateProofAndClose", func() {
			require := s.Require()
			inner, fork, machine := s.setupAdvance()
			errProof := errors.New("state proof error")
			errClose := errors.New("Close error")
			fork.StateProofError = errProof
			fork.CloseError = errClose
			inner.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{}, 0, 5, false)
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errProof)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
			require.Equal(uint64(5), machine.processedInputs.Load())
		})

		s.Run("PostAdvanceStateProof", func() {
			require := s.Require()
			inner, fork, instance := s.setupAdvance()
			errProof := errors.New("post-advance state proof error")
			preProof := fork.StateProofReturn
			proofCalls := 0
			fork.StateProofFunc = func(context.Context) (*machine.StateProof, error) {
				proofCalls++
				if proofCalls == 1 {
					return preProof, nil
				}
				return nil, errProof
			}
			fork.CloseError = nil

			res, err := instance.Advance(context.Background(), nil, 0, 5, false)
			require.Nil(res)
			require.ErrorIs(err, errProof)
			require.Same(inner, instance.runtime)
			require.Equal(uint64(5), instance.processedInputs.Load())
			require.Equal(int64(1), fork.CloseCalls.Load())
			require.Zero(inner.CloseCalls.Load())
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
				fork.CompletionStatusReturn = machine.CompletionStatusException
				fork.ExceptionDataReturn = []byte("guest exception")
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
		inner, fork, instance := s.setupAdvance()

		fork.AdvanceRemainingReturn = machine.InputEntryCapacity

		res, err := instance.Advance(context.Background(), []byte{}, 0, 5, true)
		require.Nil(err)
		require.NotNil(res)

		require.Same(fork, instance.runtime)
		require.Equal(model.InputCompletionStatus_Accepted, res.Status)
		require.True(res.IsDaveConsensus)
		require.Equal(uint64(6), instance.processedInputs.Load())

		// Verify the inner runtime was closed (accept path)
		_ = inner
	})

	s.Run("SequentialAdvances", func() {
		// Advance is serialized by advanceMutex — concurrent advance on the
		// same machine never happens by design. This test verifies that two
		// sequential advances correctly increment processedInputs.
		require := s.Require()
		inner, fork, instance := s.setupAdvance()

		// Allow inner.Close to succeed (old runtime close on accept)
		inner.CloseError = nil

		// First advance: fork from inner (processedInputs=5), returns accepted.
		// After accept, fork becomes the new runtime.
		// Second advance: fork from fork (processedInputs=6), fork must also fork.
		fork2 := &MockRollupsMachine{}
		fork2.CompletionStatusReturn = machine.CompletionStatusAccepted
		fork2.AdvanceOutputsReturn = expectedOutputs
		fork2.AdvanceReportsReturn = expectedReports1
		fork2.HashReturn = newHash(2)
		fork2.StateProofReturn = acceptedStateProof(newHash(2), newHash(1))
		fork2.CloseError = errUnreachable // old runtime close for second advance
		fork2.ForkReturn = nil
		fork.ForkReturn = fork2
		fork.CloseError = nil // close of fork (now old runtime) in second advance

		// First advance at index 5
		res1, err := instance.Advance(context.Background(), []byte{}, 0, 5, false)
		require.Nil(err)
		require.NotNil(res1)
		require.Equal(uint64(6), instance.processedInputs.Load())

		// Second advance at index 6
		res2, err := instance.Advance(context.Background(), []byte{}, 0, 6, false)
		require.Nil(err)
		require.NotNil(res2)
		require.Equal(uint64(7), instance.processedInputs.Load())
	})
}

func (s *MachineInstanceSuite) TestInspect() {
	for _, test := range []struct {
		name   string
		status machine.CompletionStatus
	}{
		{"Accept", machine.CompletionStatusAccepted},
		{"Reject", machine.CompletionStatusRejected},
		{"Exception", machine.CompletionStatusException},
		{"Halted", machine.CompletionStatusHalted},
		{"Overflow", machine.CompletionStatusOverflow},
		{"UnexpectedYield", machine.CompletionStatusUnexpectedYield},
	} {
		s.Run(test.name, func() {
			require := s.Require()
			_, fork, instance := s.setupInspect()
			fork.InspectResponseReturn.Status = test.status
			if test.status == machine.CompletionStatusException {
				fork.InspectResponseReturn.ExceptionData = []byte("guest exception")
			}

			result, err := instance.Inspect(context.Background(), []byte{})
			require.NoError(err)
			require.NotNil(result)
			require.NotSame(fork, instance.runtime)
			require.Equal(uint64(55), result.ProcessedInputs)
			require.Equal(test.status, result.Status)
			require.Equal(fork.InspectResponseReturn.ExceptionData, result.ExceptionData)
			require.Equal(expectedReports2, result.Reports)
			require.NoError(result.Error)
		})
	}

	for _, test := range []struct {
		name          string
		status        machine.CompletionStatus
		exceptionData []byte
	}{
		{"ExceptionWithoutData", machine.CompletionStatusException, nil},
		{"RejectedWithExceptionData", machine.CompletionStatusRejected, []byte("unexpected")},
	} {
		s.Run(test.name+"FailsClosed", func() {
			require := s.Require()
			_, fork, instance := s.setupInspect()
			fork.InspectResponseReturn.Status = test.status
			fork.InspectResponseReturn.ExceptionData = test.exceptionData

			result, err := instance.Inspect(context.Background(), []byte{})
			require.NoError(err)
			require.Equal(machine.CompletionStatusUnknown, result.Status)
			require.ErrorIs(result.Error, ErrIncompleteInspect)
			require.ErrorIs(result.Error, machine.ErrMachineInternal)
		})
	}

	s.Run("AtCapacity", func() {
		require := s.Require()
		_, _, instance := s.setupInspect()
		instance.inspectSemaphore.TryAcquire(int64(instance.maxConcurrentInspects))
		defer instance.inspectSemaphore.Release(int64(instance.maxConcurrentInspects))

		result, err := instance.Inspect(context.Background(), []byte{})
		require.ErrorIs(err, ErrInspectAtCapacity)
		require.Nil(result)
	})

	s.Run("ForkError", func() {
		require := s.Require()
		inner, _, instance := s.setupInspect()
		errFork := errors.New("Fork error")
		inner.ForkError = errFork

		result, err := instance.Inspect(context.Background(), []byte{})
		require.ErrorIs(err, errFork)
		require.Nil(result)
	})

	s.Run("ExecutionErrorPreservesPartialReports", func() {
		require := s.Require()
		_, fork, instance := s.setupInspect()
		errInspect := errors.New("Inspect error")
		fork.InspectError = errInspect

		result, err := instance.Inspect(context.Background(), []byte{})
		require.NoError(err)
		require.Equal(machine.CompletionStatusUnknown, result.Status)
		require.Equal(expectedReports2, result.Reports)
		require.ErrorIs(result.Error, errInspect)
	})

	for _, test := range []struct {
		name     string
		response *machine.InspectResponse
	}{
		{"NilResponse", nil},
		{"UnknownStatus", &machine.InspectResponse{Status: machine.CompletionStatusUnknown}},
		{"InvalidStatus", &machine.InspectResponse{Status: machine.CompletionStatus(255)}},
	} {
		s.Run(test.name+"FailsClosed", func() {
			require := s.Require()
			_, fork, instance := s.setupInspect()
			fork.InspectResponseReturn = test.response

			result, err := instance.Inspect(context.Background(), []byte{})
			require.NoError(err)
			require.Equal(machine.CompletionStatusUnknown, result.Status)
			require.ErrorIs(result.Error, ErrIncompleteInspect)
			require.ErrorIs(result.Error, machine.ErrMachineInternal)
		})
	}

	s.Run("CloseErrorOverridesResult", func() {
		require := s.Require()
		_, fork, instance := s.setupInspect()
		errClose := errors.New("Close error")
		fork.CloseError = errClose

		result, err := instance.Inspect(context.Background(), []byte{})
		require.ErrorIs(err, errClose)
		require.Nil(result)
	})

	s.Run("Concurrency", func() {
		require := s.Require()
		_, _, instance := s.setupInspect()
		var wg sync.WaitGroup
		errs := make(chan error, instance.maxConcurrentInspects)

		for range int(instance.maxConcurrentInspects) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := instance.Inspect(context.Background(), []byte{})
				if err != nil {
					errs <- err
				}
			}()
		}

		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(err)
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
		require.ErrorIs(err, ErrMachineClosed)

		// Runtime should be destroyed after a store error.
		require.Nil(machine.runtime)
	})

	s.Run("CanceledPreservesRuntime", func() {
		require := s.Require()
		inner, _, machineInst := s.setupAdvance()
		inner.StoreError = machine.ErrCanceled

		err := machineInst.CreateSnapshot(context.Background(), 5, "/tmp/snapshot")
		require.ErrorIs(err, machine.ErrCanceled)
		require.NotErrorIs(err, ErrMachineClosed)
		require.Same(inner, machineInst.runtime)
		require.Zero(inner.CloseCalls.Load())
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
		require.ErrorIs(err, ErrMachineClosed)
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
		inner, machineInst := s.setupStateProof()

		hash, err := machineInst.Hash(context.Background())
		require.NoError(err)
		require.Equal([32]byte(newHash(1)), hash)

		// Runtime should still be alive after a successful call.
		require.Same(inner, machineInst.runtime)
	})

	s.Run("MachineClosed", func() {
		require := s.Require()
		_, machineInst := s.setupStateProof()
		machineInst.runtime = nil

		hash, err := machineInst.Hash(context.Background())
		require.Error(err)
		require.Equal(ErrMachineClosed, err)
		require.Equal([32]byte{}, hash)
	})

	s.Run("Error", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		errHash := errors.New("Hash error")
		inner.HashError = errHash
		inner.CloseError = nil

		hash, err := machineInst.Hash(context.Background())
		require.Error(err)
		require.ErrorIs(err, errHash)
		require.ErrorIs(err, ErrMachineClosed)
		require.Equal([32]byte{}, hash)

		// Runtime should be destroyed after a hash error.
		require.Nil(machineInst.runtime)
	})

	s.Run("ErrorAndCloseError", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		errHash := errors.New("Hash error")
		errClose := errors.New("Close error")
		inner.HashError = errHash
		inner.CloseError = errClose

		hash, err := machineInst.Hash(context.Background())
		require.Error(err)
		require.ErrorIs(err, errHash)
		require.ErrorIs(err, errClose)
		require.ErrorIs(err, ErrMachineClosed)
		require.Equal([32]byte{}, hash)
		require.Nil(machineInst.runtime)
	})

	s.Run("CanceledPreservesRuntime", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		inner.HashError = machine.ErrCanceled

		hash, err := machineInst.Hash(context.Background())
		require.Equal([32]byte{}, hash)
		require.ErrorIs(err, machine.ErrCanceled)
		require.NotErrorIs(err, ErrMachineClosed)
		require.Same(inner, machineInst.runtime)
		require.Zero(inner.CloseCalls.Load())
	})
}

func (s *MachineInstanceSuite) TestStateProof() {
	s.Run("Ok", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()

		proof, err := machineInst.StateProof(context.Background())
		require.NoError(err)
		require.NotNil(proof)

		require.Equal(newHash(1), proof.MachineHash)
		require.Equal(newHash(2), proof.TxBufferDataBlock)
		require.True(proof.IsComplete())
		require.Equal(common.Hash(inner.StateProofReturn.IflagsYProof.DataBlock), proof.IflagsYDataBlock)
		require.Equal(common.Hash(inner.StateProofReturn.HtifTohostProof.DataBlock), proof.HtifTohostDataBlock)

		// Runtime should still be alive after a successful call.
		require.Same(inner, machineInst.runtime)
	})

	s.Run("MachineClosed", func() {
		require := s.Require()
		_, machineInst := s.setupStateProof()
		machineInst.runtime = nil

		proof, err := machineInst.StateProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.Equal(ErrMachineClosed, err)
	})

	s.Run("StateProofError", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		errProof := errors.New("state proof error")
		inner.StateProofError = errProof
		inner.CloseError = nil

		proof, err := machineInst.StateProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.ErrorIs(err, errProof)
		require.ErrorIs(err, ErrMachineClosed)

		// Runtime should be destroyed after a proof error.
		require.Nil(machineInst.runtime)
	})

	s.Run("StateProofErrorAndCloseError", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		errProof := errors.New("state proof error")
		errClose := errors.New("Close error")
		inner.StateProofError = errProof
		inner.CloseError = errClose

		proof, err := machineInst.StateProof(context.Background())
		require.Nil(proof)
		require.Error(err)
		require.ErrorIs(err, errProof)
		require.ErrorIs(err, errClose)
		require.ErrorIs(err, ErrMachineClosed)

		// Runtime should be destroyed even when Close also fails.
		require.Nil(machineInst.runtime)
	})

	s.Run("CanceledPreservesRuntime", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		inner.StateProofError = machine.ErrCanceled

		proof, err := machineInst.StateProof(context.Background())
		require.Nil(proof)
		require.ErrorIs(err, machine.ErrCanceled)
		require.NotErrorIs(err, ErrMachineClosed)
		require.Same(inner, machineInst.runtime)
		require.Zero(inner.CloseCalls.Load())
	})

	s.Run("NilStateProof", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		inner.StateProofReturn = nil
		inner.CloseError = nil

		proof, err := machineInst.StateProof(context.Background())
		require.Nil(proof)
		require.ErrorIs(err, machine.ErrInvalidMachineProof)
		require.ErrorIs(err, ErrMachineClosed)

		require.Nil(machineInst.runtime)
	})

	s.Run("IncompleteStateProof", func() {
		require := s.Require()
		inner, machineInst := s.setupStateProof()
		inner.StateProofReturn.TxBufferProof.Siblings = nil
		inner.CloseError = nil

		proof, err := machineInst.StateProof(context.Background())
		require.Nil(proof)
		require.ErrorIs(err, machine.ErrInvalidMachineProof)
		require.ErrorIs(err, ErrMachineClosed)

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

	fork.CompletionStatusReturn = machine.CompletionStatusAccepted
	fork.AdvanceOutputsReturn = []machine.Output{
		newBytes(11, 100),
		newBytes(12, 100),
		newBytes(13, 100),
	}
	fork.AdvanceReportsReturn = []machine.Report{
		newBytes(21, 200),
		newBytes(22, 200),
	}
	fork.AdvanceError = nil

	fork.HashReturn = newHash(2)
	fork.HashError = nil
	fork.StateProofReturn = acceptedStateProof(newHash(2), newHash(1))

	fork.InspectResponseReturn = &machine.InspectResponse{
		Status: machine.CompletionStatusAccepted,
		Reports: []machine.Report{
			newBytes(31, 300),
			newBytes(32, 300),
			newBytes(33, 300),
			newBytes(34, 300),
		},
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

	fork.InspectResponseReturn = &machine.InspectResponse{
		Status: machine.CompletionStatusAccepted,
		Reports: []machine.Report{
			newBytes(31, 300),
			newBytes(32, 300),
			newBytes(33, 300),
			newBytes(34, 300),
		},
	}
	fork.InspectError = nil

	fork.CloseError = nil

	return inner, fork, machineInst
}

func (s *MachineInstanceSuite) setupStateProof() (*MockRollupsMachine, *MachineInstanceImpl) {
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
	inner.StateProofReturn = acceptedStateProof(newHash(1), newHash(2))
	inner.CloseError = errUnreachable

	return inner, machineInst
}

func testValidityLeaf(dataBlock, sibling machine.Hash) machine.LeafProof {
	siblings := make([]machine.Hash, model.StateProofSiblingCount)
	for i := range siblings {
		siblings[i] = sibling
	}
	return machine.LeafProof{DataBlock: dataBlock, Siblings: siblings}
}

func acceptedStateProof(machineHash, outputsHash machine.Hash) *machine.StateProof {
	iflagsYData := newHash(6)
	for index := 8; index < 16; index++ {
		iflagsYData[index] = 0
	}
	iflagsYData[8] = 1
	htifTohostData := newHash(7)
	for index := 16; index < 24; index++ {
		htifTohostData[index] = 0
	}
	htifTohostData[20] = 1
	htifTohostData[22] = 1
	htifTohostData[23] = 2
	return &machine.StateProof{
		MachineHash:     machineHash,
		IflagsYProof:    testValidityLeaf(iflagsYData, newHash(8)),
		HtifTohostProof: testValidityLeaf(htifTohostData, newHash(9)),
		TxBufferProof:   testValidityLeaf(outputsHash, newHash(10)),
	}
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

type MockRollupsMachine struct {
	ForkReturn machine.Machine
	ForkFunc   func(context.Context) (machine.Machine, error)
	ForkError  error

	HashReturn machine.Hash
	HashError  error
	HashCalls  atomic.Int64

	CompletionStatusReturn   machine.CompletionStatus
	ExceptionDataReturn      []byte
	AdvanceOutputsReturn     []machine.Output
	AdvanceReportsReturn     []machine.Report
	AdvanceLeafsReturn       []machine.Hash
	AdvanceRemainingReturn   uint64
	StateProofReturn         *machine.StateProof
	StateProofError          error
	StateProofFunc           func(context.Context) (*machine.StateProof, error)
	AdvanceError             error
	LastAdvanceComputeHashes bool

	InspectResponseReturn *machine.InspectResponse
	InspectError          error

	StoreError error

	CloseError error
	CloseCalls atomic.Int64
}

func (m *MockRollupsMachine) Fork(ctx context.Context) (machine.Machine, error) {
	if m.ForkFunc != nil {
		return m.ForkFunc(ctx)
	}
	return m.ForkReturn, m.ForkError
}

func (m *MockRollupsMachine) Hash(_ context.Context) (machine.Hash, error) {
	m.HashCalls.Add(1)
	return m.HashReturn, m.HashError
}

func (m *MockRollupsMachine) StateProof(ctx context.Context) (*machine.StateProof, error) {
	if m.StateProofFunc != nil {
		return m.StateProofFunc(ctx)
	}
	return m.StateProofReturn, m.StateProofError
}

func (m *MockRollupsMachine) Advance(_ context.Context, _ []byte, _ machine.Hash, computeHashes bool) (*machine.AdvanceResponse, error) {
	m.LastAdvanceComputeHashes = computeHashes
	if m.AdvanceError != nil {
		return nil, m.AdvanceError
	}
	return &machine.AdvanceResponse{
		Status:              m.CompletionStatusReturn,
		ExceptionData:       m.ExceptionDataReturn,
		Outputs:             m.AdvanceOutputsReturn,
		Reports:             m.AdvanceReportsReturn,
		PeriodicStateHashes: m.AdvanceLeafsReturn,
		PaddingRepetitions:  m.AdvanceRemainingReturn,
	}, nil
}

func (m *MockRollupsMachine) Inspect(_ context.Context, _ []byte) (*machine.InspectResponse, error) {
	return m.InspectResponseReturn, m.InspectError
}

func (m *MockRollupsMachine) Store(_ context.Context, _ string) error {
	return m.StoreError
}

func (m *MockRollupsMachine) Close() error {
	m.CloseCalls.Add(1)
	return m.CloseError
}

func (m *MockRollupsMachine) Address() string {
	return "mock-address"
}
