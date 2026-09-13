// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestTournamentEventWindowLocksApplicationBeforeChildren(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skip(err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	newRepository := func() *PostgresRepository {
		cfg, err := pgxpool.ParseConfig(endpoint)
		require.NoError(t, err)
		cfg.MaxConns = 1 // Each operation has a stable PostgreSQL backend PID.
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		require.NoError(t, err)
		t.Cleanup(pool.Close)
		return &PostgresRepository{db: pool}
	}
	writer, deleter := newRepository(), newRepository()
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).WithEnabled(false).Create(ctx, t, writer)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(EpochStatus_Closed).WithInputBounds(0, 0).Build()
	require.NoError(t, writer.CreateEpochsAndInputs(ctx, app.Name, map[*Epoch][]*Input{epoch: {}}, 10))
	var writerPID, deleterPID int32
	require.NoError(t, writer.db.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&writerPID))
	require.NoError(t, deleter.db.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&deleterPID))

	gate, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, gate.Close(context.Background())) })
	gateTx, err := gate.Begin(ctx)
	require.NoError(t, err)
	defer gateTx.Rollback(context.Background()) //nolint:errcheck
	_, err = gateTx.Exec(ctx, `SELECT 1 FROM epoch WHERE application_id = $1 FOR UPDATE`, app.ID)
	require.NoError(t, err)
	gatePID := int32(gate.PgConn().PID()) //nolint:gosec // PostgreSQL backend PIDs fit in int32.
	waitForBlocker := func(blocked, blocker int32) {
		t.Helper()
		require.Eventually(t, func() bool {
			var blockedByExpected bool
			err := gate.QueryRow(ctx, "SELECT $1 = ANY(pg_blocking_pids($2))", blocker, blocked).Scan(&blockedByExpected)
			return err == nil && blockedByExpected
		}, 5*time.Second, 10*time.Millisecond, "backend %d must wait for backend %d", blocked, blocker)
	}

	stored := make(chan error, 1)
	go func() {
		batch := &repository.TournamentEventBatch{Tournament: repotest.NewTournamentBuilder(app.ID).Build()}
		batch.Tournament.Snapshot.AsOfBlock = 100
		stored <- writer.StoreTournamentEvents(ctx, app.ID, []*repository.TournamentEventBatch{batch}, 100)
	}()
	// The epoch lock pauses the first projection's foreign-key check. The
	// writer must already hold the application lock before it reaches here.
	waitForBlocker(writerPID, gatePID)
	deleted := make(chan error, 1)
	go func() { deleted <- deleter.DeleteApplication(ctx, app.ID) }()
	waitForBlocker(deleterPID, writerPID)

	require.NoError(t, gateTx.Commit(ctx))
	require.NoError(t, <-stored, "the event window must commit before deletion can take its application lock")
	require.NoError(t, <-deleted)
	for _, query := range []string{"SELECT count(*) FROM application", "SELECT count(*) FROM epoch", "SELECT count(*) FROM tournaments"} {
		var count int
		require.NoError(t, writer.db.QueryRow(ctx, query).Scan(&count))
		require.Zero(t, count, "deletion must remove the complete committed projection")
	}
}

