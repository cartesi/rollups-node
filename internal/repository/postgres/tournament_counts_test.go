// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/stretchr/testify/require"
)

func TestTournamentEventCountsUseEventBlocksAndExactScope(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
	root, child := batches[0].Tournament, batches[1].Tournament
	for _, test := range []struct {
		name  string
		child bool
		block uint64
		want  repository.TournamentEventCounts
	}{
		{"before joins", false, 9, repository.TournamentEventCounts{}},
		{"first join", false, 10, repository.TournamentEventCounts{CommitmentJoined: 1}},
		{"root match", false, 11, repository.TournamentEventCounts{CommitmentJoined: 2, MatchCreated: 1}},
		{"before child", false, 49, repository.TournamentEventCounts{CommitmentJoined: 2, MatchCreated: 1}},
		{"child creation", false, 50, repository.TournamentEventCounts{CommitmentJoined: 2, MatchCreated: 1, NewInnerTournament: 1}},
		{"root deletion", false, 80, repository.TournamentEventCounts{
			CommitmentJoined: 2, MatchCreated: 1, MatchDeleted: 1, NewInnerTournament: 1,
		}},
		{"before child joins", true, 50, repository.TournamentEventCounts{}},
		{"child match", true, 52, repository.TournamentEventCounts{CommitmentJoined: 2, MatchCreated: 1}},
		{"two advances", true, 55, repository.TournamentEventCounts{CommitmentJoined: 2, MatchCreated: 1, MatchAdvanced: 2}},
		{"child seal", true, 60, repository.TournamentEventCounts{
			CommitmentJoined: 2, MatchCreated: 1, MatchAdvanced: 2, LeafMatchSealed: 1,
		}},
		{"child deletion", true, 70, repository.TournamentEventCounts{
			CommitmentJoined: 2, MatchCreated: 1, MatchAdvanced: 2, LeafMatchSealed: 1, MatchDeleted: 1,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tournament := root
			if test.child {
				tournament = child
			}
			counts, err := r.GetTournamentEventCounts(t.Context(), app.ID, tournament.EpochIndex, tournament.Address, test.block)
			require.NoError(t, err)
			require.Equal(t, test.want, counts, "all projections were saved at block 100")
		})
	}
	for _, test := range []struct {
		name  string
		appID int64
		epoch uint64
	}{
		{"other application", app.ID + 1, root.EpochIndex},
		{"other epoch", app.ID, root.EpochIndex + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, batch := range batches {
				counts, err := r.GetTournamentEventCounts(t.Context(), test.appID, test.epoch, batch.Tournament.Address, 100)
				require.NoError(t, err)
				require.Equal(t, repository.TournamentEventCounts{}, counts)
			}
		})
	}
	counts, err := r.GetTournamentEventCounts(t.Context(), app.ID, root.EpochIndex, repotest.UniqueAddress(), 100)
	require.NoError(t, err)
	require.Equal(t, repository.TournamentEventCounts{}, counts)
}

func TestTournamentEventCountsExcludeUnsetOptionalEvents(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	addIntervalMatch(batch)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	counts, err := r.GetTournamentEventCounts(t.Context(), app.ID, 0, batch.Tournament.Address, 100)
	require.NoError(t, err)
	require.Equal(t, repository.TournamentEventCounts{CommitmentJoined: 2, MatchCreated: 1}, counts)
}
