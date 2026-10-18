// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type deletionGateTracer struct {
	arrived chan struct{}
	resume  chan struct{}
}

func (g *deletionGateTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if strings.Contains(query.SQL, "DELETE FROM public.application") {
		g.arrived <- struct{}{}
		select {
		case <-g.resume:
		case <-ctx.Done():
		}
	}
	return ctx
}

func (*deletionGateTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestDeliveryOverlappingDeletionsSerializeBeforeDelete(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	registerDeliveryTest(t, r, repotest.UniqueAddress())
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).WithEnabled(false).Create(t.Context(), t, r)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	gate := &deletionGateTracer{arrived: make(chan struct{}, 1), resume: make(chan struct{})}
	newRepository := func(tracer pgx.QueryTracer) *PostgresRepository {
		t.Helper()
		cfg := r.db.Config()
		cfg.MaxConns, cfg.ConnConfig.Tracer = 1, tracer
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		require.NoError(t, err)
		t.Cleanup(pool.Close)
		return &PostgresRepository{db: pool}
	}
	first, second := newRepository(gate), newRepository(nil)
	var secondPID int32
	require.NoError(t, second.db.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&secondPID))
	firstResult, secondResult := make(chan error, 1), make(chan error, 1)
	go func() { firstResult <- first.DeleteApplication(ctx, app.ID) }()
	select {
	case <-gate.arrived:
	case <-ctx.Done():
		t.Fatal("first deletion did not reach DELETE with its application lock held")
	}
	go func() { secondResult <- second.DeleteApplication(ctx, app.ID) }()
	var blockedQuery string
	require.Eventually(t, func() bool {
		err := r.db.QueryRow(ctx, "SELECT query FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock'", secondPID).
			Scan(&blockedQuery)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	// pg_stat_activity can truncate the long projection before its FOR clause.
	// The application SELECT still distinguishes waiting at the gate from DELETE.
	require.Contains(t, blockedQuery, "SELECT application.enabled", "the second gate must wait before it can reach DELETE")
	close(gate.resume)
	require.NoError(t, <-firstResult)
	require.ErrorIs(t, <-secondResult, repository.ErrNotFound)
}
