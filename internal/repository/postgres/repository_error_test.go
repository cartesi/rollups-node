// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestNewPostgresRepository_InvalidConnectionString(t *testing.T) {
	ctx := context.Background()
	_, err := postgres.NewPostgresRepository(
		ctx, "not-a-valid-connection-string", 1, time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse Postgres connection string")
}

func TestNewPostgresRepository_ConnectionSettingsNotExposed(t *testing.T) {
	t.Setenv("PGSERVICE", "")
	const marker = "DATABASE_SETTINGS_SECRET"
	for _, option := range []string{"connect_timeout", "pool_max_conns", "pool_max_conn_lifetime"} {
		t.Run(option, func(t *testing.T) {
			dsn := "postgres://" + marker + ":" + marker + "@127.0.0.1:1/" + marker +
				"?sslmode=disable&" + option + "=" + marker
			// Prove this input reaches the driver conversion that used to echo it.
			_, driverErr := pgxpool.ParseConfig(dsn)
			require.Error(t, driverErr)
			require.Contains(t, driverErr.Error(), option)
			require.Contains(t, driverErr.Error(), marker)

			_, err := postgres.NewPostgresRepository(t.Context(), dsn, 1, 0)
			require.EqualError(t, err, "failed to parse Postgres connection string")
			for _, verb := range []string{"%v", "%+v", "%#v"} {
				require.NotContains(t, fmt.Sprintf(verb, err), marker)
			}
			var raw *pgconn.ParseConfigError
			require.False(t, errors.As(err, &raw), "the raw DSN must not remain in the returned error chain")
		})
	}
}

func TestNewPostgresRepository_ContextAlreadyInterrupted(t *testing.T) {
	t.Setenv("PGSERVICE", "")
	for _, deadline := range []bool{false, true} {
		name, want := "canceled", context.Canceled
		if deadline {
			name, want = "deadline", context.DeadlineExceeded
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			} else {
				cancel()
			}
			defer cancel()
			_, err := postgres.NewPostgresRepository(ctx,
				"postgres://user:pass@127.0.0.1:1/testdb?sslmode=disable", 1, 0)
			require.ErrorIs(t, err, want)
		})
	}
}

func TestNewPostgresRepository_ContextInterruptedOnLastAttempt(t *testing.T) {
	t.Setenv("PGSERVICE", "")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := postgres.NewPostgresRepository(ctx,
			"postgres://user:pass@"+listener.Addr().String()+"/testdb?sslmode=disable", 1, 0)
		result <- err
	}()
	// Accept the connection but do not answer the PostgreSQL startup message.
	// This proves the deadline interrupts the only attempt while it is in flight.
	conn, err := listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.ErrorIs(t, <-result, context.DeadlineExceeded)
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
