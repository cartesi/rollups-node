// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"math"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
)

func (r *PostgresRepository) GetApplicationIDBoundary(ctx context.Context, f repository.ApplicationFilter) (int64, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	stmt := table.Application.SELECT(postgres.COALESCE(postgres.MAX(table.Application.ID), postgres.Int64(0)))
	if conditions := applicationConditions(f); len(conditions) > 0 {
		stmt = stmt.WHERE(postgres.AND(conditions...))
	}
	query, args := stmt.Sql()
	var boundary int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&boundary)
	return boundary, err
}

func (r *PostgresRepository) ListApplicationsByID(
	ctx context.Context, f repository.ApplicationFilter, afterID, throughID int64, limit uint64,
) ([]*model.Application, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	if limit == 0 || limit > math.MaxInt64 {
		return nil, errors.New("application page requires a bounded positive limit")
	}
	if afterID < 0 || throughID < 0 {
		return nil, errors.New("application cursor cannot be negative")
	}
	if afterID >= throughID {
		return nil, nil
	}
	f.AfterID = &afterID
	f.ThroughID = &throughID
	apps, _, err := r.listApplications(ctx, f, repository.Pagination{Limit: limit}, false, true, true)
	return apps, err
}
