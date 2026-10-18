// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestClaimObservationRefreshPreservesContentAndRejectsStaleCAS(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged} {
		t.Run(string(status), func(t *testing.T) {
			r := newEpochPublicationRepository(t)
			app, epoch := seedForeclosureStatus(t, r, model.Consensus_Quorum, status)
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			var block *uint64
			if status == model.EpochStatus_ClaimStaged {
				block = new(uint64(12))
			}
			hash := repotest.UniqueHash()
			wrongContent := *before
			wrongContent.MachineHash = new(repotest.UniqueHash())
			require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), &wrongContent, new(hash), block), repository.ErrNoUpdate)
			wrongContent = *before
			wrongContent.TxBufferDataBlock = new(repotest.UniqueHash())
			require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), &wrongContent, new(hash), block), repository.ErrNoUpdate)
			wrongContent = *before
			wrongContent.LastBlock++
			require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), &wrongContent, new(hash), block), repository.ErrNoUpdate)
			require.NoError(t, r.RefreshClaimObservation(t.Context(), before, new(hash), block))
			stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			want := *before
			want.ClaimTransactionHash, want.UpdatedAt = new(hash), stored.UpdatedAt
			if block != nil {
				want.StagedAtBlock = block
			}
			require.Equal(t, want, *stored, "a refresh changes audit metadata only")
			require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), before, new(repotest.UniqueHash()), block), repository.ErrNoUpdate)
			require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, epoch.Index, new(hash)))
			require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), stored, new(repotest.UniqueHash()), block), repository.ErrNoUpdate)
			require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
			require.NoError(t, r.DeleteApplication(t.Context(), app.ID))
			require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), stored, new(repotest.UniqueHash()), block), repository.ErrNoUpdate)
			removed, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Nil(t, removed)
		})
	}
}

func TestClaimObservationRefreshRejectsPRTAndHasOneConcurrentWinner(t *testing.T) {
	r := newEpochPublicationRepository(t)
	prtApp, prtEpoch := seedForeclosureStatus(t, r, model.Consensus_PRT, model.EpochStatus_ClaimStaged)
	prtStored, err := r.GetEpoch(t.Context(), prtApp.Name, prtEpoch.Index)
	require.NoError(t, err)
	require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), prtStored,
		new(repotest.UniqueHash()), new(uint64(12))), repository.ErrNoUpdate)
	app, epoch := seedForeclosureStatus(t, r, model.Consensus_Authority, model.EpochStatus_ClaimStaged)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func(hash common.Hash) {
			<-start
			results <- r.RefreshClaimObservation(t.Context(), before, new(hash), new(uint64(12)))
		}(repotest.UniqueHash())
	}
	close(start)
	one, two := <-results, <-results
	require.True(t, (one == nil) != (two == nil), "exactly one prior observation wins: %v / %v", one, two)
	if one == nil {
		require.ErrorIs(t, two, repository.ErrNoUpdate)
	} else {
		require.ErrorIs(t, one, repository.ErrNoUpdate)
	}
}

func TestPRTStagingClockRefreshRejectsAuthorityAndQuorum(t *testing.T) {
	r := newEpochPublicationRepository(t)
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		app, epoch := seedForeclosureStatus(t, r, consensus, model.EpochStatus_ClaimStaged)
		before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.ErrorIs(t, r.UpdateEpochStagingBlock(t.Context(), app.ID, epoch.Index, 4, 12), repository.ErrNoUpdate)
		after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

func TestClaimObservationRefreshComparesHashAndClockSeparately(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app, epoch := seedForeclosureStatus(t, r, model.Consensus_Quorum, model.EpochStatus_ClaimStaged)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NoError(t, r.RefreshClaimObservation(t.Context(), before, new(repotest.UniqueHash()), before.StagedAtBlock))
	before, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NotNil(t, before.ClaimTransactionHash)
	require.NotNil(t, before.StagedAtBlock)
	wrongHash := *before
	wrongHash.ClaimTransactionHash = new(repotest.UniqueHash())
	require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), &wrongHash, before.ClaimTransactionHash,
		new(uint64(12))), repository.ErrNoUpdate, "a non-null old hash participates in CAS")
	wrongClock := *before
	wrongClock.StagedAtBlock = new(*before.StagedAtBlock + 1)
	require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), &wrongClock, before.ClaimTransactionHash,
		new(uint64(12))), repository.ErrNoUpdate, "the clock participates even when the hash is unchanged")
	require.NoError(t, r.RefreshClaimObservation(t.Context(), before, before.ClaimTransactionHash, new(uint64(12))))
	stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, before.ClaimTransactionHash, stored.ClaimTransactionHash)
	require.Equal(t, new(uint64(12)), stored.StagedAtBlock)
}

func TestClaimObservationRefreshCanRecordMissingOwnVoteWithoutLosingStage(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app, epoch := seedForeclosureStatus(t, r, model.Consensus_Quorum, model.EpochStatus_ClaimStaged)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NoError(t, r.RefreshClaimObservation(t.Context(), before, new(repotest.UniqueHash()), before.StagedAtBlock))
	before, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.NotNil(t, before.ClaimTransactionHash)
	require.NoError(t, r.RefreshClaimObservation(t.Context(), before, nil, new(uint64(12))))
	stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimStaged, stored.Status)
	require.Nil(t, stored.ClaimTransactionHash, "a peer's deciding transaction is not evidence of our vote")
	require.Equal(t, new(uint64(12)), stored.StagedAtBlock)
	replacement := repotest.UniqueHash()
	require.NoError(t, r.RefreshClaimObservation(t.Context(), stored, new(replacement), new(uint64(12))))
	stored, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, new(replacement), stored.ClaimTransactionHash)
}

func TestClaimObservationRefreshDoesNotBorrowAnotherApplicationsConsensus(t *testing.T) {
	r := newEpochPublicationRepository(t)
	// A matching Authority row must exist before trying the PRT refresh. Without
	// the application-to-epoch join guard it would authorize the unrelated row.
	seedForeclosureStatus(t, r, model.Consensus_Authority, model.EpochStatus_ClaimStaged)
	app, epoch := seedForeclosureStatus(t, r, model.Consensus_PRT, model.EpochStatus_ClaimStaged)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.ErrorIs(t, r.RefreshClaimObservation(t.Context(), before, new(repotest.UniqueHash()),
		new(uint64(12))), repository.ErrNoUpdate)
	stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, before, stored)
}

func TestClaimObservationRefreshPreservesConcurrentAcceptance(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app, epoch := seedForeclosureStatus(t, r, model.Consensus_Quorum, model.EpochStatus_ClaimStaged)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	acceptedHash := repotest.UniqueHash()
	start := make(chan struct{})
	refreshed, accepted := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		refreshed <- r.RefreshClaimObservation(t.Context(), before, nil, new(uint64(12)))
	}()
	go func() {
		<-start
		accepted <- r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, epoch.Index, new(acceptedHash))
	}()
	close(start)
	require.NoError(t, <-accepted)
	if err := <-refreshed; err != nil {
		require.ErrorIs(t, err, repository.ErrNoUpdate)
	}
	stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimAccepted, stored.Status)
	require.Equal(t, new(acceptedHash), stored.ClaimTransactionHash)
}
