// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func seedOutputExecutionLocks(t *testing.T) (*PostgresRepository, *model.Application, []*model.Output, *pgx.Conn) {
	t.Helper()
	repo := newEpochPublicationRepository(t).(*PostgresRepository)
	seed := repotest.Seed(t.Context(), t, repo)
	require.NoError(t, repo.StoreAdvanceResult(t.Context(), seed.App.ID, &model.AdvanceResult{
		EpochIndex: seed.Epoch.Index, InputIndex: seed.Input.Index,
		Status: model.InputCompletionStatus_Accepted, StateProof: *repotest.DummyStateProof(),
		Outputs: [][]byte{[]byte("zero"), []byte("one"), []byte("two")},
	}))
	require.NoError(t, repo.UpdateEventLastCheckBlock(t.Context(), []int64{seed.App.ID}, model.MonitoredEvent_OutputExecuted, 100))
	outputs := make([]*model.Output, 3)
	for i := range outputs {
		output, err := repo.GetOutput(t.Context(), seed.App.Name, uint64(i))
		require.NoError(t, err)
		require.NotNil(t, output)
		output.ExecutionTransactionHash = new(repotest.UniqueHash())
		outputs[i] = output
	}
	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	control, err := pgx.Connect(t.Context(), endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { _ = control.Close(context.Background()) })
	return repo, seed.App, outputs, control
}

func TestOutputsExecutionLocksInIndexOrder(t *testing.T) {
	repo, app, outputs, control := seedOutputExecutionLocks(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	locker, err := control.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = locker.Rollback(context.Background()) })
	_, err = locker.Exec(ctx, `SELECT index FROM output WHERE input_epoch_application_id=$1 AND index=0 FOR UPDATE`, app.ID)
	require.NoError(t, err)
	eventOrder := []*model.Output{outputs[2], outputs[0]}
	done := make(chan error, 1)
	go func() { done <- repo.UpdateOutputsExecution(ctx, app.Name, eventOrder, 200) }()
	require.Eventually(t, func() bool {
		var waiting bool
		err := locker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))`, control.PgConn().PID()).Scan(&waiting)
		return err == nil && waiting
	}, time.Second, 10*time.Millisecond)
	_, err = locker.Exec(ctx, `SELECT index FROM output
		WHERE input_epoch_application_id=$1 AND index=2 FOR UPDATE NOWAIT`, app.ID)
	require.NoError(t, err, "the execution writer must wait on output zero before locking output two")
	require.NoError(t, locker.Rollback(ctx))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Equal(t, []*model.Output{outputs[2], outputs[0]}, eventOrder, "preserve the caller's event order")
	for _, expected := range eventOrder {
		stored, err := repo.GetOutput(ctx, app.Name, expected.Index)
		require.NoError(t, err)
		require.Equal(t, expected.ExecutionTransactionHash, stored.ExecutionTransactionHash)
	}
	cursor, err := repo.GetEventLastCheckBlock(ctx, app.ID, model.MonitoredEvent_OutputExecuted)
	require.NoError(t, err)
	require.Equal(t, uint64(200), cursor)
}

func TestOutputsExecutionBoundsLockWait(t *testing.T) {
	for _, applicationLock := range []bool{false, true} {
		target := "output row"
		if applicationLock {
			target = "application row"
		}
		t.Run(target, func(t *testing.T) {
			repo, app, outputs, control := seedOutputExecutionLocks(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			var before string
			require.NoError(t, repo.db.QueryRow(ctx, "SHOW lock_timeout").Scan(&before))
			locker, err := control.Begin(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { _ = locker.Rollback(context.Background()) })
			query := `SELECT index FROM output WHERE input_epoch_application_id=$1 AND index=0 FOR UPDATE`
			if applicationLock {
				query = `SELECT id FROM application WHERE id=$1 FOR UPDATE`
			}
			_, err = locker.Exec(ctx, query, app.ID)
			require.NoError(t, err)
			err = repo.UpdateOutputsExecution(ctx, app.Name, outputs[:1], 200)
			require.ErrorIs(t, err, repository.ErrLockTimeout)
			var pgError *pgconn.PgError
			require.True(t, errors.As(err, &pgError))
			require.Equal(t, "55P03", pgError.Code)
			require.NoError(t, ctx.Err(), "the local lock bound must expire before the request deadline")
			stored, err := repo.GetOutput(ctx, app.Name, 0)
			require.NoError(t, err)
			require.Nil(t, stored.ExecutionTransactionHash)
			cursor, err := repo.GetEventLastCheckBlock(ctx, app.ID, model.MonitoredEvent_OutputExecuted)
			require.NoError(t, err)
			require.Equal(t, uint64(100), cursor)
			var after string
			require.NoError(t, repo.db.QueryRow(ctx, "SHOW lock_timeout").Scan(&after))
			require.Equal(t, before, after, "transaction-local timeout must not leak into the pool")
			require.NoError(t, locker.Rollback(ctx))
			require.NoError(t, repo.UpdateOutputsExecution(ctx, app.Name, outputs[:1], 200))
			stored, err = repo.GetOutput(ctx, app.Name, 0)
			require.NoError(t, err)
			require.Equal(t, outputs[0].ExecutionTransactionHash, stored.ExecutionTransactionHash)
			cursor, err = repo.GetEventLastCheckBlock(ctx, app.ID, model.MonitoredEvent_OutputExecuted)
			require.NoError(t, err)
			require.Equal(t, uint64(200), cursor)
		})
	}
}
