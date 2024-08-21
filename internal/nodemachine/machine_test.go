// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package nodemachine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
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
		machine, err := New(inner, decisecond, centisecond, 3)
		require.Nil(err)
		require.NotNil(machine)
	})

	s.Run("ErrInvalidAdvanceTimeout", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := New(inner, -1, centisecond, 3)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidAdvanceTimeout, err)
	})

	s.Run("ErrInvalidInspectTimeout", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := New(inner, decisecond, -500, 3)
		require.Error(err)
		require.Nil(machine)
		require.Equal(ErrInvalidInspectTimeout, err)
	})

	s.Run("ErrInvalidMaxConcurrentInspects", func() {
		require := s.Require()
		inner := &MockRollupsMachine{}
		machine, err := New(inner, decisecond, centisecond, 0)
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

			res, err := machine.Advance(context.Background(), []byte{})
			require.Nil(err)
			require.NotNil(res)

			require.Same(fork, machine.inner)
			require.Equal(model.InputStatusAccepted, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(2), res.MachineHash)
		})

		s.Run("Reject", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()
			fork.AdvanceAcceptedReturn = false

			res, err := machine.Advance(context.Background(), []byte{})
			require.Nil(err)
			require.NotNil(res)

			require.Same(fork, machine.inner)
			require.Equal(model.InputStatusRejected, res.Status)
			require.Equal(expectedOutputs, res.Outputs)
			require.Equal(expectedReports1, res.Reports)
			require.Equal(newHash(1), res.OutputsHash)
			require.Equal(newHash(0), res.MachineHash)
		})
	})

	s.Run("Error", func() {
		s.Run("Fork", func() {
			require := s.Require()
			inner, _, machine := s.setupAdvance()
			errFork := errors.New("Fork error")
			inner.ForkError = errFork

			res, err := machine.Advance(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.Equal(errFork, err)
		})

		s.Run("Advance", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()
			errAdvance := errors.New("Advance error")
			fork.AdvanceError = errAdvance
			fork.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errAdvance)
			require.NotErrorIs(err, errUnreachable)
		})

		s.Run("AdvanceAndClose", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()
			errAdvance := errors.New("Advance error")
			errClose := errors.New("Close error")
			fork.AdvanceError = errAdvance
			fork.CloseError = errClose

			res, err := machine.Advance(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errAdvance)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
		})

		s.Run("Hash", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()
			errHash := errors.New("Hash error")
			fork.HashError = errHash
			fork.CloseError = nil

			res, err := machine.Advance(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errHash)
			require.NotErrorIs(err, errUnreachable)
		})

		s.Run("HashAndClose", func() {
			require := s.Require()
			_, fork, machine := s.setupAdvance()
			errHash := errors.New("Hash error")
			errClose := errors.New("Close error")
			fork.HashError = errHash
			fork.CloseError = errClose

			res, err := machine.Advance(context.Background(), []byte{})
			require.Error(err)
			require.Nil(res)
			require.ErrorIs(err, errHash)
			require.ErrorIs(err, errClose)
			require.NotErrorIs(err, errUnreachable)
		})

		s.Run("Close", func() {
			s.Run("Inner", func() {
				require := s.Require()
				inner, _, machine := s.setupAdvance()
				errClose := errors.New("Close error")
				inner.CloseError = errClose

				res, err := machine.Advance(context.Background(), []byte{})
				require.Error(err)
				require.NotNil(res)
				require.ErrorIs(err, errClose)
				require.NotErrorIs(err, errUnreachable)
			})

			s.Run("Fork", func() {
				require := s.Require()
				_, fork, machine := s.setupAdvance()
				errClose := errors.New("Close error")
				fork.AdvanceError = rollupsmachine.ErrException
				fork.CloseError = errClose

				res, err := machine.Advance(context.Background(), []byte{})
				require.Error(err)
				require.NotNil(res)
				require.ErrorIs(err, errClose)
				require.NotErrorIs(err, errUnreachable)
			})
		})
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
			require.False(res.Accepted)
			require.Equal(expectedReports2, res.Reports)
			require.Nil(res.Error)
		})
	})

	s.Run("Semaphore", func() {
	})

	s.Run("Error", func() {
		s.Run("Acquire", func() {
		})
		s.Run("Fork", func() {
		})
		s.Run("Inspect", func() {
		})
		s.Run("Close", func() {
		})
	})
}

func (s *NodeMachineSuite) TestClose() {
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
	machine, err := New(inner, decisecond, centisecond, 3)
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
	machine, err := New(inner, decisecond, centisecond, 3)
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

func (machine *MockRollupsMachine) Fork() (rollupsmachine.RollupsMachine, error) {
	return machine.ForkReturn, machine.ForkError
}

func (machine *MockRollupsMachine) Hash() (rollupsmachine.Hash, error) {
	return machine.HashReturn, machine.HashError
}

func (machine *MockRollupsMachine) Advance(input []byte) (
	bool, []rollupsmachine.Output, []rollupsmachine.Report, rollupsmachine.Hash, error,
) {
	return machine.AdvanceAcceptedReturn,
		machine.AdvanceOutputsReturn,
		machine.AdvanceReportsReturn,
		machine.AdvanceHashReturn,
		machine.AdvanceError
}

func (machine *MockRollupsMachine) Inspect(query []byte) (bool, []rollupsmachine.Report, error) {
	return machine.InspectAcceptedReturn, machine.InspectReportsReturn, machine.InspectError
}

func (machine *MockRollupsMachine) Close() error {
	return machine.CloseError
}
