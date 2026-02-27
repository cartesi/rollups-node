// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

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

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, 0, err
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
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.StateHashes.INNER_JOIN(
				table.Application,
				table.StateHashes.InputEpochApplicationID.EQ(table.Application.ID),
			),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.StateHashes.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.EpochIndex))))
	}

	sel = sel.WHERE(postgres.AND(conditions...))

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
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var stateHashes []*model.StateHash
	var total uint64
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
			&total,
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
