// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) GetStandingChange(
	ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string,
	txHash common.Hash, logIndex uint64,
) (*model.StandingChanged, error) {
	sel := table.StandingChanges.SELECT(standingChangeColumns, table.StandingChanges.CreatedAt, table.StandingChanges.UpdatedAt).
		FROM(table.StandingChanges.INNER_JOIN(table.Application, table.StandingChanges.ApplicationID.EQ(table.Application.ID))).
		WHERE(postgres.AND(
			getWhereClauseFromNameOrAddress(nameOrAddress),
			table.StandingChanges.EpochIndex.EQ(uint64Expr(epochIndex)),
			table.StandingChanges.TournamentAddress.EQ(postgres.Bytea(common.HexToAddress(tournamentAddress).Bytes())),
			table.StandingChanges.TxHash.EQ(postgres.Bytea(txHash.Bytes())),
			table.StandingChanges.LogIndex.EQ(uint64Expr(logIndex)),
		))
	sqlStr, args := sel.Sql()
	value, err := scanStandingChanged(r.db.QueryRow(ctx, sqlStr, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func (r *PostgresRepository) ListStandingChanges(
	ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string,
	p repository.Pagination, descending bool,
) ([]*model.StandingChanged, uint64, error) {
	if p.Limit > math.MaxInt64 || p.Offset > math.MaxInt64 {
		return nil, 0, fmt.Errorf("pagination exceeds PostgreSQL integer range")
	}
	from := table.StandingChanges.INNER_JOIN(table.Application, table.StandingChanges.ApplicationID.EQ(table.Application.ID))
	conditions := []postgres.BoolExpression{
		getWhereClauseFromNameOrAddress(nameOrAddress),
		table.StandingChanges.EpochIndex.EQ(uint64Expr(epochIndex)),
		table.StandingChanges.TournamentAddress.EQ(postgres.Bytea(common.HexToAddress(tournamentAddress).Bytes())),
	}
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	countStmt := table.StandingChanges.SELECT(postgres.COUNT(postgres.STAR)).FROM(from).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil || total == 0 {
		return nil, total, err
	}
	sel := table.StandingChanges.SELECT(standingChangeColumns, table.StandingChanges.CreatedAt, table.StandingChanges.UpdatedAt).
		FROM(from).WHERE(postgres.AND(conditions...))
	if descending {
		sel = sel.ORDER_BY(table.StandingChanges.BlockNumber.DESC(),
			table.StandingChanges.LogIndex.DESC(), table.StandingChanges.TxHash.DESC())
	} else {
		sel = sel.ORDER_BY(table.StandingChanges.BlockNumber.ASC(),
			table.StandingChanges.LogIndex.ASC(), table.StandingChanges.TxHash.ASC())
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
	var values []*model.StandingChanged
	for rows.Next() {
		value, err := scanStandingChanged(rows)
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
