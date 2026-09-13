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

func (r *PostgresRepository) GetBondEvent(
	ctx context.Context, nameOrAddress string, txHash common.Hash, logIndex uint64,
) (*BondEvent, error) {
	sel := table.BondEvents.SELECT(bondEventColumns, table.BondEvents.CreatedAt, table.BondEvents.UpdatedAt).
		FROM(table.BondEvents.INNER_JOIN(table.Application, table.BondEvents.ApplicationID.EQ(table.Application.ID))).
		WHERE(getWhereClauseFromNameOrAddress(nameOrAddress).AND(
			table.BondEvents.TxHash.EQ(postgres.Bytea(txHash.Bytes())).AND(table.BondEvents.LogIndex.EQ(uint64Expr(logIndex))),
		))
	sqlStr, args := sel.Sql()
	value, err := scanBondEvent(r.db.QueryRow(ctx, sqlStr, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func (r *PostgresRepository) ListBondEvents(
	ctx context.Context, nameOrAddress string, f repository.BondEventFilter, p repository.Pagination, descending bool,
) ([]*BondEvent, uint64, error) {
	if p.Limit > math.MaxInt64 || p.Offset > math.MaxInt64 {
		return nil, 0, fmt.Errorf("pagination exceeds PostgreSQL integer range")
	}
	from := table.BondEvents.INNER_JOIN(table.Application, table.BondEvents.ApplicationID.EQ(table.Application.ID))
	conditions := []postgres.BoolExpression{getWhereClauseFromNameOrAddress(nameOrAddress)}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.BondEvents.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
	}
	if f.TournamentAddress != nil {
		conditions = append(conditions, table.BondEvents.TournamentAddress.EQ(postgres.Bytea(f.TournamentAddress.Bytes())))
	}
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	countStmt := table.BondEvents.SELECT(postgres.COUNT(postgres.STAR)).FROM(from).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil || total == 0 {
		return nil, total, err
	}
	sel := table.BondEvents.SELECT(bondEventColumns, table.BondEvents.CreatedAt, table.BondEvents.UpdatedAt).
		FROM(from).WHERE(postgres.AND(conditions...))
	if descending {
		sel = sel.ORDER_BY(table.BondEvents.BlockNumber.DESC(), table.BondEvents.LogIndex.DESC(), table.BondEvents.TxHash.DESC())
	} else {
		sel = sel.ORDER_BY(table.BondEvents.BlockNumber.ASC(), table.BondEvents.LogIndex.ASC(), table.BondEvents.TxHash.ASC())
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
	var values []*BondEvent
	for rows.Next() {
		value, err := scanBondEvent(rows)
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
