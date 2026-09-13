// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateMatchAdvanced(ctx context.Context, nameOrAddress string, value *MatchAdvanced) error {
	if value == nil {
		return fmt.Errorf("cannot create a nil matchAdvance")
	}
	applicationID := table.Application.SELECT(table.Application.ID).WHERE(getWhereClauseFromNameOrAddress(nameOrAddress))
	values := matchAdvanceValues(value)
	values[0] = applicationID
	stmt := table.MatchAdvances.INSERT(matchAdvanceColumns).VALUES(values[0], values[1:]...)
	sqlStr, args := stmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) GetMatchAdvanced(
	ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string,
	idHashHex string, txHash common.Hash, logIndex uint64,
) (*MatchAdvanced, error) {
	sel := table.MatchAdvances.SELECT(matchAdvanceColumns, table.MatchAdvances.CreatedAt, table.MatchAdvances.UpdatedAt).
		FROM(table.MatchAdvances.INNER_JOIN(table.Application, table.MatchAdvances.ApplicationID.EQ(table.Application.ID))).
		WHERE(postgres.AND(
			getWhereClauseFromNameOrAddress(nameOrAddress),
			table.MatchAdvances.EpochIndex.EQ(uint64Expr(epochIndex)),
			table.MatchAdvances.TournamentAddress.EQ(postgres.Bytea(common.HexToAddress(tournamentAddress).Bytes())),
			table.MatchAdvances.IDHash.EQ(postgres.Bytea(common.HexToHash(idHashHex).Bytes())),
			table.MatchAdvances.TxHash.EQ(postgres.Bytea(txHash.Bytes())),
			table.MatchAdvances.LogIndex.EQ(uint64Expr(logIndex)),
		))
	sqlStr, args := sel.Sql()
	value, err := scanMatchAdvanced(r.db.QueryRow(ctx, sqlStr, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func (r *PostgresRepository) ListMatchAdvances(
	ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string,
	idHashHex string, p repository.Pagination, descending bool,
) ([]*MatchAdvanced, uint64, error) {
	if p.Limit > math.MaxInt64 || p.Offset > math.MaxInt64 {
		return nil, 0, fmt.Errorf("pagination exceeds PostgreSQL integer range")
	}
	from := table.MatchAdvances.INNER_JOIN(table.Application, table.MatchAdvances.ApplicationID.EQ(table.Application.ID))
	conditions := []postgres.BoolExpression{
		getWhereClauseFromNameOrAddress(nameOrAddress),
		table.MatchAdvances.EpochIndex.EQ(uint64Expr(epochIndex)),
		table.MatchAdvances.TournamentAddress.EQ(postgres.Bytea(common.HexToAddress(tournamentAddress).Bytes())),
		table.MatchAdvances.IDHash.EQ(postgres.Bytea(common.HexToHash(idHashHex).Bytes())),
	}
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	countStmt := table.MatchAdvances.SELECT(postgres.COUNT(postgres.STAR)).FROM(from).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil || total == 0 {
		return nil, total, err
	}
	sel := table.MatchAdvances.SELECT(matchAdvanceColumns, table.MatchAdvances.CreatedAt, table.MatchAdvances.UpdatedAt).
		FROM(from).WHERE(postgres.AND(conditions...))
	if descending {
		sel = sel.ORDER_BY(table.MatchAdvances.BlockNumber.DESC(), table.MatchAdvances.LogIndex.DESC(), table.MatchAdvances.TxHash.DESC())
	} else {
		sel = sel.ORDER_BY(table.MatchAdvances.BlockNumber.ASC(), table.MatchAdvances.LogIndex.ASC(), table.MatchAdvances.TxHash.ASC())
	}
	if p.Limit > 0 {
		sel = sel.LIMIT(int64(p.Limit))
	}
	if p.Offset > 0 {
		sel = sel.OFFSET(int64(p.Offset))
	}
	sqlStr, args := sel.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var values []*MatchAdvanced
	for rows.Next() {
		value, err := scanMatchAdvanced(rows)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return values, total, nil
}
