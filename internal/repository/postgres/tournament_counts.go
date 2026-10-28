// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
)

// GetTournamentEventCounts reads immutable event facts in one statement snapshot.
// The event's own block, rather than the projection's later AsOfBlock, bounds
// each count. A refreshed projection can therefore reconstruct an older cursor.
func (r *PostgresRepository) GetTournamentEventCounts(
	ctx context.Context, applicationID int64, epochIndex uint64, tournament common.Address, throughBlock uint64,
) (repository.TournamentEventCounts, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	commitments := table.Commitments.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		table.Commitments.ApplicationID.EQ(postgres.Int64(applicationID)).
			AND(table.Commitments.EpochIndex.EQ(uint64Expr(epochIndex))).
			AND(table.Commitments.TournamentAddress.EQ(postgres.Bytea(tournament.Bytes()))).
			AND(table.Commitments.BlockNumber.LT_EQ(uint64Expr(throughBlock))))
	matchScope := table.Matches.ApplicationID.EQ(postgres.Int64(applicationID)).
		AND(table.Matches.EpochIndex.EQ(uint64Expr(epochIndex))).
		AND(table.Matches.TournamentAddress.EQ(postgres.Bytea(tournament.Bytes())))
	matches := table.Matches.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		matchScope.AND(table.Matches.BlockNumber.LT_EQ(uint64Expr(throughBlock))))
	advances := table.MatchAdvances.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		table.MatchAdvances.ApplicationID.EQ(postgres.Int64(applicationID)).
			AND(table.MatchAdvances.EpochIndex.EQ(uint64Expr(epochIndex))).
			AND(table.MatchAdvances.TournamentAddress.EQ(postgres.Bytea(tournament.Bytes()))).
			AND(table.MatchAdvances.BlockNumber.LT_EQ(uint64Expr(throughBlock))))
	seals := table.Matches.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		matchScope.AND(table.Matches.SealBlockNumber.LT_EQ(uint64Expr(throughBlock))))
	deletions := table.Matches.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		matchScope.AND(table.Matches.DeletionBlockNumber.GT(uint64Expr(0))).
			AND(table.Matches.DeletionBlockNumber.LT_EQ(uint64Expr(throughBlock))))
	children := table.Tournaments.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		table.Tournaments.ApplicationID.EQ(postgres.Int64(applicationID)).
			AND(table.Tournaments.EpochIndex.EQ(uint64Expr(epochIndex))).
			AND(table.Tournaments.ParentTournamentAddress.EQ(postgres.Bytea(tournament.Bytes()))).
			AND(table.Tournaments.CreationBlockNumber.LT_EQ(uint64Expr(throughBlock))))
	standing := table.StandingChanges.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		table.StandingChanges.ApplicationID.EQ(postgres.Int64(applicationID)).
			AND(table.StandingChanges.EpochIndex.EQ(uint64Expr(epochIndex))).
			AND(table.StandingChanges.TournamentAddress.EQ(postgres.Bytea(tournament.Bytes()))).
			AND(table.StandingChanges.BlockNumber.LT_EQ(uint64Expr(throughBlock))))
	stmt := postgres.SELECT(commitments, matches, advances, seals, deletions, children, standing)
	query, args := stmt.Sql()
	var counts repository.TournamentEventCounts
	err := r.db.QueryRow(ctx, query, args...).Scan(&counts.CommitmentJoined, &counts.MatchCreated,
		&counts.MatchAdvanced, &counts.LeafMatchSealed, &counts.MatchDeleted, &counts.NewInnerTournament, &counts.StandingChanged)
	if err != nil {
		return counts, fmt.Errorf("counting tournament %s events through block %d: %w", tournament, throughBlock, err)
	}
	return counts, nil
}
