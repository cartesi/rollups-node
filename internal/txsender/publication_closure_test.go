// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRisingBaseWithConstantTipReusesAdequateBytes(t *testing.T) {
	for _, base := range []int64{104, 120, 150} {
		t.Run(big.NewInt(base).String(), func(t *testing.T) {
			s, b, signer := testSender(t)
			b.head.BaseFee, b.tip = big.NewInt(100), big.NewInt(10)
			b.nonces[b.head.Hash()] = 0
			b.publishFn = poolRule(t)
			p := newPlanner()
			first, err := s.PublishNext(t.Context(), p) // cap 210, tip 10
			require.NoError(t, err)
			b.advance(2, 0)
			b.head.BaseFee = big.NewInt(base)
			b.nonces[b.head.Hash()] = 0
			second, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, first.Hash, second.Hash, "adequate cached bytes are rebroadcast")
			require.Equal(t, Known, second.Class)
			require.Equal(t, 1, signer.signs, "no signature for a quote the pool must refuse")
			require.Len(t, b.published, 2)
		})
	}
}

func TestRisingQuotedTipUsesSmallerTipForAdequacy(t *testing.T) {
	s, b, signer := testSender(t)
	b.head.BaseFee, b.tip = big.NewInt(100), big.NewInt(100)
	b.nonces[b.head.Hash()] = 0
	b.publishFn = poolRule(t)
	p := newPlanner()
	first, err := s.PublishNext(t.Context(), p) // cap 300, tip 100
	require.NoError(t, err)
	b.advance(2, 0)
	b.head.BaseFee, b.tip = big.NewInt(196), big.NewInt(105)
	b.nonces[b.head.Hash()] = 0
	second, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, first.Hash, second.Hash)
	require.Equal(t, Known, second.Class)
	require.Equal(t, 1, signer.signs)
}

func TestChangedIntentPriceRetryKeepsCachedReplacementFloor(t *testing.T) {
	for _, refusal := range []string{admissionUnderpricedMessage, replacementUnderpricedMessage} {
		t.Run(refusal, func(t *testing.T) {
			s, b, signer := testSender(t)
			pool := poolRule(t)
			b.publishFn = pool
			p := newPlanner()
			_, err := s.PublishNext(t.Context(), p) // cap 22, tip 2
			require.NoError(t, err)
			b.publishFn = func(raw []byte) error {
				if len(b.published) == 2 {
					return testRPCError{refusal, -32000} // first replacement POST is refused
				}
				return pool(raw)
			}
			p.decision, p.eligibility = cancelDecision, Obsolete
			report, err := s.PublishNext(t.Context(), p)
			require.NoError(t, err)
			require.Equal(t, Accepted, report.Class)
			retry := publishedTx(t, b, 2)
			require.Equal(t, big.NewInt(25), retry.GasFeeCap(), "retry keeps the bumped cap")
			require.Equal(t, big.NewInt(25), retry.GasTipCap(), "retry uses that cap as tip")
			require.Equal(t, 3, signer.signs)
		})
	}
}

func TestZeroTipCancellationMakesStrictReplacementIncrease(t *testing.T) {
	s, b, _ := testSender(t)
	b.tip = big.NewInt(0)
	b.publishFn = poolRule(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Zero(t, publishedTx(t, b, 0).GasTipCap().Sign())
	p.decision, p.eligibility = cancelDecision, Obsolete
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, Accepted, report.Class)
	require.True(t, report.Current.Neutral)
	require.Equal(t, big.NewInt(1), publishedTx(t, b, 1).GasTipCap())
}
