// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

// poolRule independently applies geth's default replacement checks. Repeating
// identical bytes is known; both price fields of a replacement must increase.
func poolRule(t *testing.T) func([]byte) error {
	t.Helper()
	var pending *types.Transaction
	return func(raw []byte) error {
		tx := new(types.Transaction)
		require.NoError(t, tx.UnmarshalBinary(raw))
		if pending != nil {
			if tx.Hash() == pending.Hash() {
				return testRPCError{knownPublicationMessage, -32000}
			}
			for _, pair := range [][2]*big.Int{{tx.GasFeeCap(), pending.GasFeeCap()}, {tx.GasTipCap(), pending.GasTipCap()}} {
				threshold := new(big.Int).Div(new(big.Int).Mul(pair[1], big.NewInt(110)), big.NewInt(100))
				if pair[0].Cmp(pair[1]) <= 0 || pair[0].Cmp(threshold) < 0 {
					return testRPCError{replacementUnderpricedMessage, -32000}
				}
			}
		}
		pending = tx
		return nil
	}
}

func TestSmallDynamicMarketMovesRebroadcastCachedBytes(t *testing.T) {
	for _, base := range []int64{80, 100, 105, 109, 115} {
		t.Run(fmt.Sprint(base), func(t *testing.T) {
			s, b, signer := testSender(t)
			b.head.BaseFee, b.tip = big.NewInt(100), big.NewInt(10)
			b.nonces[b.head.Hash()] = 0
			b.publishFn = poolRule(t)
			p := newPlanner()
			first, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			b.advance(2, 0)
			b.head.BaseFee, b.tip = big.NewInt(base), big.NewInt(11)
			b.nonces[b.head.Hash()] = 0
			second, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			if base < 115 {
				require.Equal(t, first.Hash, second.Hash)
				require.Equal(t, Known, second.Class)
				require.Equal(t, 1, signer.signs)
			} else {
				require.NotEqual(t, first.Hash, second.Hash)
				require.Equal(t, Accepted, second.Class)
				require.Equal(t, 2, signer.signs)
			}
			require.Len(t, b.published, 2)
		})
	}
}

func TestCancellationAndUrgentReplacementClearFlatMarketPool(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, cancel := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%t/cancel=%t", legacy, cancel), func(t *testing.T) {
				s, b, signer := testSender(t)
				s.policy.Legacy, b.publishFn = legacy, poolRule(t)
				p := newPlanner()
				_, err := s.PublishNext(t.Context(), p)
				require.NoError(t, err)
				if cancel {
					p.decision, p.eligibility = cancelDecision, Obsolete
				} else {
					p.action = 2
				}
				second, err := s.PublishNext(t.Context(), p)
				require.NoError(t, err)
				require.Equal(t, Accepted, second.Class)
				require.Equal(t, cancel, second.Current.Neutral)
				for range 3 {
					repeated, repeatErr := s.PublishNext(t.Context(), p)
					require.NoError(t, repeatErr)
					require.Equal(t, second.Hash, repeated.Hash, "replacement is not bumped again on every poll")
				}
				require.Equal(t, 2, signer.signs)
				require.Len(t, b.published, 5)
			})
		}
	}
}

func TestReplacementCeilingAttemptsFreshQuoteAndPreservesCurrentOnRefusal(t *testing.T) {
	s, b, signer := testSender(t)
	b.publishFn = poolRule(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	old := s.current
	s.policy.MaxFee = big.NewInt(24) // Current cap22 requires25, including integer rounding.
	p.decision, p.eligibility = cancelDecision, Obsolete
	_, err = s.PublishNext(t.Context(), p)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrLimit, "only the provider can establish that the older copy is still in its pool")
	require.Same(t, old, s.current)
	require.Len(t, b.published, 3)
	require.Equal(t, 3, signer.signs)
	for _, raw := range b.published {
		var tx types.Transaction
		require.NoError(t, tx.UnmarshalBinary(raw))
		require.LessOrEqual(t, tx.GasFeeCap().Cmp(s.policy.MaxFee), 0)
	}
}

func TestCachedFullCapBidDoesNotBlockWhenBaseFeeOutgrowsIt(t *testing.T) {
	s, b, signer := testSender(t)
	pool := poolRule(t)
	b.publishFn = func(raw []byte) error {
		if len(b.published) == 1 {
			return testRPCError{replacementUnderpricedMessage, -32000}
		}
		return pool(raw)
	}
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	old := publishedTx(t, b, 1)
	require.Equal(t, old.GasFeeCap(), old.GasTipCap())
	b.advance(2, 0)
	b.head.BaseFee = big.NewInt(23) // Old22-wei cap no longer covers base fee.
	b.nonces[b.head.Hash()] = 0
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, Accepted, report.Class)
	replacement := publishedTx(t, b, 2)
	require.Equal(t, big.NewInt(48), replacement.GasFeeCap())
	require.Equal(t, big.NewInt(25), replacement.GasTipCap())
	require.Equal(t, 3, signer.signs)
	require.Len(t, b.published, 3)
}

