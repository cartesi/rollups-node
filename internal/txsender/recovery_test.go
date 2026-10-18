// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestRestartWithUnknownPoolBidAndLostReplyReachesPriceRetry(t *testing.T) {
	for _, test := range []struct {
		name                      string
		fullTip                   bool
		initialBase, recoveryBase int64
	}{
		{"unknown low tip", false, 200, 440},
		{"unknown full tip", true, 200, 440},
		{"quote rises after lost reply", true, 415, 860},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, signer := testSender(t)
			b.head.BaseFee, b.tip = big.NewInt(test.initialBase), big.NewInt(20)
			b.nonces[b.head.Hash()] = 0
			pool := poolRule(t)
			b.publishFn = func(raw []byte) error {
				if test.fullTip && len(b.published) == 1 {
					return testRPCError{replacementUnderpricedMessage, -32000}
				}
				return pool(raw)
			}
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			old := publishedTx(t, b, len(b.published)-1)
			require.Equal(t, big.NewInt(2*test.initialBase+20), old.GasFeeCap())
			if test.fullTip {
				require.Equal(t, old.GasFeeCap(), old.GasTipCap())
			}
			// Restart loses local bytes, but the pool still holds the old bid.
			s, err = New[int](b, signer, 31337, Policy{}, func(ctx context.Context) error { return ctx.Err() }, testLogger())
			require.NoError(t, err)
			b.advance(2, 0)
			b.head.BaseFee = big.NewInt(440)
			b.nonces[b.head.Hash()] = 0
			lost := true
			b.publishFn = func(raw []byte) error {
				if lost {
					lost = false
					return errLostReply // No admission. Retaining bytes is still required.
				}
				return pool(raw)
			}
			uncertain, err := s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, errLostReply)
			require.Equal(t, Uncertain, uncertain.Class)
			lostBytes := append([]byte(nil), s.current.raw...)
			// After several blocks, the fresh quote can differ from the retained
			// 900/20 bid. Retrying its cap as tip can still be too low.
			b.advance(16, 0)
			b.head.BaseFee = big.NewInt(test.recoveryBase)
			b.nonces[b.head.Hash()] = 0
			report, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, Accepted, report.Class)
			require.Equal(t, lostBytes, b.published[len(b.published)-2], "try retained bytes before the one retry")
			retry := publishedTx(t, b, len(b.published)-1)
			require.Equal(t, big.NewInt(2*test.recoveryBase+20), retry.GasFeeCap())
			require.Equal(t, retry.GasFeeCap(), retry.GasTipCap(), "a low-tip-only step cannot displace an unknown full-tip bid")
			require.Greater(t, retry.GasFeeCap().Cmp(b.head.BaseFee), 0, "the accepted bid is includable")
			before := signer.signs
			repeated, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, Known, repeated.Class)
			require.Equal(t, report.Hash, repeated.Hash)
			require.Equal(t, before, signer.signs, "ordinary polling must not keep increasing a repaired bid")
		})
	}
}

func TestCappedReplacementCanProgressAfterPoolEviction(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, evicted := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%t/evicted=%t", legacy, evicted), func(t *testing.T) {
				s, b, _ := testSender(t)
				s.policy.Legacy = legacy
				b.publishFn = poolRule(t)
				p := newPlanner()
				_, err := s.PublishNext(t.Context(), p)
				require.NoError(t, err)
				old := s.current
				s.policy.MaxFee = copyInt(old.cap)
				if evicted {
					b.publishFn = poolRule(t) // The empty pool cannot be inferred from our cache.
				}
				p.decision, p.eligibility = cancelDecision, Obsolete
				report, err := s.PublishNext(t.Context(), p)
				require.True(t, report.Attempted)
				require.True(t, report.Canceling)
				if evicted {
					require.NoError(t, err)
					require.Equal(t, Accepted, report.Class)
					require.True(t, s.current.neutral)
				} else {
					require.Error(t, err)
					require.Equal(t, ReplacementUnderpriced, report.Class)
					require.Same(t, old, s.current, "definite refusal retains the prior usable request")
				}
				for _, raw := range b.published {
					var tx types.Transaction
					require.NoError(t, tx.UnmarshalBinary(raw))
					require.LessOrEqual(t, tx.GasFeeCap().Cmp(s.policy.MaxFee), 0)
				}
			})
		}
	}
}

func TestLegacyReuseRepairsZeroEffectiveTip(t *testing.T) {
	s, b, _ := testSender(t)
	s.policy.Legacy, b.price, b.publishFn = true, big.NewInt(100), poolRule(t)
	p := newPlanner()
	first, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	b.advance(2, 0)
	b.head.BaseFee, b.price = big.NewInt(100), big.NewInt(105)
	b.nonces[b.head.Hash()] = 0
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.NotEqual(t, first.Hash, report.Hash)
	require.Equal(t, Accepted, report.Class)
	require.Equal(t, big.NewInt(110), s.current.cap)
	require.Equal(t, s.current.cap, s.current.tip)
}

func TestLegacyReplacementAfterDynamicQuoteKeepsEnvelopeConsistent(t *testing.T) {
	s, b, _ := testSender(t)
	b.publishFn = poolRule(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	b.head.BaseFee = nil
	b.nonces[b.head.Hash()] = 0
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, Accepted, report.Class)
	require.Equal(t, uint8(types.LegacyTxType), publishedTx(t, b, len(b.published)-1).Type())
	require.Equal(t, s.current.cap, s.current.tip)
}

func TestEmptyWalletReportsFundingBeforeEstimatorRefusal(t *testing.T) {
	s, b, signer := testSender(t)
	b.balance.SetInt64(0)
	b.estimateErr = testRPCError{unfundedEstimateMessage, -32000}
	_, err := s.PublishNext(t.Context(), newPlanner())
	require.ErrorIs(t, err, ErrLimit)
	require.ErrorContains(t, err, "signer balance")
	require.ErrorContains(t, err, "minimum funding before gas estimation")
	require.Empty(t, b.estimateBlocks)
	require.Zero(t, signer.signs)
	require.Empty(t, b.published)
}
