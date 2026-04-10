// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
)

func TestPublishWithNoSubscriber(t *testing.T) {
	connStr := getTestConnString(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	pub := NewPublisher(pool, slog.Default())
	t.Cleanup(func() { _ = pub.Close() })

	// Should not error or panic. The notification is silently delivered
	// to zero listeners.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
}

func TestCoalesceThroughPostgres(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	pub := NewPublisher(pool, slog.Default())
	t.Cleanup(func() { _ = pub.Close() })
	sub := NewSubscriber(connStr, slog.Default(), nil)
	notifCh := sub.Subscribe(events.ChannelInputReceived)
	signal := events.Coalesce(notifCh)

	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Send multiple rapid notifications.
	for i := range 20 {
		pub.Publish(ctx, events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: int64(i),
		})
	}

	// Wait for at least one signal.
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for coalesced signal")
	}

	// Give time for any additional signals to arrive.
	time.Sleep(200 * time.Millisecond)

	// Drain remaining signals. Total should be much less than 20.
	count := 1 // Already received one above.
	for {
		select {
		case <-signal:
			count++
		default:
			goto done
		}
	}
done:
	assert.LessOrEqual(t, count, 5,
		"coalesce should collapse most of 20 rapid notifications")

	cancel()
	require.NoError(t, sub.Close())
}

func TestMultipleChannelsOnOneSubscriber(t *testing.T) {
	connStr := getTestConnString(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	pub := NewPublisher(pool, slog.Default())
	t.Cleanup(func() { _ = pub.Close() })
	sub := NewSubscriber(connStr, slog.Default(), nil)
	ch := sub.Subscribe(events.ChannelInputReceived, events.ChannelEpochClosed)

	go sub.Listen(ctx) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelEpochClosed,
		ApplicationID: 2,
		EpochIndex:    3,
	})

	received := make(map[events.Channel]bool)
	for range 2 {
		select {
		case n := <-ch:
			received[n.Channel] = true
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for notification")
		}
	}

	assert.True(t, received[events.ChannelInputReceived])
	assert.True(t, received[events.ChannelEpochClosed])

	cancel()
	require.NoError(t, sub.Close())
}

func TestNotifyInsideRolledBackTransaction(t *testing.T) {
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

	// NOTIFY inside a transaction that is rolled back should not deliver.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		"SELECT pg_notify($1, $2)",
		"input_received", `{"ch":"input_received","app_id":1}`)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(ctx))

	// No notification should arrive.
	select {
	case n := <-ch:
		t.Fatalf("rolled-back NOTIFY should not deliver, got %+v", n)
	case <-time.After(500 * time.Millisecond):
		// Expected.
	}

	cancel()
	require.NoError(t, sub.Close())
}
