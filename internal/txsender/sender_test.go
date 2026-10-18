// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestLostReplyRetainsAndRebroadcastsIdenticalBytes(t *testing.T) {
	s, b, signer := testSender(t)
	p := newPlanner()
	b.publishErr = errLostReply
	first, err := s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, errLostReply)
	require.True(t, first.Attempted)
	require.Equal(t, Uncertain, first.Class)
	require.Equal(t, &Current[int]{Action: 1}, first.Current)
	b.publishErr = testRPCError{knownPublicationMessage, -32000}
	second, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, Known, second.Class)
	require.Equal(t, first.Hash, second.Hash)
	require.Equal(t, b.published[0], b.published[1])
	require.Equal(t, 1, signer.signs)
}

func TestConsumptionRetiresImmediatelyWithoutFinalizedOrReceiptReads(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	b.advance(2, 1)
	p.action = 2
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, uint64(1), report.Nonce)
	require.Nil(t, p.seen[1])
	require.Equal(t, 2, report.Current.Action)
	for _, tag := range b.tags {
		require.Equal(t, latestBlockTag, tag)
	}
}

func TestCrashAndReorgReconstructAtCurrentMinedNonce(t *testing.T) {
	s, b, f := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	b.advance(2, 1)
	p.action = 2
	_, err = s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	oldHigher := publishedTx(t, b, 1)
	// The distributed nonce-1 copy remains outside the cache after this reorg.
	b.advance(2, 0)
	b.head.Extra = []byte{2}
	b.nonces[b.head.Hash()] = 0
	p.action = 1
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, uint64(0), report.Nonce)
	require.Equal(t, uint64(1), oldHigher.Nonce())
	require.Nil(t, p.seen[2])
	// Restart loses the remaining cache too; chain and computed work suffice.
	restarted, err := New[int](b, f, 31337, Policy{}, func(context.Context) error { return nil }, testLogger())
	require.NoError(t, err)
	_, err = restarted.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.Nil(t, p.seen[3])
}

func TestSigningAcrossForwardBlocksPublishesWithoutQuietChain(t *testing.T) {
	s, b, f := testSender(t)
	p := newPlanner()
	f.afterSign = func() { b.advance(b.head.Number.Uint64()+1, 0) }
	p.recheckFn = func(block BlockRef, action int) (Eligibility, error) {
		require.Equal(t, uint64(2), block.Number)
		require.Equal(t, 1, action)
		return Useful, nil
	}
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Equal(t, 1, f.signs)
	require.Equal(t, uint64(2), b.callBlocks[len(b.callBlocks)-1].Number)
}

func TestPostSignChangesNeverPublish(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Sender[int], *fakeBackend, *fakeSigner, *fakePlanner)
		want   error
	}{
		{"reorg", func(_ *Sender[int], b *fakeBackend, f *fakeSigner, _ *fakePlanner) {
			f.afterSign = func() { b.head.Extra = []byte{2}; b.nonces[b.head.Hash()] = 0 }
		}, ErrNonCanonical},
		{"nonce consumed", func(_ *Sender[int], b *fakeBackend, f *fakeSigner, _ *fakePlanner) {
			f.afterSign = func() { b.advance(2, 1) }
		}, ErrStale},
		{"read failed", func(_ *Sender[int], b *fakeBackend, f *fakeSigner, _ *fakePlanner) {
			f.afterSign = func() { b.nonceErr = errLostReply }
		}, ErrInconclusive},
		{"local eligibility", func(_ *Sender[int], _ *fakeBackend, f *fakeSigner, p *fakePlanner) {
			f.afterSign = func() { p.eligibility = Obsolete }
		}, ErrIneligible},
		{"publication stopped", func(s *Sender[int], _ *fakeBackend, f *fakeSigner, _ *fakePlanner) {
			f.afterSign = func() { s.guard = func(context.Context) error { return ErrIneligible } }
		}, ErrIneligible},
		{"balance lost", func(_ *Sender[int], b *fakeBackend, f *fakeSigner, _ *fakePlanner) {
			f.afterSign = func() { b.balance.SetUint64(0) }
		}, ErrLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, f := testSender(t)
			p := newPlanner()
			test.change(s, b, f, p)
			report, err := s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, test.want)
			require.False(t, report.Attempted)
			require.Empty(t, b.published)
		})
	}
}

func TestWaitAndFailedEvidenceNeverCancelCurrent(t *testing.T) {
	s, b, f := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	p.decision = waitDecision
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.False(t, report.Attempted)
	p.nextErr = errLostReply
	_, err = s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, errLostReply)
	require.Len(t, b.published, 1)
	require.Equal(t, 1, f.signs)
	require.Equal(t, 1, s.current.action)
}

