// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFreshDynamicQuoteAndSingleFullCapRetry(t *testing.T) {
	s, b, _ := testSender(t)
	b.publishFn = func([]byte) error {
		if len(b.published) == 1 {
			return testRPCError{replacementUnderpricedMessage, -32000}
		}
		return nil
	}
	report, err := s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	require.Equal(t, Accepted, report.Class)
	require.Len(t, b.published, 2)
	first, retry := publishedTx(t, b, 0), publishedTx(t, b, 1)
	require.Equal(t, big.NewInt(22), first.GasFeeCap())
	require.Equal(t, big.NewInt(2), first.GasTipCap())
	require.Equal(t, first.GasFeeCap(), retry.GasFeeCap())
	require.Equal(t, retry.GasFeeCap(), retry.GasTipCap())
}

func TestFreshTipAboveOperatorCeilingUsesActualClampedCap(t *testing.T) {
	s, b, _ := testSender(t)
	s.policy.MaxFee = big.NewInt(5)
	b.head.BaseFee, b.tip = big.NewInt(1), big.NewInt(10)
	b.nonces[b.head.Hash()] = 0
	_, err := s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	require.Len(t, b.published, 1)
	tx := publishedTx(t, b, 0)
	require.Equal(t, big.NewInt(5), tx.GasFeeCap())
	require.Equal(t, big.NewInt(5), tx.GasTipCap())
}

func TestDynamicRetryUsesActualClampedCapAndNeverCompounds(t *testing.T) {
	s, b, _ := testSender(t)
	s.policy.MaxFee = big.NewInt(15)
	b.publishErr = testRPCError{replacementUnderpricedMessage, -32000}
	for range 3 {
		_, err := s.PublishNext(t.Context(), newPlanner())
		require.Error(t, err)
	}
	require.Len(t, b.published, 6)
	for index := range b.published {
		tx := publishedTx(t, b, index)
		require.Equal(t, big.NewInt(15), tx.GasFeeCap())
		if index%2 == 0 {
			require.Equal(t, big.NewInt(2), tx.GasTipCap())
		} else {
			require.Equal(t, big.NewInt(15), tx.GasTipCap())
		}
	}
	require.Nil(t, s.current, "a definite refusal establishes no preferred newly published transaction")
}

func TestLegacyRetryUsesOneFreshRoundedStep(t *testing.T) {
	for _, test := range []struct {
		quote int64
		cap   int64
		want  int64
	}{
		{1, 0, 2}, {10, 0, 11}, {101, 0, 112}, {101, 110, 110}, {101, 101, 101},
	} {
		t.Run(big.NewInt(test.quote).String()+"/"+big.NewInt(test.cap).String(), func(t *testing.T) {
			s, b, _ := testSender(t)
			s.policy.Legacy, b.price = true, big.NewInt(test.quote)
			b.head.BaseFee = nil
			b.nonces[b.head.Hash()] = 0
			if test.cap > 0 {
				s.policy.MaxFee = big.NewInt(test.cap)
			}
			b.publishErr = testRPCError{replacementUnderpricedMessage, -32000}
			_, err := s.PublishNext(t.Context(), newPlanner())
			require.Error(t, err)
			last := publishedTx(t, b, len(b.published)-1)
			require.Equal(t, big.NewInt(test.want), last.GasPrice())
			if test.cap == test.quote {
				require.Len(t, b.published, 1, "an identical retry has no benefit")
			}
		})
	}
}

func TestLegacySmallQuoteChangeRetainsBytesUntilReplacementIsPossible(t *testing.T) {
	s, b, _ := testSender(t)
	s.policy.Legacy = true
	b.price = big.NewInt(100)
	b.publishFn = poolRule(t)
	_, err := s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	b.price = big.NewInt(101)
	_, err = s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	require.Equal(t, b.published[0], b.published[1], "a small quote change does not require a replacement")
	b.price = big.NewInt(110)
	_, err = s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	require.Equal(t, big.NewInt(110), publishedTx(t, b, 2).GasPrice())
}

func TestZeroQuoteAndCeilingBelowBaseRefuseExplicitly(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*Sender[int], *fakeBackend)
		want  error
	}{
		{"zero legacy quote", func(s *Sender[int], b *fakeBackend) { s.policy.Legacy = true; b.price.SetInt64(0) }, ErrInconclusive},
		{"zero dynamic quote", func(_ *Sender[int], b *fakeBackend) {
			b.head.BaseFee.SetInt64(0)
			b.tip.SetInt64(0)
			b.nonces[b.head.Hash()] = 0
		}, ErrInconclusive},
		{"ceiling below base", func(s *Sender[int], _ *fakeBackend) { s.policy.MaxFee = big.NewInt(9) }, ErrLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, f := testSender(t)
			test.setup(s, b)
			_, err := s.PublishNext(t.Context(), newPlanner())
			require.ErrorIs(t, err, test.want)
			require.Zero(t, f.signs)
			require.Empty(t, b.published)
		})
	}
}
