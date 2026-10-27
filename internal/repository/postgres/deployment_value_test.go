// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDeploymentAssessmentUsesTierConstraint(t *testing.T) {
	r, _, key := deploymentFixture(t)
	_, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{
		Identity: key, ConfirmedTier: new(model.DeploymentTier("unknown")), Reason: "confirmed mismatch",
	})
	var failure *pgconn.PgError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "23514", failure.Code)
	require.Equal(t, "deployment_admission_state_confirmed_tier_check", failure.ConstraintName)
	require.Equal(t, &model.DeploymentAdmissionState{ApplicationID: key.ApplicationID}, readDeployment(t, r, key))
}