func TestPositiveCancellationIsReplaceableByUsefulWork(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	p.decision, p.eligibility = cancelDecision, Obsolete
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.True(t, report.Current.Neutral)
	neutral := publishedTx(t, b, 1)
	require.Equal(t, s.From(), *neutral.To())
	require.Zero(t, neutral.Value().Sign())
	require.Empty(t, neutral.Data())
	p.decision, p.action, p.eligibility = publishDecision, 2, Useful
	report, err = s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.False(t, report.Current.Neutral)
	require.Equal(t, 2, report.Current.Action)
}

func TestFreshCancelIsIdleAndUnknownCancellationCannotPost(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	p.decision = cancelDecision
	report, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	require.False(t, report.Attempted)
	p.decision = publishDecision
	_, err = s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	p.decision, p.eligibility = cancelDecision, ProtocolWait
	_, err = s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, ErrInconclusive)
	require.Len(t, b.published, 1)
}

func TestExactSimulationUsesEveryFieldAndPaddedGas(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	p.request.Value = big.NewInt(77)
	b.estimate = 170_396
	b.callFn = func(msg ethereum.CallMsg) ([]byte, error) {
		require.Equal(t, s.From(), msg.From)
		require.Equal(t, p.request.To, *msg.To)
		require.Equal(t, p.request.Data, msg.Data)
		require.Equal(t, p.request.Value, msg.Value)
		require.Equal(t, uint64(340_792), msg.Gas)
		require.Equal(t, big.NewInt(22), msg.GasFeeCap)
		require.Equal(t, big.NewInt(2), msg.GasTipCap)
		return []byte{1}, nil
	}
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
}

func TestFalseReturnAndFailedExactCallNeverSign(t *testing.T) {
	for _, booleanReturn := range []bool{false, true} {
		t.Run(map[bool]string{false: "false ABI return", true: "RPC failure"}[booleanReturn], func(t *testing.T) {
			s, b, f := testSender(t)
			p := newPlanner()
			p.request.CheckReturn = func([]byte) bool { return false }
			want := ErrIneligible
			if booleanReturn {
				b.callFn = func(ethereum.CallMsg) ([]byte, error) { return nil, errLostReply }
				want = errLostReply
			}
			_, err := s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, want)
			require.Zero(t, f.signs)
			require.Empty(t, b.published)
		})
	}
}

func TestGasSizingRemainsBoundedWithoutInventingAnEstimate(t *testing.T) {
	for _, test := range []struct {
		name        string
		explicit    uint64
		estimateErr error
		gas         uint64
	}{
		{"explicit", 800_000, nil, 800_000},
		{"estimate unavailable", 0, errLostReply, 0},
		{"padded estimate", 0, nil, 171_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, _ := testSender(t)
			p := newPlanner()
			p.request.GasLimit, b.estimateErr = test.explicit, test.estimateErr
			_, err := s.PublishNext(t.Context(), p)
			if test.estimateErr != nil {
				require.ErrorIs(t, err, test.estimateErr)
				require.Empty(t, b.published)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.gas, publishedTx(t, b, 0).Gas())
		})
	}
}

func TestCurrentContentIsCopiedAndEqualActionCannotChangeIt(t *testing.T) {
	s, _, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	p.request.Data[0] = 99
	p.request.Value.SetInt64(5)
	require.Equal(t, []byte{1, 2, 3}, s.current.request.Data)
	require.Zero(t, s.current.request.Value.Sign())
	_, err = s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, ErrContent)
}

func TestPreconditionsRefuseBeforeSigning(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*Sender[int], *fakeBackend, *fakePlanner)
		want  error
	}{
		{"unfunded", func(_ *Sender[int], b *fakeBackend, _ *fakePlanner) { b.balance.SetUint64(0) }, ErrLimit},
		{"code signer", func(_ *Sender[int], b *fakeBackend, _ *fakePlanner) { b.code = []byte{1} }, ErrUnsupported},
		{"gas cap", func(_ *Sender[int], _ *fakeBackend, p *fakePlanner) { p.request.GasLimit = 1 << 25 }, ErrLimit},
		{"fee cap", func(s *Sender[int], _ *fakeBackend, _ *fakePlanner) { s.policy.MaxFee = big.NewInt(1) }, ErrLimit},
		{"negative value", func(_ *Sender[int], _ *fakeBackend, p *fakePlanner) { p.request.Value.SetInt64(-1) }, ErrLimit},
		{"owner failure", func(s *Sender[int], _ *fakeBackend, _ *fakePlanner) {
			s.guard = func(context.Context) error { return ErrIneligible }
		}, ErrIneligible},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, f := testSender(t)
			p := newPlanner()
			test.setup(s, b, p)
			_, err := s.PublishNext(t.Context(), p)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, f.signs)
			require.Empty(t, b.published)
		})
	}
}

