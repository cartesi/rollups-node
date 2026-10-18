// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/require"
)

func newTournamentInterval(t *testing.T) (*PostgresRepository, *model.Application, []*repository.TournamentEventBatch) {
	t.Helper()
	r, app, batches := newPassiveObserverWindow(t)
	root := batches[0].Tournament
	stmt := table.Epoch.UPDATE(table.Epoch.TournamentAddress).SET(postgres.Bytea(root.Address.Bytes())).
		WHERE(table.Epoch.ApplicationID.EQ(postgres.Int64(app.ID)).AND(table.Epoch.Index.EQ(uint64Expr(root.EpochIndex))))
	query, args := stmt.Sql()
	_, err := r.db.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	return r, app, batches
}

func TestTournamentIntervalNewDescendantsAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "late foreign key failure"}[fail], func(t *testing.T) {
			r, app, batches := newTournamentInterval(t)
			root, child := batches[0].Tournament, batches[1].Tournament
			root.MaxLevel, child.MaxLevel, child.Kind = 3, 3, model.TournamentKindNonLeaf
			grandchild := repotest.NewTournamentBuilder(app.ID).WithLevel(2).
				WithParent(child.Address, batches[1].Matches[0].IDHash).Build()
			grandchild.MaxLevel, grandchild.Kind, grandchild.StartInstant = 3, model.TournamentKindLeaf, 60
			grandchild.CreationEvent.BlockNumber = 60
			if fail {
				grandchild.ParentMatchIDHash = new(repotest.UniqueHash())
			}
			batches = append(batches, &repository.TournamentEventBatch{Tournament: grandchild})
			err := r.StoreTournamentInterval(t.Context(), app.ID, nil, batches)
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			for _, batch := range batches {
				stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
				require.NoError(t, err)
				if fail {
					require.Nil(t, stored)
				} else {
					require.Equal(t, batch.Tournament.Snapshot, stored.Snapshot)
					require.Equal(t, batch.Tournament.CreationEvent, stored.CreationEvent)
				}
			}
			stored, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			if fail {
				require.Zero(t, stored.LastTournamentCheckBlock)
			} else {
				require.Equal(t, observerCheckpoint, stored.LastTournamentCheckBlock)
			}
		})
	}
}

func TestTournamentIntervalCursorCompareAndSet(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(50)), batches), repository.ErrTournamentIntervalStale)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
	require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches), repository.ErrTournamentIntervalStale)
	root := *batches[0].Tournament
	root.Snapshot.AsOfBlock++
	next := []*repository.TournamentEventBatch{{Tournament: &root}}
	require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, next), repository.ErrTournamentIntervalStale,
		"nil cannot replace an existing root even when its endpoint advances")
	require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(99)), next), repository.ErrTournamentIntervalStale)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint), next)
		}()
	}
	close(start)
	one, two := <-results, <-results
	require.True(t, (one == nil) != (two == nil), "one cursor owner must win: %v / %v", one, two)
	if one == nil {
		require.ErrorIs(t, two, repository.ErrTournamentIntervalStale)
	} else {
		require.ErrorIs(t, one, repository.ErrTournamentIntervalStale)
	}
	require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint+1), next),
		repository.ErrTournamentIntervalStale)
}

func TestTournamentIntervalRejectsInvalidGroups(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func([]*repository.TournamentEventBatch) []*repository.TournamentEventBatch
	}{
		{"missing primary", func(_ []*repository.TournamentEventBatch) []*repository.TournamentEventBatch { return nil }},
		{"duplicate", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch { return append(b, b[1]) }},
		{"unrelated root", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch {
			b[1].Tournament = repotest.NewTournamentBuilder(b[0].Tournament.ApplicationID).Build()
			return b
		}},
		{"new primary child", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch { return b[1:] }},
		{"wrong parent", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch {
			b[1].Tournament.ParentTournamentAddress = new(repotest.UniqueAddress())
			return b
		}},
		{"wrong epoch", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch {
			b[1].Tournament.EpochIndex++
			return b
		}},
		{"wrong level", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch {
			b[1].Tournament.Level++
			return b
		}},
		{"creation before parent interval", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch {
			b[1].Tournament.StartInstant = 0
			b[1].Tournament.CreationEvent.BlockNumber = 0
			return b
		}},
		{"creation after endpoint", func(b []*repository.TournamentEventBatch) []*repository.TournamentEventBatch {
			b[1].Tournament.StartInstant = observerCheckpoint + 1
			b[1].Tournament.CreationEvent.BlockNumber = observerCheckpoint + 1
			return b
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, app, batches := newTournamentInterval(t)
			require.Error(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, test.change(batches)))
			stored, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Zero(t, stored.LastTournamentCheckBlock)
			rows, total, err := r.ListTournaments(t.Context(), app.Name, repository.TournamentFilter{}, repository.Pagination{}, false)
			require.NoError(t, err)
			require.Zero(t, total)
			require.Empty(t, rows)
		})
	}
}

