// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/stretchr/testify/require"
)

// Separate nodes cannot coordinate through their databases. Exercise two real
// senders using the same account and pool, without treating either nonce use as
// proof that the other node's protocol action completed.
func TestAccidentalSharedWalletBoundsConflictsAndReconstructsOutstandingWork(t *testing.T) {
	first, backend, signer := testSender(t)
	backend.publishFn = poolRule(t)
	second, err := New[int](backend, signer, 31337, Policy{},
		func(ctx context.Context) error { return ctx.Err() }, testLogger())
	require.NoError(t, err)
	firstPlan, secondPlan := newPlanner(), newPlanner()
	secondPlan.action = 2
	secondPlan.request.Data = []byte{4, 5, 6}

	initial, err := first.PublishNext(t.Context(), firstPlan)
	require.NoError(t, err)
	require.Equal(t, Accepted, initial.Class)
	for range 3 {
		before := len(backend.published)
		conflict, conflictErr := second.PublishNext(t.Context(), secondPlan)
		var publicationErr *PublicationError
		require.ErrorAs(t, conflictErr, &publicationErr)
		require.False(t, HardFailure(conflictErr), "price contention cannot latch a terminal failure")
		require.False(t, PassInvalid(conflictErr))
		require.Equal(t, ReplacementUnderpriced, conflict.Class)
		require.LessOrEqual(t, len(backend.published)-before, 2, "one pass cannot start an unbounded price auction")
		require.Nil(t, conflict.Current, "definite refusals do not establish an admitted transaction")
	}

	// The first node's transaction consumes the nonce, without completing the
	// second node's action. Reconstruct that same outstanding action at nonce 1.
	backend.advance(2, 1)
	backend.publishFn = poolRule(t)
	recovered, err := second.PublishNext(t.Context(), secondPlan)
	require.NoError(t, err)
	require.Equal(t, Accepted, recovered.Class)
	require.Equal(t, uint64(1), recovered.Nonce)
	require.Equal(t, secondPlan.action, recovered.Current.Action)
	require.Equal(t, secondPlan.request.Data, publishedTx(t, backend, len(backend.published)-1).Data())
	require.Nil(t, secondPlan.seen[len(secondPlan.seen)-1])
}

func TestInvalidSignerFactoryIsUnsupportedWithoutPublishing(t *testing.T) {
	sender, backend, signer := testSender(t)
	sender.factory = missingSignerFactory{signer}
	report, err := sender.PublishNext(t.Context(), newPlanner())
	require.ErrorIs(t, err, ErrUnsupported)
	require.True(t, PassInvalid(err))
	require.False(t, report.Attempted)
	require.Empty(t, backend.published)
}

type missingSignerFactory struct{ *fakeSigner }

func (f missingSignerFactory) NewTransactOpts(context.Context) (*bind.TransactOpts, error) {
	return &bind.TransactOpts{From: f.From()}, nil
}