func TestCachedCapClippingQuotedTipGetsOneMinimumReplacement(t *testing.T) {
	for _, base := range []int64{21, 22} {
		t.Run(fmt.Sprint(base), func(t *testing.T) {
			s, b, signer := testSender(t)
			b.publishFn = poolRule(t)
			p := newPlanner()
			first, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			b.advance(2, 0)
			b.head.BaseFee = big.NewInt(base)
			b.nonces[b.head.Hash()] = 0
			second, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, Accepted, second.Class)
			require.NotEqual(t, first.Hash, second.Hash)
			tx := publishedTx(t, b, 1)
			require.Equal(t, big.NewInt(2*base+2), tx.GasFeeCap())
			require.Equal(t, big.NewInt(3), tx.GasTipCap(), "use the pool minimum rather than paying the full cap")
			for range 3 {
				repeated, repeatErr := s.PublishNext(t.Context(), p)
				require.NoError(t, repeatErr)
				require.Equal(t, second.Hash, repeated.Hash)
				require.Equal(t, Known, repeated.Class)
			}
			require.Equal(t, 2, signer.signs, "an adequate replacement is not bumped on later polls")
		})
	}
}

func TestClippedTipReplacementHonorsOperatorCeiling(t *testing.T) {
	for _, ceiling := range []int64{24, 25} {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			s, b, signer := testSender(t)
			s.policy.MaxFee = big.NewInt(ceiling)
			b.publishFn = poolRule(t)
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			b.advance(2, 0)
			b.head.BaseFee = big.NewInt(22)
			b.nonces[b.head.Hash()] = 0
			for range 3 {
				_, publishErr := s.PublishNext(t.Context(), p)
				require.NoError(t, publishErr)
				require.Equal(t, big.NewInt(ceiling), s.current.cap)
				if ceiling == 24 {
					require.Equal(t, big.NewInt(24), s.current.tip, "one full-cap retry clears geth's integer threshold")
				} else {
					require.Equal(t, big.NewInt(3), s.current.tip)
				}
			}
			require.Equal(t, map[int64]int{24: 3, 25: 2}[ceiling], signer.signs)
		})
	}
}

func TestCachedReplacementRefusalWaitsButMixedTransportFailureRemainsAnError(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			s, b, signer := testSender(t)
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			b.publishErr = testRPCError{replacementUnderpricedMessage, -32000}
			if mixed {
				b.publishErr = errors.Join(b.publishErr, errLostReply)
			}
			report, err := s.PublishNext(t.Context(), p)
			if mixed {
				require.ErrorIs(t, err, errLostReply)
				require.Equal(t, Uncertain, report.Class)
			} else {
				require.NoError(t, err)
				require.Equal(t, ReplacementUnderpriced, report.Class)
			}
			require.Equal(t, b.published[0], b.published[1])
			if mixed {
				require.Equal(t, 1, signer.signs)
				require.Len(t, b.published, 2)
			} else {
				require.Equal(t, 2, signer.signs, "a definite refusal permits one fresh-quote retry")
				require.Len(t, b.published, 3)
			}
		})
	}
}

func TestRefusedCachedPriceRetryIsStillAnOrdinaryWait(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	old := s.current
	b.publishFn = func([]byte) error {
		if len(b.published) == 2 {
			return testRPCError{admissionUnderpricedMessage, -32000}
		}
		return testRPCError{replacementUnderpricedMessage, -32000}
	}
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, ReplacementUnderpriced, report.Class)
	require.Same(t, old, s.current)
	require.Len(t, b.published, 3)
}

func TestFinalPublicationGuardChecksCachedAndNewBytes(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprint(retained), func(t *testing.T) {
			s, b, signer := testSender(t)
			p := newPlanner()
			if retained {
				_, err := s.PublishNext(t.Context(), p)
				require.NoError(t, err)
			}
			before := len(b.published)
			checks := 0
			s.guard = func(context.Context) error {
				checks++
				if checks == 3 {
					return ErrIneligible
				}
				return nil
			}
			report, err := s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, ErrIneligible)
			require.False(t, report.Attempted)
			require.Len(t, b.published, before)
			require.Equal(t, 3, checks)
			require.Equal(t, 1, signer.signs)
		})
	}
}

func TestCancellationCannotPublishWhenPostSignRecheckFindsCurrentUseful(t *testing.T) {
	s, b, signer := testSender(t)
	p := newPlanner()
	first, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	old := s.current
	p.decision, p.eligibility = cancelDecision, Obsolete
	signer.afterSign = func() { p.eligibility = Useful }
	report, err := s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, ErrIneligible)
	require.False(t, report.Attempted)
	require.Same(t, old, s.current)
	require.False(t, report.Current.Neutral)
	require.Equal(t, first.Hash, s.current.hash)
	require.Len(t, b.published, 1)
	require.Equal(t, 2, signer.signs, "eligibility changed while the neutral was being signed")
}

func TestReadCancellationStaysPureWhileJoinedFailuresRemainVisible(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			s, b, _ := testSender(t)
			b.headerErr = fmt.Errorf("in-flight RPC: %w", context.Canceled)
			if mixed {
				b.headerErr = errors.Join(b.headerErr, errLostReply)
			}
			_, err := s.PublishNext(t.Context(), newPlanner())
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, !mixed, errutil.IsOnlyCancellation(err))
			require.Equal(t, mixed, errors.Is(err, ErrInconclusive))
			if mixed {
				require.ErrorIs(t, err, errLostReply)
			}
			require.Empty(t, b.published)
		})
	}
}
