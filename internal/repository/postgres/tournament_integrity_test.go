// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const (
	integrityFirstCheckpoint = uint64(100)
	integrityNextCheckpoint  = uint64(200)
)

func newTournamentIntegrityFixture(t *testing.T) (*PostgresRepository, *Application, *Tournament, *Match) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, r)
	epoch := repotest.NewEpochBuilder(app.ID).WithStatus(EpochStatus_Closed).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*Epoch][]*Input{epoch: {}}, epoch.LastBlock))
	tournament := repotest.NewTournamentBuilder(app.ID).Build()
	one := repotest.NewCommitmentBuilder(app.ID).WithTournamentAddress(tournament.Address).Build()
	two := repotest.NewCommitmentBuilder(app.ID).WithTournamentAddress(tournament.Address).Build()
	match := repotest.NewMatchBuilder(app.ID).WithTournamentAddress(tournament.Address).
		WithCommitmentOne(one.Commitment).WithCommitmentTwo(two.Commitment).Build()
	tournament.Snapshot.AsOfBlock = integrityFirstCheckpoint
	one.Snapshot.AsOfBlock = integrityFirstCheckpoint
	two.Snapshot.AsOfBlock = integrityFirstCheckpoint
	match.Snapshot.AsOfBlock = integrityFirstCheckpoint
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{{
		Tournament: tournament, Commitments: []*Commitment{one, two},
	}}, integrityFirstCheckpoint))
	return r, app, tournament, match
}

func TestMatchDeletionMetadataRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name string
		hash *common.Hash
	}{
		{name: "live"},
		{name: "deleted with zero hash", hash: new(common.Hash)},
		{name: "deleted with nonzero hash", hash: new(common.HexToHash("0x123"))},
	} {
		for _, bulk := range []bool{false, true} {
			writer := "create"
			if bulk {
				writer = "event window"
			}
			t.Run(test.name+"/"+writer, func(t *testing.T) {
				r, app, tournament, match := newTournamentIntegrityFixture(t)
				tournament.Snapshot.AsOfBlock = integrityNextCheckpoint
				match.Snapshot.AsOfBlock = integrityNextCheckpoint
				match.DeletionTxHash = test.hash
				if test.hash != nil {
					match.DeletionReason = MatchDeletionReason_TIMEOUT
					match.DeletionBlockNumber = integrityNextCheckpoint
					match.DeletionLogIndex = new(uint64(0))
					match.Snapshot.Phase = MatchPhaseUninitialized
					match.Snapshot.Bisection = nil
				}
				if bulk {
					require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{{
						Tournament: tournament, Matches: []*Match{match},
					}}, integrityNextCheckpoint))
				} else {
					require.NoError(t, r.CreateMatch(t.Context(), app.Name, match))
				}
				stored, err := r.GetMatch(t.Context(), app.Name, match.EpochIndex, tournament.Address.Hex(), match.IDHash.Hex())
				require.NoError(t, err)
				require.Equal(t, match.DeletionReason, stored.DeletionReason)
				require.Equal(t, match.DeletionBlockNumber, stored.DeletionBlockNumber)
				require.Equal(t, match.DeletionTxHash, stored.DeletionTxHash)
				listed, _, err := r.ListMatches(t.Context(), app.Name, repository.MatchFilter{}, repository.Pagination{}, false)
				require.NoError(t, err)
				require.Len(t, listed, 1)
				require.Equal(t, match.DeletionTxHash, listed[0].DeletionTxHash)
				var isNull bool
				require.NoError(t, r.db.QueryRow(t.Context(), `SELECT deletion_tx_hash IS NULL FROM matches`).Scan(&isNull))
				require.Equal(t, test.hash == nil, isNull)
			})
		}
	}
}

