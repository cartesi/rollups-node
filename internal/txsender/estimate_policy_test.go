// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/stretchr/testify/require"
)

func TestZeroFeeEstimationDoesNotChangeExecutionOrSignedFees(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, optIn := range []bool{false, true} {
			for _, value := range []int64{0, 77} {
				t.Run(fmt.Sprintf("legacy=%t/opt_in=%t/value=%d", legacy, optIn, value), func(t *testing.T) {
					s, b, _ := testSender(t)
					s.policy.Legacy = legacy
					p := newPlanner()
					p.request.Value = big.NewInt(value)
					p.request.EstimateWithZeroFees = optIn
					feeCap, tip := big.NewInt(22), big.NewInt(2)
					if legacy {
						feeCap, tip = big.NewInt(12), big.NewInt(12)
					}
					b.estimateFn = func(msg ethereum.CallMsg) (uint64, error) {
						require.Equal(t, p.request.Value, msg.Value)
						require.Equal(t, p.request.Data, msg.Data)
						require.Equal(t, p.request.To, *msg.To)
						if legacy {
							want := feeCap
							if optIn {
								want = new(big.Int)
							}
							require.Equal(t, want, msg.GasPrice)
							require.Nil(t, msg.GasFeeCap)
						} else {
							wantCap, wantTip := feeCap, tip
							if optIn {
								wantCap, wantTip = new(big.Int), new(big.Int)
							}
							require.Equal(t, wantCap, msg.GasFeeCap)
							require.Equal(t, wantTip, msg.GasTipCap)
							require.Nil(t, msg.GasPrice)
						}
						return b.estimate, nil
					}
					report, err := s.PublishNext(t.Context(), p)
					require.NoError(t, err)
					require.True(t, report.Attempted)
					require.NotEmpty(t, b.estimateBlocks)
					require.NotEmpty(t, b.calls)
					for _, msg := range b.calls {
						if legacy {
							require.Equal(t, feeCap, msg.GasPrice)
						} else {
							require.Equal(t, feeCap, msg.GasFeeCap)
							require.Equal(t, tip, msg.GasTipCap)
						}
					}
					tx := publishedTx(t, b, 0)
					require.Equal(t, feeCap, tx.GasFeeCap())
					require.Equal(t, tip, tx.GasTipCap())
					require.Equal(t, p.request.Value, tx.Value())
				})
			}
		}
	}
}

func TestZeroFeeEstimationPreservesExecutionRefusals(t *testing.T) {
	s, b, signer := testSender(t)
	p := newPlanner()
	p.request.EstimateWithZeroFees = true
	refusal := testRPCError{"execution reverted: invalid commitment", 3}
	b.estimateErr = refusal
	_, err := s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, refusal)
	require.ErrorContains(t, err, "gas estimate refused")
	require.NotErrorIs(t, err, ErrLimit)
	require.Zero(t, signer.signs)
	require.Empty(t, b.published)
}

func TestCancellationDoesNotInheritZeroFeeEstimation(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	p.request.EstimateWithZeroFees = true
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	b.estimateFn = func(msg ethereum.CallMsg) (uint64, error) {
		require.Positive(t, msg.GasFeeCap.Sign())
		require.Positive(t, msg.GasTipCap.Sign())
		return b.estimate, nil
	}
	p.decision, p.eligibility = cancelDecision, Obsolete
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.True(t, report.Canceling)
}
