// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/require"
)

func TestApplicationDeletionRequiresDisabledAndNoChainRegistration(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	require.ErrorIs(t, r.DeleteApplication(t.Context(), app.ID), repository.ErrApplicationEnabled)
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
	require.NoError(t, r.DeleteApplication(t.Context(), app.ID))
	require.ErrorIs(t, r.DeleteApplication(t.Context(), app.ID), repository.ErrNotFound)
}

func TestApplicationDeletionRechecksConcurrentEnable(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithEnabled(false).Create(t.Context(), t, r)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	require.NoError(t, err)
	defer rollbackDeliveryTx(tx)
	stmt := table.Application.UPDATE(table.Application.Enabled).SET(postgres.Bool(true)).
		WHERE(table.Application.ID.EQ(postgres.Int64(app.ID)))
	query, args := stmt.Sql()
	_, err = tx.Exec(ctx, query, args...)
	require.NoError(t, err)
	var pid int32
	require.NoError(t, tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	result := make(chan error, 1)
	go func() { result <- r.DeleteApplication(ctx, app.ID) }()
	require.Eventually(t, func() bool {
		var blocked bool
		err := r.db.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", pid).Scan(&blocked)
		return err == nil && blocked
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, tx.Commit(ctx))
	require.ErrorIs(t, <-result, repository.ErrApplicationEnabled)
	stored, err := r.GetApplication(ctx, app.Name)
	require.NoError(t, err)
	require.True(t, stored.Enabled, "the committed enable must prevent deletion after its lock wait")
}

func TestApplicationDeletionRollsBackCancelledCascade(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithEnabled(false).Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: {}}, 10))
	blocker, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(blocker)
	stmt := table.Epoch.SELECT(table.Epoch.Index).WHERE(table.Epoch.ApplicationID.EQ(postgres.Int64(app.ID))).FOR(postgres.UPDATE())
	query, args := stmt.Sql()
	_, err = blocker.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	require.Error(t, r.DeleteApplication(ctx, app.ID))
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.NoError(t, blocker.Rollback(t.Context()))
	stored, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.NotNil(t, stored, "failed cascade must roll back the application deletion")
	storedEpoch, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NotNil(t, storedEpoch, "failed cascade must retain child records")
	require.NoError(t, r.DeleteApplication(t.Context(), app.ID), "deletion must recover after the lock is released")
}