func TestMatchDeletionMetadataRejectsIncompleteTuples(t *testing.T) {
	r, app, _, match := newTournamentIntegrityFixture(t)
	require.NoError(t, r.CreateMatch(t.Context(), app.Name, match))
	for _, test := range []struct {
		name   string
		reason MatchDeletionReason
		block  any
		hash   *common.Hash
		code   string
	}{
		{"live with transaction", MatchDeletionReason_NOT_DELETED, 0, new(common.Hash), "23514"},
		{"live with block", MatchDeletionReason_NOT_DELETED, 1, nil, "23514"},
		{"deleted without transaction", MatchDeletionReason_TIMEOUT, 1, nil, "23514"},
		{"deleted without block", MatchDeletionReason_TIMEOUT, 0, new(common.Hash), "23514"},
		{"live with null block", MatchDeletionReason_NOT_DELETED, nil, nil, "23502"},
		{"deleted with null block", MatchDeletionReason_TIMEOUT, nil, new(common.Hash), "23502"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Keep the current phase coherent so this test isolates the event tuple.
			_, err := r.db.Exec(t.Context(), `UPDATE matches SET
                deletion_reason = $1::"MatchDeletionReason", deletion_block_number = $2, deletion_tx_hash = $3,
                deletion_log_index = CASE WHEN $1 = 'NOT_DELETED' THEN NULL ELSE 0 END,
                phase = CASE WHEN $1 = 'NOT_DELETED' THEN 'BISECTING'::"MatchPhase" ELSE 'UNINITIALIZED'::"MatchPhase" END,
                revealing_parent = CASE WHEN $1 = 'NOT_DELETED' THEN revealing_parent ELSE NULL END,
                waiting_left = CASE WHEN $1 = 'NOT_DELETED' THEN waiting_left ELSE NULL END,
                waiting_right = CASE WHEN $1 = 'NOT_DELETED' THEN waiting_right ELSE NULL END,
                segment_start_position = CASE WHEN $1 = 'NOT_DELETED' THEN segment_start_position ELSE NULL END,
                segment_start_cycle = CASE WHEN $1 = 'NOT_DELETED' THEN segment_start_cycle ELSE NULL END,
                current_height = CASE WHEN $1 = 'NOT_DELETED' THEN current_height ELSE NULL END,
                responder = CASE WHEN $1 = 'NOT_DELETED' THEN responder ELSE NULL END`,
				test.reason.String(), test.block, hashToBytes(test.hash))
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, test.code, pgErr.Code)
			if test.block == nil {
				require.Equal(t, "deletion_block_number", pgErr.ColumnName)
			} else {
				require.Equal(t, "matches_deletion_columns_set_together", pgErr.ConstraintName)
			}
		})
	}
}

