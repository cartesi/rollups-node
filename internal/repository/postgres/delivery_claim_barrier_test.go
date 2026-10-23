// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func TestDeliveryClaimSelectorsAreIndependentOfOtherEpochStates(t *testing.T) {
	for _, outstanding := range []model.EpochStatus{
		model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged,
	} {
		t.Run(outstanding.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_Quorum).Create(t.Context(), t, r)
			seedClaimPageEpochs(t, r, app, outstanding, model.EpochStatus_ClaimAccepted, model.EpochStatus_ClaimComputed)

			candidates, _, err := claimPageMaps(r.SelectClaimsToSubmitPage)(t.Context(), 0, 16)
			require.NoError(t, err)
			require.Equal(t, uint64(2), candidates[app.ID].Index)

			// An unrelated later accepted epoch cannot hide an older observation candidate.
			selectObservation := claimPageMaps(r.SelectClaimsToStagePage)
			if outstanding == model.EpochStatus_ClaimStaged {
				selectObservation = claimPageMaps(r.SelectClaimsToAcceptPage)
			}
			candidates, _, err = selectObservation(t.Context(), 0, 16)
			require.NoError(t, err)
			require.Equal(t, uint64(0), candidates[app.ID].Index)

			require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, 0, nil))
			candidates, _, err = claimPageMaps(r.SelectClaimsToSubmitPage)(t.Context(), 0, 16)
			require.NoError(t, err)
			require.Equal(t, uint64(2), candidates[app.ID].Index)
			observed, _, err := selectObservation(t.Context(), 0, 16)
			require.NoError(t, err)
			require.Empty(t, observed, "acceptance retires only the observation candidate")
		})
	}
}
