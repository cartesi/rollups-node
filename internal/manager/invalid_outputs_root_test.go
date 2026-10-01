// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func (s *MachineInstanceSuite) TestInvalidOutputsRootPreservesFinalEvidence() {
	for _, collect := range []bool{false, true} {
		name := "authority"
		if collect {
			name = "prt"
		}
		s.Run(name, func() {
			inner, fork, instance := s.setupAdvance()
			fork.CompletionStatusReturn = machine.CompletionStatusInvalidOutputsRoot
			fork.CloseError = nil
			preProof := acceptedStateProof(newHash(0x11), newHash(0x12))
			postProof := acceptedStateProof(newHash(0x21), newHash(0x22))
			// Retain the actual declaration, not a repaired 32-byte length.
			const tohostOffset = 16
			binary.LittleEndian.PutUint64(postProof.HtifTohostProof.DataBlock[tohostOffset:],
				uint64(2)<<56|uint64(1)<<48|uint64(1)<<32|31)
			proofCalls := 0
			fork.StateProofFunc = func(context.Context) (*machine.StateProof, error) {
				proofCalls++
				if proofCalls == 1 {
					return preProof, nil
				}
				return postProof, nil
			}
			if collect {
				fork.AdvanceLeafsReturn = []machine.Hash{newHash(0x31)}
				fork.AdvanceRemainingReturn = machine.InputEntryCapacity - 1
			}

			result, err := instance.Advance(context.Background(), []byte("input"), 3, 5, collect)
			s.Require().NoError(err)
			s.Require().NotNil(result)
			s.Equal(model.InputCompletionStatus_InvalidOutputsRoot, result.Status)
			wantProof, err := stateProofFromMachine(postProof)
			s.Require().NoError(err)
			s.Equal(*wantProof, result.StateProof)
			s.NotEqual(preProof.MachineHash, postProof.MachineHash)
			s.Equal(fork.AdvanceLeafsReturn, result.PeriodicStateHashes)
			s.Equal(fork.AdvanceRemainingReturn, result.PaddingRepetitions)
			s.Equal(collect, result.IsDaveConsensus)
			s.Empty(result.Outputs)
			s.Equal(expectedReports1, result.Reports)
			s.Nil(result.ExceptionData)
			s.Equal(2, proofCalls)
			s.Equal(uint64(6), instance.ProcessedInputs())
			s.Nil(instance.runtime)
			s.Equal(int64(1), inner.CloseCalls.Load())
			s.Equal(int64(1), fork.CloseCalls.Load())

			next, err := instance.Advance(context.Background(), []byte("next"), 3, 6, collect)
			s.ErrorIs(err, ErrMachineClosed)
			s.Nil(next)
			s.Equal(uint64(6), instance.ProcessedInputs())
		})
	}
}

func (s *MachineInstanceSuite) TestInvalidOutputsRootNeedsFinalProof() {
	inner, fork, instance := s.setupAdvance()
	fork.CompletionStatusReturn = machine.CompletionStatusInvalidOutputsRoot
	fork.CloseError = nil
	proofErr := errors.New("could not read final machine proof")
	proofCalls := 0
	fork.StateProofFunc = func(context.Context) (*machine.StateProof, error) {
		proofCalls++
		if proofCalls == 1 {
			return fork.StateProofReturn, nil
		}
		return nil, proofErr
	}

	result, err := instance.Advance(context.Background(), []byte("input"), 3, 5, false)
	s.ErrorIs(err, proofErr)
	s.Nil(result)
	s.Equal(uint64(5), instance.ProcessedInputs())
	s.Same(inner, instance.runtime)
	s.Equal(int64(1), fork.CloseCalls.Load())
	s.Zero(inner.CloseCalls.Load())
}

func TestInvalidOutputsRootExcludedFromMachineWorkAndDrain(t *testing.T) {
	repo := &MockMachineRepository{}
	queries := 0
	repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Run(func(args mock.Arguments) {
			filter := args.Get(1).(repository.ApplicationFilter)
			require.NotNil(t, filter.Status)
			require.Equal(t, model.ApplicationStatus_OK, *filter.Status)
			require.NotEqual(t, model.ApplicationStatus_InvalidOutputsRoot, *filter.Status)
			queries++
		}).Return([]*model.Application{}, uint64(0), nil).Twice()

	apps, err := getMachineApplications(context.Background(), repo)
	require.NoError(t, err)
	require.Empty(t, apps)
	require.Equal(t, 2, queries, "both normal execution and foreclosure drain must exclude the new status")
	repo.AssertNotCalled(t, "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}