func TestTournamentWindowRollsBackInvalidDeletion(t *testing.T) {
	r, app, tournament, match := newTournamentIntegrityFixture(t)
	require.NoError(t, r.CreateMatch(t.Context(), app.Name, match))
	tournament.Snapshot.AsOfBlock = integrityNextCheckpoint
	tournament.Snapshot.Standing = TournamentStandingRootWinner
	tournament.Snapshot.FinishedAtBlock = integrityNextCheckpoint
	tournament.Snapshot.Candidate = new(match.CommitmentOne)
	tournament.Snapshot.WinnerCommitment = new(match.CommitmentOne)
	tournament.Snapshot.FinalStateHash = new(common.Hash)
	deletion := *match
	deletion.Winner = WinnerCommitment_ONE
	deletion.DeletionReason = MatchDeletionReason_TIMEOUT
	deletion.DeletionBlockNumber = integrityNextCheckpoint
	deletion.DeletionLogIndex = new(uint64(0))
	deletion.Snapshot = MatchSnapshot{AsOfBlock: integrityNextCheckpoint, Phase: MatchPhaseUninitialized, TimeoutOutcome: MatchTimeoutNone}
	batch := &repository.TournamentEventBatch{Tournament: tournament, Matches: []*Match{&deletion}}
	err := r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{batch}, integrityNextCheckpoint)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "matches_deletion_columns_set_together", pgErr.ConstraintName)
	stored, err := r.GetTournament(t.Context(), app.Name, tournament.Address.Hex())
	require.NoError(t, err)
	require.Zero(t, stored.Snapshot.FinishedAtBlock)
	require.Nil(t, stored.Snapshot.WinnerCommitment)
	require.Nil(t, stored.Snapshot.FinalStateHash)
	storedMatch, err := r.GetMatch(t.Context(), app.Name, match.EpochIndex, tournament.Address.Hex(), match.IDHash.Hex())
	require.NoError(t, err)
	require.Equal(t, MatchDeletionReason_NOT_DELETED, storedMatch.DeletionReason)
	require.Nil(t, storedMatch.DeletionTxHash)
	storedApp, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, integrityFirstCheckpoint, storedApp.LastTournamentCheckBlock)

	// The atomic path replaces the removed single-row update methods. Verify
	// its successful update as well as the failed window above.
	deletion.DeletionTxHash = new(common.Hash)
	require.NoError(t, r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{batch}, integrityNextCheckpoint))
	stored, err = r.GetTournament(t.Context(), app.Name, tournament.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, tournament.Snapshot, stored.Snapshot)
	storedMatch, err = r.GetMatch(t.Context(), app.Name, match.EpochIndex, tournament.Address.Hex(), match.IDHash.Hex())
	require.NoError(t, err)
	require.Equal(t, deletion.Winner, storedMatch.Winner)
	require.Equal(t, deletion.DeletionReason, storedMatch.DeletionReason)
	require.Equal(t, deletion.DeletionBlockNumber, storedMatch.DeletionBlockNumber)
	require.Equal(t, deletion.DeletionTxHash, storedMatch.DeletionTxHash)
	storedApp, err = r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, integrityNextCheckpoint, storedApp.LastTournamentCheckBlock)
}

func TestTournamentAddressIdentifiesOneEpochPerApplication(t *testing.T) {
	r, app, tournament, _ := newTournamentIntegrityFixture(t)
	// Re-observing the same identity remains idempotent.
	require.NoError(t, r.CreateTournament(t.Context(), app.Name, tournament))
	nextEpoch := repotest.NewEpochBuilder(app.ID).WithIndex(1).WithStatus(EpochStatus_Closed).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*Epoch][]*Input{nextEpoch: {}}, nextEpoch.LastBlock))
	duplicate := *tournament
	duplicate.EpochIndex = nextEpoch.Index
	err := r.CreateTournament(t.Context(), app.Name, &duplicate)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "tournaments_address_idx", pgErr.ConstraintName)

	updated := *tournament
	updated.Snapshot.AsOfBlock = integrityNextCheckpoint
	updated.Snapshot.Standing = TournamentStandingRootFailed
	updated.Snapshot.FinishedAtBlock = integrityNextCheckpoint
	duplicate.Snapshot.AsOfBlock = integrityNextCheckpoint
	err = r.StoreTournamentEvents(t.Context(), app.ID, []*repository.TournamentEventBatch{
		{Tournament: &updated}, {Tournament: &duplicate},
	}, integrityNextCheckpoint)
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "tournaments_address_idx", pgErr.ConstraintName)
	stored, err := r.GetTournament(t.Context(), app.Name, tournament.Address.Hex())
	require.NoError(t, err)
	require.Equal(t, tournament.EpochIndex, stored.EpochIndex)
	require.Zero(t, stored.Snapshot.FinishedAtBlock)
	storedApp, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, integrityFirstCheckpoint, storedApp.LastTournamentCheckBlock)

	otherApp := repotest.NewApplicationBuilder().WithConsensus(Consensus_PRT).Create(t.Context(), t, r)
	otherEpoch := repotest.NewEpochBuilder(otherApp.ID).WithStatus(EpochStatus_Closed).Build()
	require.NoError(t, r.CreateEpochsAndInputs(t.Context(), otherApp.Name, map[*Epoch][]*Input{otherEpoch: {}}, otherEpoch.LastBlock))
	otherTournament := *tournament
	otherTournament.ApplicationID = otherApp.ID
	require.NoError(t, r.CreateTournament(t.Context(), otherApp.Name, &otherTournament), "the uniqueness key includes the application")
}
