// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestEpochStagingBlockRefreshPreservesClaimAndUsesCAS(t *testing.T) {
	for _, block := range []uint64{3, 9} {
		t.Run(strconv.FormatUint(block, 10), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, epoch := seedForeclosureStatus(t, r, model.Consensus_PRT, model.EpochStatus_ClaimStaged)
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.NoError(t, r.UpdateEpochStagingBlock(t.Context(), app.ID, epoch.Index, 4, block))
			stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			want := *before
			want.StagedAtBlock, want.UpdatedAt = new(block), stored.UpdatedAt
			require.Equal(t, want, *stored, "refresh changes only the staging block and update timestamp")
			require.ErrorIs(t, r.UpdateEpochStagingBlock(t.Context(), app.ID, epoch.Index, 4, 12), repository.ErrNoUpdate)
			require.ErrorIs(t, r.UpdateEpochStagingBlock(t.Context(), app.ID+1, epoch.Index, block, 12), repository.ErrNoUpdate)
			require.ErrorIs(t, r.UpdateEpochStagingBlock(t.Context(), app.ID, epoch.Index+1, block, 12), repository.ErrNoUpdate)
			require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, epoch.Index, nil))
			require.ErrorIs(t, r.UpdateEpochStagingBlock(t.Context(), app.ID, epoch.Index, block, 12), repository.ErrNoUpdate)
			stored, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_ClaimAccepted, stored.Status)
			require.Equal(t, new(block), stored.StagedAtBlock)
		})
	}
}

func TestEpochStagingBlockConcurrentRefreshHasOneWinner(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app, epoch := seedForeclosureStatus(t, r, model.Consensus_PRT, model.EpochStatus_ClaimStaged)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, block := range []uint64{7, 9} {
		go func() {
			<-start
			results <- r.UpdateEpochStagingBlock(t.Context(), app.ID, epoch.Index, 4, block)
		}()
	}
	close(start)
	one, two := <-results, <-results
	require.True(t, (one == nil) != (two == nil), "one stored staging observation must win: %v / %v", one, two)
	if one == nil {
		require.ErrorIs(t, two, repository.ErrNoUpdate)
	} else {
		require.ErrorIs(t, one, repository.ErrNoUpdate)
	}
	stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Contains(t, []uint64{7, 9}, *stored.StagedAtBlock)
}
