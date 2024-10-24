// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/stretchr/testify/suite"
)

func TestNodeMachine(t *testing.T) {
	suite.Run(t, new(NodeMachineSuite))
}

type NodeMachineSuite struct{ suite.Suite }

func (s *NodeMachineSuite) TestNew() {
	s.Run("Ok", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := NewNodeMachine(inner, 0, decisecond, centisecond, 3)
		require.Nil(err)
		require.NotNil(machine)
	})

	s.Run("ErrInvalidAdvanceTimeout", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := NewNodeMachine(inner, 0, -1, centisecond, 3)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidAdvanceTimeout, err)
	})

	s.Run("ErrInvalidInspectTimeout", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := NewNodeMachine(inner, 0, decisecond, -500, 3)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidInspectTimeout, err)
	})

	s.Run("ErrInvalidMaxConcurrentInspects", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := NewNodeMachine(inner, 0, decisecond, centisecond, 0)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidMaxConcurrentInspects, err)
	})
}

func (s *NodeMachineSuite) TestAdvance() {
	s.Run("Ok", func() {
		s.Run("Accept", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Nil(err)
			require.NotNil(res)

			require.Same(fork, machine.inner)
			require.Equal(model.InputStatusAccepted, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(2), *res.MachineHash)
			require.Equal(uint64(6), machine.processedInputs)
		})

		s.Run("Reject", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()
			fork.AdvanceAcceptedReturn = false

			res, err := machine.Advance(context.Background(), []byte{}, 5)
			require.Nil(err)
			require.NotNil(res)

			require.Same(fork, machine.inner)
			require.Equal(model.InputStatusRejected, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Nil(res.MachineHash)
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
				require.Nil(res.MachineHash)
				require.Equal(uint64(6), machine.processedInputs)
			})
		}

		testSoftError("Exception",
			rollupsmachine.ErrException,
			model.InputStatusException)

		testSoftError("Halted",
			rollupsmachine.ErrHalted,
			model.InputStatusMachineHalted)

		testSoftError("OutputsLimit",
			rollupsmachine.ErrOutputsLimitExceeded,
			model.InputStatusOutputsLimitExceeded)

		testSoftError("CycleLimit",
			rollupsmachine.ErrCycleLimitExceeded,
			model.InputStatusCycleLimitExceeded)

		testSoftError("TimeLimit",
			cartesimachine.ErrTimedOut,
			model.InputStatusTimeLimitExceeded)

		testSoftError("PayloadLengthLimit",
			rollupsmachine.ErrPayloadLengthLimitExceeded,
			model.InputStatusPayloadLengthLimitExceeded)
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

func (s *NodeMachineSuite) TestInspect() {
	s.Run("Ok", func() {
		s.Run("Accept", func() {
			require := s.Require()
			_, fork, machine := s.setupInspect()

			res, err := machine.Inspect(context.Background(), []byte{})
			require.Nil(err)
			require.NotNil(res)

			require.NotSame(fork, machine.inner)
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

			require.NotSame(fork, machine.inner)
			require.Equal(uint64(55), res.ProcessedInputs)
			require.False(res.Accepted)
			require.Equal(expectedReports2, res.Reports)
			require.Nil(res.Error)
		})
	})

	s.Run("Error", func() {
		s.Run("Acquire", func() {
			s.T().Skip("TODO")
		})

		s.Run("Fork", func() {
			s.T().Skip("TODO")
		})

		s.Run("Inspect", func() {
			s.T().Skip("TODO")
		})

		s.Run("Close", func() {
			s.T().Skip("TODO")
		})
	})

	s.Run("Concurrency", func() {
		// At most N Inspects can be active concurrently.
		s.T().Skip("TODO")
	})

}

func (s *NodeMachineSuite) TestClose() {
	// No Advances and/or Inspects can be active concurrently to Close.
	s.T().Skip("TODO")
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

func (s *NodeMachineSuite) setupAdvance() (*MockRollupsMachine, *MockRollupsMachine, *NodeMachine) {
	inner := &MockRollupsMachine{}
	machine, err := NewNodeMachine(inner, 5, decisecond, centisecond, 3)
	s.Require().Nil(err)

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

func (s *NodeMachineSuite) setupInspect() (*MockRollupsMachine, *MockRollupsMachine, *NodeMachine) {
	inner := &MockRollupsMachine{}
	machine, err := NewNodeMachine(inner, 55, decisecond, centisecond, 3)
	s.Require().Nil(err)

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

func newHash(n byte) model.Hash {
	hash := rollupsmachine.Hash{}
	for i := 0; i < 32; i++ {
		hash[i] = n
	}
	return hash
}

func newBytes(n byte, size int) []byte {
	bytes := make([]byte, size)
	for i := 0; i < size; i++ {
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
