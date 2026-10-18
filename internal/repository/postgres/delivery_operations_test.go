// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const (
	operationCancelled = "cancelled"
	operationDeadline  = "deadline"
)

func TestConnectionDiagnosticsClassifyWithoutLeakingValues(t *testing.T) {
	secret := "password=sensitive database=private host=internal"
	for _, test := range []struct {
		name, want string
		cause      error
	}{
		{"authentication", "authentication refused", &pgconn.PgError{Code: "28P01", Message: secret}},
		{"database", "SQLSTATE 3D000", &pgconn.PgError{Code: "3D000", Message: secret}},
		{"invalid code", "connection failed", &pgconn.PgError{Code: secret, Message: secret}},
		{"DNS", "DNS resolution failed", &net.DNSError{Name: secret, Err: secret}},
		{"refused", "connection refused", fmt.Errorf("%s: %w", secret, syscall.ECONNREFUSED)},
		{operationCancelled, operationCancelled, fmt.Errorf("%s: %w", secret, context.Canceled)},
		{operationDeadline, "connection timeout", fmt.Errorf("%s: %w", secret, context.DeadlineExceeded)},
		{"unclassified", "connection failed", errors.New(secret)},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := safePostgresConnectCause(test.cause)
			require.Contains(t, diagnostic, test.want)
			require.NotContains(t, diagnostic, secret)
			require.NotContains(t, diagnostic, "sensitive")
		})
	}
}

func TestDeletionLocalTimeoutsDoNotLeakAfterCommitOnTheSameBackend(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	conn, err := r.db.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	setting := func(key, value string) postgres.Expression {
		return postgres.Func("set_config", postgres.String(key), postgres.String(value), postgres.Bool(false))
	}
	stmt := postgres.SELECT(setting("lock_timeout", "7s"), setting("statement_timeout", "30s"))
	query, args := stmt.Sql()
	_, err = conn.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	require.NoError(t, setDeletionTimeouts(ctx, tx))
	var lockTimeout, statementTimeout string
	querySettings := postgres.SELECT(postgres.Func("current_setting", postgres.String("lock_timeout")),
		postgres.Func("current_setting", postgres.String("statement_timeout")))
	query, args = querySettings.Sql()
	require.NoError(t, tx.QueryRow(ctx, query, args...).Scan(&lockTimeout, &statementTimeout))
	require.Equal(t, "2s", lockTimeout)
	require.NotEqual(t, "30s", statementTimeout)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, conn.QueryRow(ctx, query, args...).Scan(&lockTimeout, &statementTimeout))
	require.Equal(t, "7s", lockTimeout)
	require.Equal(t, "30s", statementTimeout, "the same connection must retain its pre-transaction settings")
}

func TestDeletionTimeoutClampsLongCallerDeadlines(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for _, duration := range []time.Duration{596 * time.Hour, 597 * time.Hour, 1000 * time.Hour} {
		t.Run(duration.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), duration)
			defer cancel()
			tx, err := r.db.Begin(ctx)
			require.NoError(t, err)
			defer rollbackDeliveryTx(tx)
			require.NoError(t, setDeletionTimeouts(ctx, tx))
			stmt := postgres.SELECT(postgres.CAST(postgres.Func("current_setting", postgres.String("statement_timeout"))).AS_TEXT())
			query, args := stmt.Sql()
			var setting string
			require.NoError(t, tx.QueryRow(ctx, query, args...).Scan(&setting))
			if duration.Milliseconds() > math.MaxInt32 {
				require.Equal(t, "2147483647ms", setting)
			} else {
				require.NotEqual(t, "2147483647ms", setting)
			}
		})
	}
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	tx, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	require.ErrorIs(t, setDeletionTimeouts(ctx, tx), context.DeadlineExceeded)
}

type rollbackLogTransaction struct {
	pgx.Tx
	cause      error
	connection *pgx.Conn
}

func (t rollbackLogTransaction) Rollback(context.Context) error { return t.cause }
func (t rollbackLogTransaction) Conn() *pgx.Conn                { return t.connection }

func TestCancelledRollbackIsDebugAndUnexpectedRollbackIsError(t *testing.T) {
	for _, test := range []struct {
		name, level string
		cause       error
	}{
		{operationCancelled, "DEBUG", context.Canceled}, {operationDeadline, "DEBUG", context.DeadlineExceeded},
		{"unexpected", "ERROR", errors.New("unexpected rollback failure")},
		{"already complete", "", pgx.ErrTxClosed},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			rollbackDeliveryTx(rollbackLogTransaction{cause: test.cause})
			if test.level == "" {
				require.Empty(t, output.String())
			} else {
				require.Contains(t, output.String(), "level="+test.level)
			}
		})
	}
}

func TestRollbackAfterConnectionLossIsDebug(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	conn, err := r.db.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	require.NoError(t, conn.Conn().Close(t.Context()))
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	rollbackDeliveryTx(rollbackLogTransaction{cause: net.ErrClosed, connection: conn.Conn()})
	require.Contains(t, output.String(), "level=DEBUG")
	require.NotContains(t, output.String(), "level=ERROR")
}
