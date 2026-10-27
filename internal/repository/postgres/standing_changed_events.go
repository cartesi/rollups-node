// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func insertStandingChangedChunk(ctx context.Context, tx pgx.Tx, appID int64, values []*model.StandingChanged) error {
	if len(values) == 0 {
		return nil
	}
	stmt := table.StandingChanges.INSERT(standingChangeColumns)
	for _, value := range values {
		if value == nil || value.ApplicationID != appID {
			return fmt.Errorf("invalid standing change application identity")
		}
		row := standingChangeValues(value)
		stmt = stmt.VALUES(row[0], row[1:]...)
	}
	query, args := stmt.ON_CONFLICT(
		table.StandingChanges.ApplicationID, table.StandingChanges.TxHash, table.StandingChanges.LogIndex,
	).DO_NOTHING().Sql()
	command, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() == int64(len(values)) {
		return nil
	}
	// Exact retries retain the original timestamps. A contradictory payload
	// must fail the owning interval before any projection or cursor commits.
	for _, value := range values {
		stmt := table.StandingChanges.SELECT(standingChangeColumns,
			table.StandingChanges.CreatedAt, table.StandingChanges.UpdatedAt).WHERE(postgres.AND(
			table.StandingChanges.ApplicationID.EQ(postgres.Int64(appID)),
			table.StandingChanges.TxHash.EQ(postgres.Bytea(value.TxHash.Bytes())),
			table.StandingChanges.LogIndex.EQ(uint64Expr(value.LogIndex)),
		))
		query, args := stmt.Sql()
		stored, err := scanStandingChanged(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return err
		}
		if !sameStandingChangedEvent(stored, value) {
			return fmt.Errorf("%w: transaction %s log %d", repository.ErrTournamentEventConflict, value.TxHash, value.LogIndex)
		}
	}
	return nil
}

func sameStandingChangedEvent(a, b *model.StandingChanged) bool {
	return a.ApplicationID == b.ApplicationID &&
		a.EpochIndex == b.EpochIndex &&
		a.TournamentAddress == b.TournamentAddress &&
		a.MatchCount == b.MatchCount &&
		sameOptionalHash(a.Dangling, b.Dangling) &&
		sameOptionalHash(a.ParentCommitment, b.ParentCommitment) &&
		a.ResultAt == b.ResultAt &&
		a.WinnerExpiresAt == b.WinnerExpiresAt &&
		a.BlockNumber == b.BlockNumber &&
		a.TxHash == b.TxHash &&
		a.LogIndex == b.LogIndex
}

func sameOptionalHash(a, b *common.Hash) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
