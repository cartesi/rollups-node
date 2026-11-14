// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestIORPublicationPagesFilterBeforeApplicationAndEpochLimits(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted,
		model.EpochStatus_ClaimStaged} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			blocked, _ := seedProblemEpochs(t, r)
			blockedEpochs := []*model.Epoch{storeTestIORClaim(t, r, blocked, 0), storeTestIORClaim(t, r, blocked, 1)}
			app, epochs := seedProblemEpochs(t, r)
			first := storeTestIORClaim(t, r, app, 0)
			require.NoError(t, r.StoreClaimAndProofs(t.Context(), epochs[1], nil))
			query := r.SelectClaimsToSubmitPage
			// Submitted IOR is a defensive consumer fixture; fresh local submission is vetoed.
			for _, item := range []struct {
				app    *model.Application
				epochs []*model.Epoch
			}{
				{blocked, blockedEpochs}, {app, []*model.Epoch{first, epochs[1]}},
			} {
				for _, epoch := range item.epochs {
					if status == model.EpochStatus_ClaimSubmitted || status == model.EpochStatus_ClaimStaged {
						require.NoError(t, r.UpdateEpochWithSubmittedClaim(t.Context(), item.app.ID, epoch.Index, *epoch.TxBufferDataBlock))
					}
					if status == model.EpochStatus_ClaimStaged {
						require.NoError(t, r.UpdateEpochToStaged(t.Context(), item.app.ID, epoch.Index, 100))
					}
				}
			}
			if status == model.EpochStatus_ClaimSubmitted {
				query = r.SelectClaimsToStagePage
			}
			if status == model.EpochStatus_ClaimStaged {
				query = r.SelectClaimsToAcceptPage
			}
			page, err := query(t.Context(), repository.ClaimCursor{PublishableOnly: true}, 2)
			require.NoError(t, err)
			require.Equal(t, app.ID, page.ApplicationID, "an application with only IOR rows must consume no publisher turn")
			require.Len(t, page.Work, 1)
			require.Equal(t, uint64(1), page.Work[0].Epoch.Index, "the older IOR must not consume the epoch window")
			require.True(t, page.Work[0].Epoch.HasMatchingOutputsRoot())
			observed, err := query(t.Context(), repository.ClaimCursor{AfterApplicationID: blocked.ID}, 2)
			require.NoError(t, err)
			require.Equal(t, app.ID, observed.ApplicationID)
			require.Len(t, observed.Work, 2)
			require.True(t, observed.Work[0].Epoch.HasInvalidOutputsRoot(), "observation keeps the IOR source")
			require.True(t, observed.Work[1].Epoch.HasMatchingOutputsRoot())
		})
	}
}

func TestInvalidOutputsRootFilterRequiresPresentUnequalRoots(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epochs := seedProblemEpochs(t, r)
	first := storeTestIORClaim(t, r, app, 0)
	check := func() {
		got, total, err := r.ListEpochs(t.Context(), app.Name, repository.EpochFilter{InvalidOutputsRoot: true},
			repository.Pagination{}, false)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, got, 1)
		require.Equal(t, uint64(0), got[0].Index)
		require.Equal(t, first.CalculatedOutputsRoot, got[0].CalculatedOutputsRoot)
		require.True(t, got[0].HasInvalidOutputsRoot())
	}
	check() // The next INPUTS_PROCESSED row has a declared root but no calculated root.
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epochs[1], nil))
	check() // Matching roots are not IOR.
	require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, 0, nil))
	check() // Acceptance does not erase the durable mismatch.
	all, _, err := r.ListEpochs(context.Background(), app.Name, repository.EpochFilter{}, repository.Pagination{}, false)
	require.NoError(t, err)
	require.Len(t, all, 3, "default discovery does not apply the mismatch filter")
}