func TestCancelledContextNeverSigns(t *testing.T) {
	s, b, f := testSender(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.PublishNext(ctx, newPlanner())
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, f.signs)
	require.Empty(t, b.published)
}

func TestSignerCannotChangeSignedFields(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*types.DynamicFeeTx)
	}{
		{"nonce", func(tx *types.DynamicFeeTx) { tx.Nonce++ }},
		{"to", func(tx *types.DynamicFeeTx) { other := common.HexToAddress("0x999"); tx.To = &other }},
		{"value", func(tx *types.DynamicFeeTx) { tx.Value = big.NewInt(1) }},
		{"data", func(tx *types.DynamicFeeTx) { tx.Data = []byte{99} }},
		{"gas", func(tx *types.DynamicFeeTx) { tx.Gas++ }},
		{"cap", func(tx *types.DynamicFeeTx) { tx.GasFeeCap = big.NewInt(99) }},
		{"tip", func(tx *types.DynamicFeeTx) { tx.GasTipCap = big.NewInt(3) }},
		{"chain", func(tx *types.DynamicFeeTx) { tx.ChainID = big.NewInt(1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, b, f := testSender(t)
			f.mutate = func(tx *types.Transaction) *types.Transaction {
				dynamic := &types.DynamicFeeTx{ChainID: tx.ChainId(), Nonce: tx.Nonce(), To: tx.To(), Value: tx.Value(),
					Data: tx.Data(), Gas: tx.Gas(), GasFeeCap: tx.GasFeeCap(), GasTipCap: tx.GasTipCap()}
				test.mutate(dynamic)
				return types.NewTx(dynamic)
			}
			_, err := s.PublishNext(t.Context(), newPlanner())
			if test.name == "chain" {
				require.Error(t, err, "the signer itself can reject a changed chain before returning a signature")
			} else {
				require.ErrorIs(t, err, ErrContent)
			}
			require.Empty(t, b.published)
		})
	}
}

func TestDefiniteRefusalRetainsEarlierUsablePreference(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	old := s.current
	p.action = 2
	b.publishErr = testRPCError{replacementUnderpricedMessage, -32000}
	report, err := s.PublishNext(t.Context(), p)
	require.Error(t, err)
	require.Equal(t, ReplacementUnderpriced, report.Class)
	require.Same(t, old, s.current)
	require.Equal(t, 1, report.Current.Action)
	require.Len(t, b.published, 3, "one fresh attempt and one bounded price retry")
}

func TestUncertainChangedRequestBecomesPreferredWithoutDiscardingNetworkCopy(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	first := append([]byte(nil), b.published[0]...)
	p.action = 2
	b.publishErr = errLostReply
	report, err := s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, errLostReply)
	require.Equal(t, 2, report.Current.Action)
	require.Equal(t, first, b.published[0], "the provider can still retain the earlier signed version")
}

func TestNonceLowReplyDoesNotProveConsumption(t *testing.T) {
	s, b, _ := testSender(t)
	b.publishErr = testRPCError{nonceLowMessage, -32000}
	report, err := s.PublishNext(t.Context(), newPlanner())
	require.Error(t, err)
	require.True(t, WaitOnly(err))
	require.Equal(t, uint64(0), report.Nonce)
	require.Nil(t, report.Current)
	require.ErrorIs(t, err, err.(*PublicationError).Err)
	b.publishErr = errors.Join(b.publishErr, errLostReply)
	_, err = s.PublishNext(t.Context(), newPlanner())
	require.False(t, WaitOnly(err))
}

func TestLaggingCanonicalReplicaCannotDiscardCurrentNonce(t *testing.T) {
	s, b, _ := testSender(t)
	b.advance(2, 1)
	_, err := s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	current := s.current
	b.head = b.headers[1]
	report, err := s.PublishNext(t.Context(), newPlanner())
	require.ErrorIs(t, err, ErrInconclusive)
	require.False(t, report.Attempted)
	require.Same(t, current, s.current)
	require.Len(t, b.published, 1)
}

func TestJoinedPublicationTransportFailureRetainsAmbiguousNewPreference(t *testing.T) {
	s, b, _ := testSender(t)
	p := newPlanner()
	_, err := s.PublishNext(t.Context(), p)
	require.NoError(t, err)
	p.action = 2
	b.publishErr = errors.Join(testRPCError{replacementUnderpricedMessage, -32000}, errLostReply)
	report, err := s.PublishNext(t.Context(), p)
	require.ErrorIs(t, err, errLostReply)
	require.Equal(t, Uncertain, report.Class)
	require.Equal(t, 2, report.Current.Action)
	require.Len(t, b.published, 2, "ambiguous evidence does not authorize a price retry")
}
