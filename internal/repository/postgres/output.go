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

func (r *postgresRepository) GetOutput(
	ctx context.Context,
	nameOrAddress string,
	outputIndex uint64,
) (*model.Output, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	sel := table.Output.
		SELECT(
			table.Output.InputEpochApplicationID,
			table.Output.InputIndex,
			table.Output.Index,
			table.Output.RawData,
			table.Output.Hash,
			table.Output.OutputHashesSiblings,
			table.Output.ExecutionTransactionHash,
			table.Output.CreatedAt,
			table.Output.UpdatedAt,
		).
		FROM(
			table.Output.
				INNER_JOIN(table.Application,
					table.Output.InputEpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Output.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", outputIndex)))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var o model.Output
	err = row.Scan(
		&o.InputEpochApplicationID,
		&o.InputIndex,
		&o.Index,
		&o.RawData,
		&o.Hash,
		&o.OutputHashesSiblings,
		&o.ExecutionTransactionHash,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *postgresRepository) UpdateOutputsExecution(
	ctx context.Context,
	nameOrAddress string,
	outputs []*model.Output,
	lastOutputCheckBlock uint64,
) error {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}

	for _, o := range outputs {
		if o.ExecutionTransactionHash == nil {
			return errors.Join(
				fmt.Errorf("output ExecutionTransactionHash must be not nil when updating app %s output %d", nameOrAddress, o.Index),
				tx.Rollback(ctx),
			)
		}
		updStmt := table.Output.
			UPDATE(
				table.Output.ExecutionTransactionHash,
			).
			SET(
				postgres.Bytea(o.ExecutionTransactionHash.Bytes()),
			).
			FROM(
				table.Application,
			).
			WHERE(
				whereClause.
					AND(table.Output.InputEpochApplicationID.EQ(table.Application.ID)).
					AND(table.Output.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", o.Index)))),
			)

		sqlStr, args := updStmt.Sql()
		cmd, err := r.db.Exec(ctx, sqlStr, args...)
		if err != nil {
			return errors.Join(err, tx.Rollback(ctx))
		}
		if cmd.RowsAffected() != 1 {
			return errors.Join(
				fmt.Errorf("no row affected when updating app %s epoch %d", nameOrAddress, o.Index),
				tx.Rollback(ctx),
			)
		}
	}

	// Update last claim check block
	appUpdateStmt := table.Application.
		UPDATE(
			table.Application.LastOutputCheckBlock,
		).
		SET(
			postgres.RawFloat(fmt.Sprintf("%d", lastOutputCheckBlock)),
		).
		WHERE(whereClause)

	sqlStr, args := appUpdateStmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	return nil
}

func (r *postgresRepository) ListOutputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.OutputFilter,
	p repository.Pagination,
) ([]*model.Output, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	sel := table.Output.
		SELECT(
			table.Output.InputEpochApplicationID,
			table.Output.InputIndex,
			table.Output.Index,
			table.Output.RawData,
			table.Output.Hash,
			table.Output.OutputHashesSiblings,
			table.Output.ExecutionTransactionHash,
			table.Output.CreatedAt,
			table.Output.UpdatedAt,
		).
		FROM(
			table.Output.
				INNER_JOIN(
					table.Application,
					table.Output.InputEpochApplicationID.EQ(table.Application.ID),
				),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.BlockRange != nil {
		sel = sel.
			FROM( // Overwrite FROM clause to include input table
				table.Output.INNER_JOIN(
					table.Application,
					table.Output.InputEpochApplicationID.EQ(table.Application.ID),
				).
					INNER_JOIN(
						table.Input,
						table.Output.InputIndex.EQ(table.Input.Index).
							AND(table.Output.InputEpochApplicationID.EQ(table.Input.EpochApplicationID)),
					),
			)

		conditions = append(conditions, table.Input.BlockNumber.BETWEEN(
			postgres.RawFloat(fmt.Sprintf("%d", f.BlockRange.Start)),
			postgres.RawFloat(fmt.Sprintf("%d", f.BlockRange.End)),
		))

		conditions = append(conditions, table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())))
	}

	if f.InputIndex != nil {
		conditions = append(conditions, table.Output.InputIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.InputIndex))))
	}

	sel = sel.WHERE(postgres.AND(conditions...)).ORDER_BY(table.Output.Index.ASC())

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

	var outputs []*model.Output
	for rows.Next() {
		var out model.Output
		err := rows.Scan(
			&out.InputEpochApplicationID,
			&out.InputIndex,
			&out.Index,
			&out.RawData,
			&out.Hash,
			&out.OutputHashesSiblings,
			&out.ExecutionTransactionHash,
			&out.CreatedAt,
			&out.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, &out)
	}
	return outputs, nil
}
