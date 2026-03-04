// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/stretchr/testify/require"
)

func TestNewPostgresRepository_InvalidConnectionString(t *testing.T) {
	ctx := context.Background()
	_, err := postgres.NewPostgresRepository(
		ctx, "not-a-valid-connection-string", 1, time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse Postgres connection string")
}

func TestNewPostgresRepository_UnreachableHostRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	// Port 1 on localhost is almost certainly not running PostgreSQL.
	// With maxRetries=2 and minimal delay the retries exhaust quickly.
	_, err := postgres.NewPostgresRepository(
		ctx, "postgres://user:pass@localhost:1/testdb?connect_timeout=1", 2, time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to ping Postgres after 2 retries")
}

func TestNewPostgresRepository_ContextCancelledDuringRetry(t *testing.T) {
	// Use a short-lived context so that it expires while the function is
	// waiting between retry attempts (delay is deliberately long).
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := postgres.NewPostgresRepository(
		ctx,
		"postgres://user:pass@localhost:1/testdb?connect_timeout=1",
		100,            // many retries — we won't exhaust them
		10*time.Second, // long delay — context expires before this elapses
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
