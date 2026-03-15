// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/events/eventstest"
)

func TestSubscriberRoundTrip(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	pub := NewPublisher(pool, slog.Default())
	sub := NewSubscriber(connStr, slog.Default(), nil)
	ch := sub.Subscribe(events.ChannelInputReceived)

	go sub.Listen(ctx) //nolint:errcheck

	// Give the listener time to connect and issue LISTEN.
	time.Sleep(200 * time.Millisecond)

	expected := events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 42,
		EpochIndex:    7,
	}
	pub.Publish(ctx, expected)

	got := eventstest.WaitForNotification(t, ch, 5*time.Second)
	assert.Equal(t, expected, got)

	cancel()
	require.NoError(t, sub.Close())
}

func TestSubscriberChannelIsolation(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	pub := NewPublisher(pool, slog.Default())
	sub := NewSubscriber(connStr, slog.Default(), nil)
	ch := sub.Subscribe(events.ChannelEpochClosed)

	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Publish on a different channel.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})

	select {
	case n := <-ch:
		t.Fatalf("should not receive notification on wrong channel, got %+v", n)
	case <-time.After(500 * time.Millisecond):
		// Expected: no delivery.
	}

	cancel()
	require.NoError(t, sub.Close())
}

func TestSubscriberMalformedPayload(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	sub := NewSubscriber(connStr, slog.Default(), nil)
	ch := sub.Subscribe(events.ChannelInputReceived)

	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Send malformed JSON directly.
	_, err = pool.Exec(ctx,
		"SELECT pg_notify($1, $2)",
		"input_received", "this is not json{{{")
	require.NoError(t, err)

	// Then send a valid notification.
	validPayload, _ := json.Marshal(events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 99,
	})
	_, err = pool.Exec(ctx,
		"SELECT pg_notify($1, $2)",
		"input_received", string(validPayload))
	require.NoError(t, err)

	// The subscriber should skip the malformed one and deliver the valid one.
	got := eventstest.WaitForNotification(t, ch, 5*time.Second)
	assert.Equal(t, int64(99), got.ApplicationID)

	cancel()
	require.NoError(t, sub.Close())
}

func TestSubscriberListenWithoutSubscribe(t *testing.T) {
	sub := NewSubscriber("postgres://localhost/test", slog.Default(), nil)
	err := sub.Listen(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without Subscribe")
}

func TestSubscriberPanicsOnInvalidChannel(t *testing.T) {
	sub := NewSubscriber("postgres://localhost/test", slog.Default(), nil)
	assert.Panics(t, func() {
		sub.Subscribe("bogus_channel")
	})
}

func TestSubscriberBufferOverflow(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	sub := NewSubscriber(connStr, slog.Default(), nil)
	// Use the default buffer size (64).
	ch := sub.Subscribe(events.ChannelInputReceived)

	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Flood with more notifications than the buffer can hold.
	// Don't read from ch so the buffer fills up.
	for i := range 100 {
		payload, _ := json.Marshal(events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: int64(i),
		})
		_, err := pool.Exec(ctx,
			"SELECT pg_notify($1, $2)",
			"input_received", string(payload))
		require.NoError(t, err)
	}

	// Give notifications time to arrive.
	time.Sleep(500 * time.Millisecond)

	// Drain and count. Should be <= buffer size (64).
	drained := eventstest.DrainChannel(ch)
	assert.LessOrEqual(t, len(drained), defaultBufferSize,
		"should not exceed buffer size")
	assert.Greater(t, len(drained), 0, "should have received some notifications")

	cancel()
	require.NoError(t, sub.Close())
}

func TestSubscriberReconnectsAfterConnectionKill(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	pub := NewPublisher(pool, slog.Default())
	sub := NewSubscriber(connStr, slog.Default(), nil)
	ch := sub.Subscribe(events.ChannelInputReceived)

	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Verify initial connectivity.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
	got := eventstest.WaitForNotification(t, ch, 5*time.Second)
	assert.Equal(t, int64(1), got.ApplicationID)

	// Kill all event listener connections.
	_, err = pool.Exec(ctx, fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity "+
			"WHERE application_name = 'rollups-node-events' AND pid <> pg_backend_pid()"))
	require.NoError(t, err)

	// Wait for reconnection (backoff starts at 500ms).
	time.Sleep(2 * time.Second)

	// Verify the subscriber recovered and can receive again.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 2,
	})
	got = eventstest.WaitForNotification(t, ch, 5*time.Second)
	assert.Equal(t, int64(2), got.ApplicationID)

	cancel()
	require.NoError(t, sub.Close())
}

func TestSubscriberCloseIsIdempotent(t *testing.T) {
	sub := NewSubscriber("postgres://localhost/test", slog.Default(), nil)
	_ = sub.Subscribe(events.ChannelInputReceived)
	require.NoError(t, sub.Close())
	require.NoError(t, sub.Close())
}

func TestSubscriberContextCancelDuringListen(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithCancel(context.Background())

	sub := NewSubscriber(connStr, slog.Default(), nil)
	_ = sub.Subscribe(events.ChannelInputReceived)

	done := make(chan error, 1)
	go func() {
		done <- sub.Listen(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not return after context cancellation")
	}
}
