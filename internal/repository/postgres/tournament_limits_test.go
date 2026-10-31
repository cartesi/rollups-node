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

func newRootEventInterval(t *testing.T) (*PostgresRepository, *model.Application, *repository.TournamentEventBatch) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	root := repotest.NewTournamentBuilder(app.ID).Build()
	root.StartInstant = 99
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).WithBlocks(0, 99).Build()
	epoch.TournamentAddress = &root.Address
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: {}}, 100))
	return r, app, &repository.TournamentEventBatch{Tournament: root}
}

func addIntervalCommitment(batch *repository.TournamentEventBatch) *model.Commitment {
	value := repotest.NewCommitmentBuilder(batch.Tournament.ApplicationID).
		WithTournamentAddress(batch.Tournament.Address).Build()
	batch.Commitments = append(batch.Commitments, value)
	return value
}

func addIntervalMatch(batch *repository.TournamentEventBatch) *model.Match {
	one, two := addIntervalCommitment(batch), addIntervalCommitment(batch)
	value := repotest.NewMatchBuilder(batch.Tournament.ApplicationID).WithTournamentAddress(batch.Tournament.Address).
		WithCommitmentOne(one.Commitment).WithCommitmentTwo(two.Commitment).Build()
	batch.Matches = append(batch.Matches, value)
	return value
}

func TestTournamentIntervalStatementsAbovePreviousBindLimits(t *testing.T) {
	for _, test := range []struct {
		name  string
		rows  int
		fill  func(*repository.TournamentEventBatch, int)
		table postgres.Table
	}{
		{"commitments", 4682, func(batch *repository.TournamentEventBatch, count int) {
			for range count {
				addIntervalCommitment(batch)
			}
		}, table.Commitments},
		{"matches", 1821, func(batch *repository.TournamentEventBatch, count int) {
			for range count {
				addIntervalMatch(batch)
			}
		}, table.Matches},
		{"advances", 5958, func(batch *repository.TournamentEventBatch, count int) {
			for range count {
				match := addIntervalMatch(batch)
				batch.MatchAdvances = append(batch.MatchAdvances, repotest.NewMatchAdvancedBuilder(match.ApplicationID).
					WithTournamentAddress(match.TournamentAddress).WithIDHash(match.IDHash).Build())
			}
		}, table.MatchAdvances},
		{"bonds", 4682, func(batch *repository.TournamentEventBatch, count int) {
			for range count {
				batch.BondEvents = append(batch.BondEvents, &model.BondEvent{
					ApplicationID: batch.Tournament.ApplicationID, TournamentAddress: batch.Tournament.Address,
					Type: model.BondEventPartialRefund, BlockNumber: 100, TxHash: repotest.UniqueHash(),
					Refund: &model.PartialBondRefund{Recipient: repotest.UniqueAddress(), Success: true},
				})
			}
		}, table.BondEvents},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, app, batch := newRootEventInterval(t)
			test.fill(batch, test.rows)
			require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
			query, args := test.table.SELECT(postgres.COUNT(postgres.STAR)).Sql()
			var count int
			require.NoError(t, r.db.QueryRow(t.Context(), query, args...).Scan(&count))
			require.Equal(t, test.rows, count)
			stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, uint64(100), stored.Snapshot.AsOfBlock)
			// Refreshed participants keep their old creation blocks. Both kinds
			// must pass membership checks before a later projection.
			stored.Snapshot.AsOfBlock = 110
			for _, value := range batch.Commitments {
				value.Snapshot.AsOfBlock = 110
			}
			for _, value := range batch.Matches {
				value.Snapshot.AsOfBlock = 110
			}
			refresh := &repository.TournamentEventBatch{
				Tournament: stored, Commitments: batch.Commitments, Matches: batch.Matches,
			}
			require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)),
				[]*repository.TournamentEventBatch{refresh}))
		})
	}
}

func TestTournamentIntervalLateChunkConflictRollsBack(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	match := addIntervalMatch(batch)
	for index := range tournamentRowsPerStatement + 1 {
		batch.MatchAdvances = append(batch.MatchAdvances, repotest.NewMatchAdvancedBuilder(app.ID).
			WithTournamentAddress(batch.Tournament.Address).WithIDHash(match.IDHash).Build())
		batch.MatchAdvances[index].CurrentHeight = uint64(index) + 1
	}
	duplicate := *batch.MatchAdvances[0]
	batch.MatchAdvances[len(batch.MatchAdvances)-1] = &duplicate
	// An exact retry across chunk boundaries keeps one immutable event row.
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	query, args := table.MatchAdvances.SELECT(postgres.COUNT(postgres.STAR)).Sql()
	var count int
	require.NoError(t, r.db.QueryRow(t.Context(), query, args...).Scan(&count))
	require.Equal(t, tournamentRowsPerStatement, count)

	// Existing rows and the cursor must survive a later contradictory chunk.
	root := *batch.Tournament
	root.Snapshot.AsOfBlock = 110
	next := &repository.TournamentEventBatch{Tournament: &root}
	for index := range tournamentRowsPerStatement {
		advance := repotest.NewMatchAdvancedBuilder(app.ID).WithTournamentAddress(root.Address).WithIDHash(match.IDHash).Build()
		advance.BlockNumber = 105
		advance.CurrentHeight = uint64(index) + uint64(tournamentRowsPerStatement) + 2
		next.MatchAdvances = append(next.MatchAdvances, advance)
	}
	conflict := *next.MatchAdvances[0]
	conflict.LeftNode = repotest.UniqueHash()
	next.MatchAdvances = append(next.MatchAdvances, &conflict)
	err := r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)), []*repository.TournamentEventBatch{next})
	require.ErrorIs(t, err, repository.ErrTournamentEventConflict)
	require.NoError(t, r.db.QueryRow(t.Context(), query, args...).Scan(&count))
	require.Equal(t, tournamentRowsPerStatement, count, "the first new chunk must roll back")
	stored, err := r.GetTournament(t.Context(), app.Name, root.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, uint64(100), stored.Snapshot.AsOfBlock)
	storedApp, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, uint64(100), storedApp.LastTournamentCheckBlock)
}

func TestTournamentIntervalHistoricalParticipantsUseBoundArray(t *testing.T) {
	r, app, batch := newRootEventInterval(t)
	require.NoError(t, r.StoreTournamentInterval(t.Context(), app.ID, nil, []*repository.TournamentEventBatch{batch}))
	batch.Tournament.Snapshot.AsOfBlock = 110
	// These intentionally absent identities must produce the domain verdict,
	// even when an unchunked membership statement would exceed 65,535 binds.
	const participants = 65533
	commitment := repotest.NewCommitmentBuilder(app.ID).WithTournamentAddress(batch.Tournament.Address).Build()
	commitment.Snapshot.AsOfBlock = 110
	for range participants {
		value := *commitment
		value.Commitment = repotest.UniqueHash()
		batch.Commitments = append(batch.Commitments, &value)
	}
	err := r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)), []*repository.TournamentEventBatch{batch})
	require.ErrorContains(t, err, "new commitment precedes tournament interval")
	batch.Commitments = nil
	match := repotest.NewMatchBuilder(app.ID).WithTournamentAddress(batch.Tournament.Address).Build()
	match.Snapshot.AsOfBlock = 110
	for range participants {
		value := *match
		value.IDHash = repotest.UniqueHash()
		batch.Matches = append(batch.Matches, &value)
	}
	err = r.StoreTournamentInterval(t.Context(), app.ID, new(uint64(100)), []*repository.TournamentEventBatch{batch})
	require.ErrorContains(t, err, "new match precedes tournament interval")
}
