// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestCanonicalCollectionChecksFinalEntry(t *testing.T) {
	root := common.HexToHash("0x1234")
	final := root
	require.NoError(t, validateCanonicalInputHashCollection(final, root))
	wrong := common.HexToHash("0x5678")
	err := validateCanonicalInputHashCollection(wrong, root)
	require.ErrorContains(t, err, final.Hex())
	require.ErrorContains(t, err, wrong.Hex())
	require.ErrorContains(t, err, "deterministic")
}

func (s *MachineInstanceSuite) TestFinalEntryUsesCanonicalCompletionProof() {
	for _, status := range []machine.CompletionStatus{
		machine.CompletionStatusAccepted, machine.CompletionStatusRejected, machine.CompletionStatusHalted,
	} {
		for _, wrong := range []bool{false, true} {
			s.Run(fmt.Sprintf("status=%d/wrong=%t", status, wrong), func() {
				inner, fork, instance := s.setupAdvance()
				pre := acceptedStateProof(newHash(0x11), newHash(0x12))
				post := acceptedStateProof(newHash(0x21), newHash(0x22))
				proofCalls := 0
				fork.StateProofReturn = post
				fork.StateProofFunc = func(context.Context) (*machine.StateProof, error) {
					proofCalls++
					if proofCalls == 1 {
						return pre, nil
					}
					return post, nil
				}
				fork.CompletionStatusReturn = status
				fork.AdvanceLeafsReturn = []machine.Hash{newHash(0x31)}
				fork.AdvanceRemainingReturn = model.InputHashCollectionCapacity - 1
				canonical, other := post.MachineHash, pre.MachineHash
				if status == machine.CompletionStatusRejected {
					canonical, other = pre.MachineHash, post.MachineHash
				}
				entry := machine.Hash(model.FinalStateEntry(common.Hash(canonical)))
				closeErr := errors.New("fork close failed")
				if wrong {
					entry = model.FinalStateEntry(common.Hash(other))
					fork.CloseError = closeErr
				} else {
					fork.CloseError = nil
				}
				fork.AdvanceFinalStateEntryReturn = &entry
				result, err := instance.Advance(s.T().Context(), []byte("input"), 2, 5, true)
				if wrong {
					s.ErrorIs(err, closeErr)
					s.Nil(result)
					s.ErrorContains(err, common.Hash(entry).Hex())
					s.ErrorContains(err, model.FinalStateEntry(common.Hash(canonical)).Hex())
					s.Same(inner, instance.runtime)
					s.Equal(uint64(5), instance.ProcessedInputs())
					s.Equal(int64(1), fork.CloseCalls.Load())
					s.Zero(inner.CloseCalls.Load())
					return
				}
				s.Require().NoError(err)
				s.Require().NotNil(result)
				s.Equal(common.Hash(canonical), result.MachineHash)
				s.Equal(uint64(6), instance.ProcessedInputs())
				switch status {
				case machine.CompletionStatusAccepted:
					s.Same(fork, instance.runtime)
					s.Equal(int64(1), inner.CloseCalls.Load())
					s.Zero(fork.CloseCalls.Load())
				case machine.CompletionStatusRejected:
					s.Same(inner, instance.runtime)
					s.Zero(inner.CloseCalls.Load())
					s.Equal(int64(1), fork.CloseCalls.Load())
				case machine.CompletionStatusHalted:
					s.Nil(instance.runtime)
					s.Equal(int64(1), inner.CloseCalls.Load())
					s.Equal(int64(1), fork.CloseCalls.Load())
				case machine.CompletionStatusUnknown, machine.CompletionStatusException,
					machine.CompletionStatusOverflow, machine.CompletionStatusUnexpectedYield, machine.CompletionStatusInvalidOutputsRoot:
					s.FailNow("unexpected completion status in final-entry fixture")
				}
				wantProofCalls := 2
				if status == machine.CompletionStatusRejected {
					wantProofCalls = 1
				}
				s.Equal(wantProofCalls, proofCalls)
			})
		}
	}
}
