// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func (r *postgresRepository) GetReport(
	ctx context.Context,
	nameOrAddress string,
	reportIndex uint64,
) (*model.Report, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	sel := table.Report.
		SELECT(
			table.Report.InputEpochApplicationID,
			table.Report.InputIndex,
			table.Report.Index,
			table.Report.RawData,
			table.Report.CreatedAt,
			table.Report.UpdatedAt,
		).
		FROM(
			table.Report.
				INNER_JOIN(table.Application,
					table.Report.InputEpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Report.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", reportIndex)))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var rp model.Report
	err = row.Scan(
		&rp.InputEpochApplicationID,
		&rp.InputIndex,
		&rp.Index,
		&rp.RawData,
		&rp.CreatedAt,
		&rp.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rp, nil
}

func (r *postgresRepository) ListReports(
	ctx context.Context,
	nameOrAddress string,
	f repository.ReportFilter,
	p repository.Pagination,
) ([]*model.Report, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	sel := table.Report.
		SELECT(
			table.Report.InputEpochApplicationID,
			table.Report.InputIndex,
			table.Report.Index,
			table.Report.RawData,
			table.Report.CreatedAt,
			table.Report.UpdatedAt,
		).
		FROM(
			table.Report.
				INNER_JOIN(table.Application,
					table.Report.InputEpochApplicationID.EQ(table.Application.ID),
				),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.InputIndex != nil {
		conditions = append(conditions, table.Report.InputIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.InputIndex))))
	}

	sel = sel.WHERE(postgres.AND(conditions...)).ORDER_BY(table.Report.Index.ASC())

	if p.Limit > 0 {
		sel = sel.LIMIT(p.Limit)
	}
	if p.Offset > 0 {
		sel = sel.OFFSET(p.Offset)
	}

	sqlStr, args := sel.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
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
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, &rp)
	}
	return reports, nil
}