func TestTournamentIntervalExistingChildrenAdvanceIndependently(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
	root := *batches[0].Tournament
	root.Snapshot.AsOfBlock = 110
	root.Snapshot.BondRecovery = model.TournamentBondRecovery{Disposition: model.BondDispositionRecovered}
	child := *batches[1].Tournament
	child.Snapshot.AsOfBlock = 110
	require.ErrorIs(t, r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint),
		[]*repository.TournamentEventBatch{{Tournament: &root}, {Tournament: &child}}), repository.ErrTournamentIntervalStale)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint),
		[]*repository.TournamentEventBatch{{Tournament: &root}}))
	storedChild, err := r.GetTournament(t.Context(), app.Name, child.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, observerCheckpoint, storedChild.Snapshot.AsOfBlock)
	// Child remains a current inner winner and can move beyond its retired parent.
	child.Snapshot.AsOfBlock = 120
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint),
		[]*repository.TournamentEventBatch{{Tournament: &child}}))
	root.Snapshot.AsOfBlock = 115
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(110)),
		[]*repository.TournamentEventBatch{{Tournament: &root}}))
	storedApp, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, uint64(120), storedApp.LastTournamentCheckBlock)
}

func TestTournamentIntervalLateRootIgnoresApplicationHighWater(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	stmt := table.Application.UPDATE(table.Application.LastTournamentCheckBlock).SET(uint64Expr(200)).
		WHERE(table.Application.ID.EQ(postgres.Int64(app.ID)))
	query, args := stmt.Sql()
	_, err := r.db.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
	stored, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, uint64(200), stored.LastTournamentCheckBlock)
}

func TestTournamentIntervalRequiresRootSealProvenance(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	batches[0].Tournament.Address = repotest.UniqueAddress()
	batches[0].Commitments, batches[0].Matches = nil, nil
	require.ErrorContains(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches[:1]), "epoch seal")
}

func TestTournamentIntervalRejectsBoundaryViolations(t *testing.T) {
	t.Run("seal after interval", func(t *testing.T) {
		r, app, batch := newRootEventInterval(t)
		stmt := table.Epoch.UPDATE(table.Epoch.LastBlock).SET(uint64Expr(200)).
			WHERE(table.Epoch.ApplicationID.EQ(postgres.Int64(app.ID)))
		query, args := stmt.Sql()
		_, err := r.db.Exec(t.Context(), query, args...)
		require.NoError(t, err)
		require.ErrorContains(t, r.StoreTournamentInterval(t.Context(), app.ID, nil,
			[]*repository.TournamentEventBatch{batch}), "epoch seal")
	})
	t.Run("event before primary cursor", func(t *testing.T) {
		r, app, batches := newTournamentInterval(t)
		require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
		child := *batches[1].Tournament
		child.Snapshot.AsOfBlock = 120
		advance := *batches[1].MatchAdvances[0]
		advance.TxHash, advance.BlockNumber = repotest.UniqueHash(), 90
		err := r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint),
			[]*repository.TournamentEventBatch{{Tournament: &child, MatchAdvances: []*model.MatchAdvanced{&advance}}})
		require.ErrorContains(t, err, "precedes tournament interval")
	})
	t.Run("descendant skips level", func(t *testing.T) {
		r, app, batches := newTournamentInterval(t)
		// Both projections are individually valid; only their parent relation is wrong.
		batches[0].Tournament.MaxLevel = 3
		batches[1].Tournament.MaxLevel, batches[1].Tournament.Level = 3, 2
		err := r.StoreTournamentInterval(t.Context(), app.ID, nil, batches)
		require.ErrorContains(t, err, "does not belong to its parent's interval")
	})
}

func TestTournamentIntervalRefreshPreservesParticipantHistoryBounds(t *testing.T) {
	r, app, batches := newTournamentInterval(t)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, batches))
	refresh := batches[0]
	refresh.Tournament.Snapshot.AsOfBlock = 110
	for _, value := range refresh.Commitments {
		value.Snapshot.AsOfBlock = 110
	}
	for _, value := range refresh.Matches {
		value.Snapshot.AsOfBlock = 110
	}
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(observerCheckpoint),
		[]*repository.TournamentEventBatch{refresh}))
	root := *refresh.Tournament
	root.Snapshot.AsOfBlock = 111
	oldJoin := repotest.NewCommitmentBuilder(app.ID).WithTournamentAddress(root.Address).Build()
	oldJoin.Snapshot.AsOfBlock = 111
	oldMatch := repotest.NewMatchBuilder(app.ID).WithTournamentAddress(root.Address).
		WithCommitmentOne(refresh.Commitments[0].Commitment).WithCommitmentTwo(refresh.Commitments[1].Commitment).Build()
	oldMatch.Snapshot.AsOfBlock = 111
	for _, test := range []struct {
		name  string
		batch *repository.TournamentEventBatch
	}{
		{"new historical commitment", &repository.TournamentEventBatch{Tournament: &root, Commitments: []*model.Commitment{oldJoin}}},
		{"new historical match", &repository.TournamentEventBatch{Tournament: &root, Matches: []*model.Match{oldMatch}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(110)), []*repository.TournamentEventBatch{test.batch})
			require.ErrorContains(t, err, "precedes tournament interval")
			stored, err := r.GetTournament(t.Context(), app.Name, root.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, uint64(110), stored.Snapshot.AsOfBlock)
		})
	}
}
