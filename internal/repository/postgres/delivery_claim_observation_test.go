// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeliveryClaimPagesObserveEnabledApplicationsAtEveryStatus(t *testing.T) {
	for _, status := range model.ApplicationStatusAllValues {
		for _, enabled := range []bool{false, true} {
			name := status.String() + "/disabled"
			if enabled {
				name = status.String() + "/enabled"
			}
			t.Run(name, func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				app := repotest.NewApplicationBuilder().WithEnabled(enabled).Create(t.Context(), t, r)
				epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).Build()
				require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: {}}, 10))
				repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, model.EpochStatus_ClaimComputed)
				if status != model.ApplicationStatus_OK {
					reason := "execution stopped"
					require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, status, &reason))
				}
				later := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
				laterEpoch := repotest.NewEpochBuilder(later.ID).WithStatus(model.EpochStatus_Closed).Build()
				require.NoError(t, r.CreateEpochsAndInputs(t.Context(), later.Name, map[*model.Epoch][]*model.Input{laterEpoch: {}}, 10))
				repotest.AdvanceEpochStatus(t.Context(), t, r, later.Name, laterEpoch, model.EpochStatus_ClaimComputed)
				candidates, apps, err := claimPageMaps(r.SelectClaimsToSubmitPage)(t.Context(), 0, 1)
				require.NoError(t, err)
				require.Len(t, candidates, 1)
				if enabled {
					require.Contains(t, candidates, app.ID)
					require.True(t, apps[app.ID].Enabled)
					require.Equal(t, status, apps[app.ID].Status)
				} else {
					require.NotContains(t, candidates, app.ID)
					require.NotContains(t, apps, app.ID)
					require.Contains(t, candidates, later.ID)
				}
				candidates, _, err = claimPageMaps(r.SelectClaimsToSubmitPage)(t.Context(), app.ID, 1)
				require.NoError(t, err)
				require.Len(t, candidates, 1)
				require.Contains(t, candidates, later.ID)
			})
		}
	}
}
