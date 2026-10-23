// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"errors"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestClaimPublicationErrorIdentifiesActionAndOriginalCause(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	cause := errors.New("upstream unavailable")
	backend.fail[app.IApplicationAddress] = cause
	err := s.publisher.tick(t.Context())
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), app.IApplicationAddress.Hex())
	require.Contains(t, err.Error(), "action authority-submit")
	require.Contains(t, err.Error(), "epoch 3")
	require.Contains(t, err.Error(), "nonce 0")
	require.EqualValues(t, 3, epoch.Index)
}

func TestClaimHardFailureRetainsItsOriginalCause(t *testing.T) {
	s, _, chain := newServiceMock(t)
	s.submissionEnabled = false
	cause := errors.Join(txsender.ErrFinality, errors.New("finalized block 20 changed hash"))
	chain.On("getDefaultBlockNumber", mock.Anything).Return((*big.Int)(nil), cause).Once()
	_, err := s.Tick(t.Context())
	require.ErrorIs(t, err, txsender.ErrFinality)
	chain.On("getDefaultBlockNumber", mock.Anything).Return((*big.Int)(nil), txsender.ErrContent).Once()
	_, err = s.Tick(t.Context())
	require.NoError(t, err, "the latched observer error is rate limited")
	require.NotNil(t, s.failure.Load())
	require.ErrorIs(t, *s.failure.Load(), txsender.ErrFinality)
	require.Contains(t, (*s.failure.Load()).Error(), "finalized block 20 changed hash")
	require.False(t, s.Ready())
	chain.AssertExpectations(t)
}
