// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const validationWins = "validation"

func TestClaimValidationConcurrentForeclosure(t *testing.T) {
	for _, first := range []string{validationWins, "foreclosure"} {
		t.Run(first+" wins", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch, proof := machinePublicationEpoch(t, r, false)
			require.NoError(t, r.UpdateEpochInputsProcessed(ctx, app.Name, epoch.Index, proof))
			require.NoError(t, r.UpdateApplicationForeclosure(ctx, app.ID, 49, repotest.UniqueHash(), 80))
			endpoint, err := db.GetTestDatabaseEndpoint()
			require.NoError(t, err)
			worker := func() (*PostgresRepository, uint32) {
				cfg, e := pgxpool.ParseConfig(endpoint)
				require.NoError(t, e)
				cfg.MaxConns = 1
				pool, e := pgxpool.NewWithConfig(ctx, cfg)
				require.NoError(t, e)
				t.Cleanup(pool.Close)
				lease, e := pool.Acquire(ctx)
				require.NoError(t, e)
				pid := lease.Conn().PgConn().PID()
				lease.Release()
				return &PostgresRepository{db: pool}, pid
			}
			publisher, publisherPID := worker()
			forecloser, forecloserPID := worker()
			control, err := r.db.Begin(ctx)
			require.NoError(t, err)
			defer rollbackDeliveryTx(control)
			var index uint64
			require.NoError(t, control.QueryRow(ctx, `SELECT index FROM epoch WHERE application_id=$1 AND index=$2 FOR UPDATE`,
				app.ID, epoch.Index).Scan(&index))
			waitBlocked := func(pid uint32) {
				require.Eventually(t, func() bool {
					var blocked bool
					e := control.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, int64(pid)).Scan(&blocked)
					return e == nil && blocked
				}, 3*time.Second, 5*time.Millisecond, "writer %d must be blocked on the real epoch update", pid)
			}
			published := make(chan error, 1)
			foreclosed := make(chan error, 1)
			startPublication := func() {
				go func() { published <- publisher.StoreClaimAndProofs(ctx, epoch, nil) }()
				waitBlocked(publisherPID)
			}
			startForeclosure := func() {
				go func() { foreclosed <- forecloser.UpdateEpochWithForeclosedClaim(ctx, app.ID, epoch.Index) }()
				waitBlocked(forecloserPID)
			}
			if first == validationWins {
				startPublication()
				startForeclosure()
			} else {
				startForeclosure()
				startPublication()
			}
			// Both SQL updates are in flight before the lock is released. PostgreSQL
			// rechecks the losing writer against the row version committed by the winner.
			require.NoError(t, control.Commit(ctx))
			select {
			case e := <-foreclosed:
				require.NoError(t, e)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case e := <-published:
				if first == validationWins {
					require.NoError(t, e)
				} else {
					require.ErrorIs(t, e, repository.ErrEpochForeclosed)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.Equal(t, first == validationWins, claimValidated(t, r, app.ID, epoch.Index))
			roots, err := r.ReadClaimRoots(ctx, app.ID, 59)
			require.NoError(t, err)
			require.Equal(t, first == validationWins, roots.Ready)
			if roots.Ready {
				require.Equal(t, proof.MachineHash, roots.MachineHash)
				require.Equal(t, proof.TxBufferDataBlock, roots.OutputsRoot)
			}
		})
	}
}