func TestTournamentEventWindowIsAtomic(t *testing.T) {
	for _, test := range []struct {
		name  string
		fault func([]*repository.TournamentEventBatch)
	}{
		{name: "child row", fault: func(batches []*repository.TournamentEventBatch) {
			child := *batches[1].Tournament
			child.ParentMatchIDHash = new(repotest.UniqueHash())
			batches[1].Tournament = &child
		}},
		{name: "child event", fault: func(batches []*repository.TournamentEventBatch) {
			commitment := *batches[1].Commitments[0]
			commitment.TournamentAddress = repotest.UniqueAddress()
			batches[1].Commitments = []*Commitment{&commitment}
		}},
		{name: "last root event", fault: func(batches []*repository.TournamentEventBatch) {
			last := batches[len(batches)-1]
			invalid := repotest.NewMatchBuilder(last.Tournament.ApplicationID).
				WithEpochIndex(last.Tournament.EpochIndex).WithTournamentAddress(last.Tournament.Address).Build()
			invalid.Snapshot.AsOfBlock = 100
			last.Matches = append(last.Matches, invalid)
		}},
		{name: "duplicate commitment", fault: func(batches []*repository.TournamentEventBatch) {
			batches[1].Commitments = append(batches[1].Commitments, batches[1].Commitments[0])
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			endpoint, err := db.GetTestDatabaseEndpoint()
			if err != nil {
				t.Skip(err)
			}
			require.NoError(t, db.SetupTestPostgres(endpoint))
			raw, err := NewPostgresRepository(t.Context(), endpoint, 1, 0)
			require.NoError(t, err)
			t.Cleanup(raw.Close)
			r := raw.(*PostgresRepository)
			app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, raw)
			epoch0 := repotest.NewEpochBuilder(app.ID).WithIndex(0).WithStatus(EpochStatus_Closed).
				WithBlocks(0, 9).WithInputBounds(0, 0).Build()
			epoch1 := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(EpochStatus_Closed).
				WithBlocks(9, 19).WithInputBounds(0, 0).Build()
			require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
				map[*Epoch][]*Input{epoch0: {}, epoch1: {}}, 20))

			root := repotest.NewTournamentBuilder(app.ID).Build()
			root.Snapshot.AsOfBlock = 50
			require.NoError(t, r.CreateTournament(t.Context(), app.Name, root))
			updatedRoot := *root
			updatedRoot.Snapshot.AsOfBlock = 100
			updatedRoot.Snapshot.AcceptsJoins = false
			makeBatch := func(tournament *Tournament) *repository.TournamentEventBatch {
				tournament.Snapshot.AsOfBlock = 100
				tournament.Snapshot.Standing = TournamentStandingMatchesActive
				one := repotest.NewCommitmentBuilder(app.ID).WithEpochIndex(tournament.EpochIndex).
					WithTournamentAddress(tournament.Address).Build()
				two := repotest.NewCommitmentBuilder(app.ID).WithEpochIndex(tournament.EpochIndex).
					WithTournamentAddress(tournament.Address).Build()
				match := repotest.NewMatchBuilder(app.ID).WithEpochIndex(tournament.EpochIndex).
					WithTournamentAddress(tournament.Address).WithCommitmentOne(one.Commitment).
					WithCommitmentTwo(two.Commitment).Build()
				one.Snapshot.AsOfBlock = 100
				two.Snapshot.AsOfBlock = 100
				match.Snapshot.AsOfBlock = 100
				return &repository.TournamentEventBatch{Tournament: tournament,
					Commitments: []*Commitment{one, two}, Matches: []*Match{match}}
			}
			parent := makeBatch(&updatedRoot)
			child := repotest.NewTournamentBuilder(app.ID).WithLevel(1).
				WithParent(root.Address, parent.Matches[0].IDHash).Build()
			secondRoot := repotest.NewTournamentBuilder(app.ID).WithEpochIndex(1).Build()
			good := []*repository.TournamentEventBatch{parent, makeBatch(child), makeBatch(secondRoot)}
			bad := make([]*repository.TournamentEventBatch, len(good))
			for i, batch := range good {
				batchCopy := *batch
				bad[i] = &batchCopy
			}
			test.fault(bad)
			require.Error(t, r.StoreTournamentEvents(t.Context(), app.ID, bad, 100))
			storedRoot, err := r.GetTournament(t.Context(), app.Name, root.Address.Hex())
			require.NoError(t, err)
			require.Equal(t, root.Snapshot, storedRoot.Snapshot, "projection update must roll back with the last batch")
			storedApp, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Zero(t, storedApp.LastTournamentCheckBlock)
			for _, target := range []struct {
				query string
				count int
			}{
				{"SELECT count(*) FROM tournaments", 1}, {"SELECT count(*) FROM commitments", 0},
				{"SELECT count(*) FROM matches", 0}, {"SELECT count(*) FROM match_advances", 0},
			} {
				var count int
				require.NoError(t, r.db.QueryRow(t.Context(), target.query).Scan(&count))
				require.Equal(t, target.count, count, target.query)
			}

			require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, good, 100))
			storedApp, err = r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.Equal(t, uint64(100), storedApp.LastTournamentCheckBlock)
			for _, batch := range good {
				stored, err := r.GetTournament(t.Context(), app.Name, batch.Tournament.Address.Hex())
				require.NoError(t, err)
				require.NotNil(t, stored)
				require.Equal(t, batch.Tournament.Snapshot, stored.Snapshot)
				commitments, total, err := r.ListCommitments(t.Context(), app.Name,
					repository.CommitmentFilter{TournamentAddress: new(batch.Tournament.Address.Hex())}, repository.Pagination{}, false)
				require.NoError(t, err)
				require.Equal(t, uint64(2), total)
				require.Len(t, commitments, 2, "retry must not duplicate the rolled-back event rows")
			}
		})
	}
}
