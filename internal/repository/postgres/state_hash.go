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

func (r *PostgresRepository) CountEpochStateHashes(ctx context.Context, appID int64, epochIndex uint64) (uint64, error) {
	stmt := table.StateHashes.SELECT(postgres.COUNT(postgres.STAR)).WHERE(
		postgres.AND(
			table.StateHashes.InputEpochApplicationID.EQ(postgres.Int64(appID)),
			table.StateHashes.EpochIndex.EQ(uint64Expr(epochIndex)),
		),
	)
	query, args := stmt.Sql()
	var count uint64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *PostgresRepository) ListEpochStateHashes(
	ctx context.Context, appID int64, epochIndex uint64, afterIndex *uint64, limit uint64,
) ([]model.EpochStateHash, error) {
	if limit == 0 || limit > repository.MaxStateHashPageSize {
		return nil, fmt.Errorf("state hash page limit must be between 1 and %d", repository.MaxStateHashPageSize)
	}
	conditions := []postgres.BoolExpression{
		table.StateHashes.InputEpochApplicationID.EQ(postgres.Int64(appID)),
		table.StateHashes.EpochIndex.EQ(uint64Expr(epochIndex)),
	}
	if afterIndex != nil {
		conditions = append(conditions, table.StateHashes.Index.GT(uint64Expr(*afterIndex)))
	}
	stmt := table.StateHashes.SELECT(
		table.StateHashes.Index,
		table.StateHashes.InputIndex,
		table.StateHashes.MachineHash,
		table.StateHashes.Repetitions,
	).WHERE(postgres.AND(conditions...)).ORDER_BY(table.StateHashes.Index.ASC()).LIMIT(int64(limit))
	query, args := stmt.Sql()
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []model.EpochStateHash
	for rows.Next() {
		var row model.EpochStateHash
		if err := rows.Scan(&row.Index, &row.InputIndex, &row.MachineHash, &row.Repetitions); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
