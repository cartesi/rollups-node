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

func TestServiceGasFloorDoesNotHideL2PostingEstimate(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	p.request.GasLimit = 650_000
	b.estimate = 1_200_000 // Execution plus L1 posting cost, expressed in L2 gas.
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, uint64(1_800_000), publishedTx(t, b, 0).Gas())
}

func TestGasSizingUsesPreparedBlockEvenWhenSigningCrossesAHead(t *testing.T) {
	s, b, signer := testSender(t)
	prepared := BlockRef{Number: b.head.Number.Uint64(), Hash: b.head.Hash()}
	signer.afterSign = func() { b.advance(2, 0) }
	report, err := s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Equal(t, []BlockRef{prepared}, b.estimateBlocks)
	require.Equal(t, prepared, b.callBlocks[0])
	require.Equal(t, uint64(2), b.callBlocks[len(b.callBlocks)-1].Number)
}

func TestNeutralUsesEstimatedPostingGasWithoutContractBranchPadding(t *testing.T) {
	for _, estimate := range []uint64{21_000, 40_000} {
		t.Run(new(big.Int).SetUint64(estimate).String(), func(t *testing.T) {
			s, b, _ := testSender(t)
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			b.estimate = estimate
			p.decision, p.eligibility = cancelDecision, Obsolete
			_, err = s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, estimate, publishedTx(t, b, 1).Gas())
		})
	}
}

func TestGasEstimateRefusalIsNotReclassifiedAsInsufficientBalance(t *testing.T) {
	for _, refusal := range []testRPCError{
		{"execution reverted: action is not useful", 3},
		{unfundedEstimateMessage, -32000},
	} {
		t.Run(refusal.Error(), func(t *testing.T) {
			s, b, signer := testSender(t)
			b.estimateErr = refusal
			_, err := s.PublishNext(t.Context(), newPlanner())
			require.ErrorIs(t, err, refusal)
			require.ErrorContains(t, err, "gas estimate refused")
			require.NotContains(t, err.Error(), "gas estimate unavailable")
			require.NotErrorIs(t, err, ErrLimit)
			require.Zero(t, signer.signs)
			require.Empty(t, b.published)
		})
	}
}

func TestGasEstimateUnavailableDoesNotInventANewAllowance(t *testing.T) {
	s, b, signer := testSender(t)
	b.estimateErr = errLostReply
	b.balance = big.NewInt(22_000_000)
	_, err := s.PublishNext(t.Context(), newPlanner())
	require.ErrorIs(t, err, errLostReply)
	require.ErrorContains(t, err, "gas estimate unavailable")
	require.NotContains(t, err.Error(), "gas estimate refused")
	require.NotErrorIs(t, err, ErrLimit)
	require.Zero(t, signer.signs)
	require.Empty(t, b.published)
}

func TestGasEstimateAboveSupportedCeilingRefusesBeforeSigning(t *testing.T) {
	s, b, signer := testSender(t)
	b.estimate = supportedTransactionGasCeiling + 1
	_, err := s.PublishNext(t.Context(), newPlanner())
	require.ErrorIs(t, err, ErrLimit)
	require.Zero(t, signer.signs)
	require.Empty(t, b.published)
}

func TestCachedGasIsReusedForEstimateJitterButReplacedWhenInsufficient(t *testing.T) {
	for _, test := range []struct {
		name     string
		estimate uint64
		floor    uint64
		reuse    bool
	}{
		{"padding jitter", 21_001, 0, true},
		{"L2 posting increase", 180_000, 0, false},
		{"service floor increase", 21_000, 200_000, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, signer := testSender(t)
			b.publishFn = poolRule(t)
			p := newPlanner()
			first, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			b.estimate, p.request.GasLimit = test.estimate, test.floor
			second, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			if test.reuse {
				require.Equal(t, first.Hash, second.Hash)
				require.Equal(t, 1, signer.signs)
			} else {
				require.NotEqual(t, first.Hash, second.Hash)
				require.Equal(t, 2, signer.signs)
				require.Equal(t, Accepted, second.Class, "changed gas needs a bid the pool can replace")
				require.GreaterOrEqual(t, publishedTx(t, b, 1).Gas(), max(test.estimate, test.floor))
			}
			require.Len(t, b.published, 2)
		})
	}
}

