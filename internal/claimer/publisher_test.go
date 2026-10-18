// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPublisherCancellationStopsItsLoop(t *testing.T) {
	s, _, _ := newDeliveryFixture(t)
	s.publisher.interval = time.Hour
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	finished := make(chan error, 1)
	go func() { finished <- s.publisher.run(ctx) }()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("publisher loop ignored cancellation")
	}
}
