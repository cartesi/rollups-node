// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, 0, err
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
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
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
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.InputIndex != nil {
		conditions = append(conditions, table.Report.InputIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.InputIndex))))
	}

	if f.EpochIndex != nil {
		conditions = append(conditions, table.Input.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.EpochIndex))))
		conditions = append(conditions, table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())))
	}

	sel = sel.WHERE(postgres.AND(conditions...))

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
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []*model.Report
	var total uint64
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
			&total,
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
