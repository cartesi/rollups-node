// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

// Publication continues while observation blocks inside a provider call.
func TestBlockedObservationDoesNotStopPublication(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	queueDelivery(t, backend, lookup, 1)
	chain := s.blockchain.(*claimerBlockchainMock)
	started := make(chan struct{})
	chain.On("getDefaultBlockNumber", mock.Anything).
		Run(func(args mock.Arguments) { close(started); <-args.Get(0).(context.Context).Done() }).
		Return((*big.Int)(nil), context.Canceled).Once()
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { _, err := s.Tick(ctx); finished <- err }()
	<-started
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1)
	cancel()
	select {
	case err := <-finished:
		require.NoError(t, err, "actual service cancellation is a clean observer shutdown")
	case <-time.After(time.Second):
		t.Fatal("observer ignored cancellation")
	}
	chain.AssertExpectations(t)
}
