// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
)

func TestBusPublishSubscribeRoundTrip(t *testing.T) {
	bus := NewBus(64)
	ch := bus.Subscribe(events.ChannelInputReceived)

	n := events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 42,
		EpochIndex:    7,
	}
	bus.Publish(context.Background(), n)

	select {
	case got := <-ch:
		assert.Equal(t, n, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestBusChannelIsolation(t *testing.T) {
	bus := NewBus(64)
	ch := bus.Subscribe(events.ChannelEpochClosed)

	bus.Publish(context.Background(), events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})

	select {
	case <-ch:
		t.Fatal("should not receive notification on unsubscribed channel")
	case <-time.After(50 * time.Millisecond):
		// Expected: no delivery.
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := NewBus(64)
	ch1 := bus.Subscribe(events.ChannelClaimComputed)
	ch2 := bus.Subscribe(events.ChannelClaimComputed)

	n := events.Notification{
		Channel:       events.ChannelClaimComputed,
		ApplicationID: 1,
	}
	bus.Publish(context.Background(), n)

	for _, ch := range []<-chan events.Notification{ch1, ch2} {
		select {
		case got := <-ch:
			assert.Equal(t, n, got)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for notification")
		}
	}
}

func TestBusDropsWhenBufferFull(t *testing.T) {
	bus := NewBus(2)
	ch := bus.Subscribe(events.ChannelInputReceived)

	// Fill the buffer.
	for i := range 2 {
		bus.Publish(context.Background(), events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: int64(i),
		})
	}

	// This should be silently dropped, not block.
	bus.Publish(context.Background(), events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 99,
	})

	// Drain and verify only 2 notifications were delivered.
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	assert.Equal(t, 2, count)
}

func TestBusCloseClosesSubscriberChannels(t *testing.T) {
	bus := NewBus(64)
	ch := bus.Subscribe(events.ChannelInputReceived)

	require.NoError(t, bus.Close())

	_, ok := <-ch
	assert.False(t, ok, "subscriber channel should be closed")
}

func TestBusCloseIsIdempotent(t *testing.T) {
	bus := NewBus(64)
	_ = bus.Subscribe(events.ChannelInputReceived)

	require.NoError(t, bus.Close())
	require.NoError(t, bus.Close())
}

func TestBusPublishAfterClose(t *testing.T) {
	bus := NewBus(64)
	_ = bus.Subscribe(events.ChannelInputReceived)
	require.NoError(t, bus.Close())

	// Should not panic.
	bus.Publish(context.Background(), events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
}

func TestBusListenBlocksUntilContextCanceled(t *testing.T) {
	bus := NewBus(64)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- bus.Listen(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Listen did not return after context cancellation")
	}
}

func TestBusConcurrentAccess(t *testing.T) {
	bus := NewBus(64)
	ch := bus.Subscribe(events.ChannelInputReceived, events.ChannelEpochClosed)

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent publishers.
	for i := range 10 {
		wg.Go(func() {
			for j := range 100 {
				bus.Publish(ctx, events.Notification{
					Channel:       events.ChannelInputReceived,
					ApplicationID: int64(i*100 + j),
				})
			}
		})
	}

	// Concurrent consumer.
	consumed := make(chan int, 1)
	go func() {
		count := 0
		for range ch {
			count++
		}
		consumed <- count
	}()

	wg.Wait()
	require.NoError(t, bus.Close())

	select {
	case n := <-consumed:
		assert.Greater(t, n, 0, "should have consumed at least some notifications")
	case <-time.After(time.Second):
		t.Fatal("consumer did not finish")
	}
}

func TestBusDefaultBufferSize(t *testing.T) {
	bus := NewBus(0)
	assert.Equal(t, defaultBufferSize, bus.bufferSize)
}
