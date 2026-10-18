// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDeliveryProvenanceAndRegistrationDoNotWaitForApplicationLock(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	signer := repotest.UniqueAddress()
	registerDeliveryTest(t, r, signer)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).
		WithEnabled(false).Create(t.Context(), t, r)
	gate, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(gate)
	stmt := table.Application.SELECT(table.Application.ID).WHERE(table.Application.ID.EQ(postgres.Int64(app.ID))).FOR(postgres.UPDATE())
	query, args := stmt.Sql()
	_, err = gate.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	require.NoError(t, r.CheckSubmitterProvenance(ctx, deliveryPRTKey))
	_, err = r.RegisterSubmitter(ctx, deliveryPRTKey, config.PersistentSubmitterConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
		ClaimSubmissionEnabled: true, ResolvedSignerAddress: signer,
	})
	require.NoError(t, err, "startup registration must no longer share application-deletion locks")
}

func TestDeliveryTerminalClaimsRequireExistingProvenance(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimRejected, model.EpochStatus_ClaimForeclosed} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			_, err := r.RegisterObserver(t.Context(), deliveryObserverKey, config.PersistentChainConfig{
				DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
			})
			require.NoError(t, err)
			app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
			epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).Build()
			require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: {}}, 10))
			repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, status)
			_, err = r.RegisterSubmitter(t.Context(), deliveryClaimerKey, config.PersistentSubmitterConfig{
				DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
				ClaimSubmissionEnabled: true, ResolvedSignerAddress: repotest.UniqueAddress(),
			})
			require.ErrorContains(t, err, "provenance is absent")
		})
	}
}

func TestDeliveryDeletionBoundsLockWaitWithoutLeakingSetting(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithEnabled(false).Create(t.Context(), t, r)
	var defaultTimeout string
	require.NoError(t, r.db.QueryRow(t.Context(), "SHOW lock_timeout").Scan(&defaultTimeout))
	locker, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(locker)
	stmt := table.Application.SELECT(table.Application.ID).WHERE(table.Application.ID.EQ(postgres.Int64(app.ID))).
		FOR(postgres.UPDATE())
	query, args := stmt.Sql()
	_, err = locker.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err = r.DeleteApplication(ctx, app.ID)
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, "55P03", pgError.Code)
	require.NoError(t, ctx.Err(), "server lock timeout must precede the caller deadline")
	var afterTimeout string
	require.NoError(t, r.db.QueryRow(t.Context(), "SHOW lock_timeout").Scan(&afterTimeout))
	require.Equal(t, defaultTimeout, afterTimeout, "SET LOCAL must not alter subsequent pool users")
	require.NoError(t, locker.Rollback(t.Context()))
	require.NoError(t, r.DeleteApplication(t.Context(), app.ID))
}