func TestEstimatorOutageReusesHeldGasWithoutIncreasingFunding(t *testing.T) {
	for _, neutral := range []bool{false, true} {
		t.Run(fmt.Sprintf("neutral=%t", neutral), func(t *testing.T) {
			s, b, signer := testSender(t)
			b.publishFn = poolRule(t)
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			if neutral {
				p.decision, p.eligibility = cancelDecision, Obsolete
				_, err = s.PublishNext(t.Context(), p)
				require.NoError(t, err)
			}
			old := s.current
			signs := signer.signs
			b.balance.Mul(old.cap, new(big.Int).SetUint64(old.gas)).Add(b.balance, old.request.Value)
			b.estimateErr = errLostReply
			report, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, Known, report.Class)
			require.Equal(t, old.raw, s.current.raw)
			require.Equal(t, old.gas, s.current.gas)
			require.Equal(t, signs, signer.signs)
			// Keeping gas never bypasses the exact execution check.
			b.callFn = func(ethereum.CallMsg) ([]byte, error) { return nil, errLostReply }
			before := len(b.published)
			_, err = s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, errLostReply)
			require.Len(t, b.published, before)
		})
	}
}

func TestEstimatorOutageCannotSizeChangedWork(t *testing.T) {
	for _, kind := range []string{"new action", "cancellation", "higher service floor"} {
		t.Run(kind, func(t *testing.T) {
			s, b, signer := testSender(t)
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			old := s.current
			b.estimateErr = errLostReply
			switch kind {
			case "new action":
				p.action = 2
			case "cancellation":
				p.decision, p.eligibility = cancelDecision, Obsolete
			default:
				p.request.GasLimit = old.gas + 1
			}
			report, err := s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, errLostReply)
			require.False(t, report.Attempted)
			require.Same(t, old, s.current)
			require.Equal(t, 1, signer.signs)
			require.Len(t, b.published, 1)
		})
	}
}

func TestFreshQuoteDoesNotRejectFundsSufficientForReusableBytes(t *testing.T) {
	for _, neutral := range []bool{false, true} {
		t.Run(fmt.Sprintf("neutral=%t", neutral), func(t *testing.T) {
			s, b, _ := testSender(t)
			p := newPlanner()
			p.request.GasLimit = 200_000
			b.publishFn = poolRule(t)
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			if neutral {
				p.decision, p.eligibility = cancelDecision, Obsolete
				_, err = s.PublishNext(t.Context(), p)
				require.NoError(t, err)
			}
			old := s.current
			b.balance.Mul(old.cap, new(big.Int).SetUint64(old.gas))
			b.advance(2, 0)
			b.head.BaseFee.SetInt64(12) // Higher fresh cap; unchanged tip cannot replace adequate held bytes.
			b.nonces[b.head.Hash()] = 0
			estimateRefusal := testRPCError{unfundedEstimateMessage, -32000}
			b.estimateFn = func(msg ethereum.CallMsg) (uint64, error) {
				cost := new(big.Int).Mul(msg.GasFeeCap, new(big.Int).SetUint64(old.gas))
				if cost.Cmp(b.balance) > 0 {
					return 0, estimateRefusal
				}
				return b.estimate, nil
			}
			report, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, Known, report.Class)
			require.Equal(t, old.raw, s.current.raw)
			// Once the retained quote is inadequate, sizing must use the fresh
			// quote and report its refusal rather than reuse the cheaper fees.
			b.advance(3, 0)
			b.head.BaseFee.Add(old.cap, big.NewInt(1))
			b.nonces[b.head.Hash()] = 0
			before := len(b.published)
			report, err = s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, estimateRefusal)
			require.ErrorContains(t, err, "gas estimate refused")
			require.False(t, report.Attempted)
			require.Len(t, b.published, before)
			require.Equal(t, old.raw, s.current.raw)
		})
	}
}
