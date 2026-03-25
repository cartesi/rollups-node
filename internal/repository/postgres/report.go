// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-jet/jet/v2/postgres"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func (r *PostgresRepository) GetReport(
	ctx context.Context,
	nameOrAddress string,
	reportIndex uint64,
) (*model.Report, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Report.
		SELECT(
			table.Report.InputEpochApplicationID,
			table.Report.InputIndex,
			table.Report.Index,
			table.Report.RawData,
			table.Report.CreatedAt,
			table.Report.UpdatedAt,
			table.Input.EpochIndex,
		).
		FROM(
			table.Report.INNER_JOIN(
				table.Application,
				table.Report.InputEpochApplicationID.EQ(table.Application.ID),
			).INNER_JOIN(
				table.Input,
				table.Report.InputIndex.EQ(table.Input.Index).
					AND(table.Report.InputEpochApplicationID.EQ(table.Input.EpochApplicationID)),
			),
		).
		WHERE(
			whereClause.
				AND(table.Report.Index.EQ(uint64Expr(reportIndex))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var rp model.Report
	err := row.Scan(
		&rp.InputEpochApplicationID,
		&rp.InputIndex,
		&rp.Index,
		&rp.RawData,
		&rp.CreatedAt,
		&rp.UpdatedAt,
		&rp.EpochIndex,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rp, nil
}

func (r *PostgresRepository) ListReports(
	ctx context.Context,
	nameOrAddress string,
	f repository.ReportFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Report, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.Report.INNER_JOIN(
		table.Application,
		table.Report.InputEpochApplicationID.EQ(table.Application.ID),
	).INNER_JOIN(
		table.Input,
		table.Report.InputIndex.EQ(table.Input.Index).
			AND(table.Report.InputEpochApplicationID.EQ(table.Input.EpochApplicationID)),
	)

	conditions := []postgres.BoolExpression{whereClause}
	if f.InputIndex != nil {
		conditions = append(conditions, table.Report.InputIndex.EQ(uint64Expr(*f.InputIndex)))
	}

	if f.EpochIndex != nil {
		conditions = append(conditions, table.Input.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
		conditions = append(conditions, table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())))
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.Report.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.Report.
		SELECT(
			table.Report.InputEpochApplicationID,
			table.Report.InputIndex,
			table.Report.Index,
			table.Report.RawData,
			table.Report.CreatedAt,
			table.Report.UpdatedAt,
			table.Input.EpochIndex,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.Report.Index.DESC())
	} else {
		sel = sel.ORDER_BY(table.Report.Index.ASC())
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

	var reports []*model.Report
	for rows.Next() {
		var rp model.Report
		err := rows.Scan(
			&rp.InputEpochApplicationID,
			&rp.InputIndex,
			&rp.Index,
			&rp.RawData,
			&rp.CreatedAt,
			&rp.UpdatedAt,
			&rp.EpochIndex,
		)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, &rp)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return reports, total, nil
}
