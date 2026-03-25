// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"

	"github.com/go-jet/jet/v2/postgres"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func (r *PostgresRepository) ListStateHashes(
	ctx context.Context,
	nameOrAddress string,
	f repository.StateHashFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.StateHash, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.StateHashes.
		INNER_JOIN(table.Application,
			table.StateHashes.InputEpochApplicationID.EQ(table.Application.ID),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.StateHashes.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.StateHashes.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.StateHashes.
		SELECT(
			table.StateHashes.InputEpochApplicationID,
			table.StateHashes.EpochIndex,
			table.StateHashes.InputIndex,
			table.StateHashes.Index,
			table.StateHashes.MachineHash,
			table.StateHashes.Repetitions,
			table.StateHashes.CreatedAt,
			table.StateHashes.UpdatedAt,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.StateHashes.Index.DESC())
	} else {
		sel = sel.ORDER_BY(table.StateHashes.Index.ASC())
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

	var stateHashes []*model.StateHash
	for rows.Next() {
		var sh model.StateHash
		err := rows.Scan(
			&sh.InputEpochApplicationID,
			&sh.EpochIndex,
			&sh.InputIndex,
			&sh.Index,
			&sh.MachineHash,
			&sh.Repetitions,
			&sh.CreatedAt,
			&sh.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		stateHashes = append(stateHashes, &sh)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return stateHashes, total, nil
}
