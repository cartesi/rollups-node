// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestDatabaseOperationBoundsAndCancellation(t *testing.T) {
	start := time.Now()
	ctx, cancel := beginDatabaseOperation(context.Background())
	deadline, bounded := ctx.Deadline()
	require.True(t, bounded)
	require.WithinDuration(t, start.Add(databaseOperationTimeout), deadline, time.Second)
	cancel()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	parent, done := context.WithTimeout(t.Context(), time.Second)
	defer done()
	ctx, cancel = beginDatabaseOperation(parent)
	defer cancel()
	parentDeadline, _ := parent.Deadline()
	deadline, _ = ctx.Deadline()
	require.Equal(t, parentDeadline, deadline, "operation bounds must preserve shorter caller deadlines")
	done()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

type operationDeadlineTracer struct {
	mu        sync.Mutex
	deadlines []time.Time
}

func (t *operationDeadlineTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	deadline, _ := ctx.Deadline()
	t.mu.Lock()
	t.deadlines = append(t.deadlines, deadline)
	t.mu.Unlock()
	return ctx
}

func (*operationDeadlineTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestDatabaseOperationBoundsPoolAcquisitionAndActualQuery(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	tracer := &operationDeadlineTracer{}
	cfg := r.db.Config()
	cfg.MaxConns, cfg.ConnConfig.Tracer = 1, tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	bounded := &PostgresRepository{db: pool}
	stored, err := bounded.GetApplication(context.Background(), app.Name)
	require.NoError(t, err)
	require.Equal(t, app.ID, stored.ID)
	tracer.mu.Lock()
	deadlines := append([]time.Time(nil), tracer.deadlines...)
	tracer.mu.Unlock()
	require.NotEmpty(t, deadlines)
	for _, deadline := range deadlines {
		require.False(t, deadline.IsZero(), "the actual driver query must carry the operation deadline")
		require.LessOrEqual(t, time.Until(deadline), databaseOperationTimeout)
	}
	conn, err := pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err = bounded.GetApplication(ctx, app.Name)
	require.ErrorIs(t, err, context.DeadlineExceeded, "pool exhaustion must obey the caller deadline")
	conn.Release()
	stored, err = bounded.GetApplication(t.Context(), app.Name)
	require.NoError(t, err, "a timed-out acquisition must not prevent later progress")
	require.Equal(t, app.ID, stored.ID)
}
