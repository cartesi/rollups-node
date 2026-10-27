// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDeploymentSchemaRejectsInvalidControlValues(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	_, err := r.db.Exec(t.Context(), `INSERT INTO deployment_admission_state(application_id) VALUES ($1)
		ON CONFLICT (application_id) DO NOTHING`, app.ID)
	require.NoError(t, err)
	for _, test := range []struct {
		name, constraint string
		generation       int64
		tier, reason     any
	}{
		{name: "negative generation", generation: -1, constraint: "deployment_admission_state_reset_generation_check"},
		{name: "unknown tier", tier: "unknown", reason: "mismatch", constraint: "deployment_admission_state_confirmed_tier_check"},
		{name: "tier without reason", tier: "table", constraint: "deployment_admission_state_restriction_pair_check"},
		{name: "reason without tier", reason: "mismatch", constraint: "deployment_admission_state_restriction_pair_check"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := r.db.Exec(t.Context(), `UPDATE deployment_admission_state
				SET reset_generation=$2,confirmed_tier=$3,deployment_reason=$4 WHERE application_id=$1`,
				app.ID, test.generation, test.tier, test.reason)
			var failure *pgconn.PgError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, "23514", failure.Code)
			require.Equal(t, test.constraint, failure.ConstraintName)
		})
	}
}

func TestDeploymentSchemaApplicationRemovalCascades(t *testing.T) {
	repo := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, repo)
	_, err := repo.db.Exec(t.Context(), `INSERT INTO deployment_admission_state
		(application_id, confirmed_tier, deployment_reason) VALUES ($1, 'table', 'table mismatch')`, app.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateApplicationEnabled(t.Context(), app.ID, false))
	require.NoError(t, repo.DeleteApplication(t.Context(), app.ID))
	var count uint64
	require.NoError(t, repo.db.QueryRow(t.Context(), `SELECT count(*) FROM deployment_admission_state
		WHERE application_id = $1`, app.ID).Scan(&count))
	require.Zero(t, count)
}
