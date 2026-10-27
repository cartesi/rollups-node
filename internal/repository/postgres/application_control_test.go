// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestApplicationCreationIncludesDeploymentControl(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum, model.Consensus_PRT} {
		app := repotest.NewApplicationBuilder().WithConsensus(consensus).Create(t.Context(), t, r)
		state, err := r.GetDeploymentAdmissionState(t.Context(), deploymentKey(app))
		require.NoError(t, err)
		require.Equal(t, &model.DeploymentAdmissionState{ApplicationID: app.ID}, state)
	}
}

func TestApplicationCreationRollsBackWhenControlFails(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	_, err := r.db.Exec(t.Context(), `ALTER TABLE deployment_admission_state
		ADD CONSTRAINT control_creation_rejected CHECK (application_id < 0)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := r.db.Exec(context.Background(), `ALTER TABLE deployment_admission_state DROP CONSTRAINT control_creation_rejected`)
		require.NoError(t, err)
	})
	app := repotest.NewApplicationBuilder().Build()
	_, err = r.CreateApplication(t.Context(), app, false)
	var failure *pgconn.PgError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "control_creation_rejected", failure.ConstraintName)
	const applicationTableName = "application"
	for _, table := range []string{applicationTableName, "execution_parameters", "deployment_admission_state"} {
		var count uint64
		require.NoError(t, r.db.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count))
		require.Zero(t, count, "control failure must roll back %s", table)
	}
}
