// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/events"
)

func getTestConnString(t *testing.T) string {
	t.Helper()
	connStr := os.Getenv("CARTESI_TEST_DATABASE_CONNECTION")
	if connStr == "" {
		t.Skip("CARTESI_TEST_DATABASE_CONNECTION not set")
	}
	return connStr
}

func TestPublisherIntegration(t *testing.T) {
	connStr := getTestConnString(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	defer pool.Close()

	// Set up a raw LISTEN connection to verify the notification arrives.
	listenConn, err := pgx.Connect(ctx, connStr)
	require.NoError(t, err)
	defer listenConn.Close(ctx)

	_, err = listenConn.Exec(ctx, "LISTEN input_received")
	require.NoError(t, err)

	pub := NewPublisher(pool, slog.Default())

	notification := events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 42,
		EpochIndex:    7,
	}
	pub.Publish(ctx, notification)

	// Wait for the notification to arrive.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pgNotif, err := listenConn.WaitForNotification(waitCtx)
	require.NoError(t, err)

	assert.Equal(t, "input_received", pgNotif.Channel)

	var received events.Notification
	require.NoError(t, json.Unmarshal([]byte(pgNotif.Payload), &received))
	assert.Equal(t, notification, received)
}

func TestPublisherDoesNotPanicOnClosedPool(t *testing.T) {
	connStr := getTestConnString(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	pool.Close()

	pub := NewPublisher(pool, slog.Default())

	// Should log a warning, not panic.
	pub.Publish(ctx, events.Notification{
		Channel:       events.ChannelInputReceived,
		ApplicationID: 1,
	})
}
