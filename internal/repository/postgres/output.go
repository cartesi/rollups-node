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

func (r *PostgresRepository) GetOutput(
	ctx context.Context,
	nameOrAddress string,
	outputIndex uint64,
) (*model.Output, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

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
			table.Input.EpochIndex,
		).
		FROM(
			table.Output.INNER_JOIN(
				table.Application,
				table.Output.InputEpochApplicationID.EQ(table.Application.ID),
			).INNER_JOIN(
				table.Input,
				table.Output.InputIndex.EQ(table.Input.Index).
					AND(table.Output.InputEpochApplicationID.EQ(table.Input.EpochApplicationID)),
			),
		).
		WHERE(
			whereClause.
				AND(table.Output.Index.EQ(uint64Expr(outputIndex))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var o model.Output
	err := row.Scan(
		&o.InputEpochApplicationID,
		&o.InputIndex,
		&o.Index,
		&o.RawData,
		&o.Hash,
		&o.OutputHashesSiblings,
		&o.ExecutionTransactionHash,
		&o.CreatedAt,
		&o.UpdatedAt,
		&o.EpochIndex,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *PostgresRepository) UpdateOutputsExecution(
	ctx context.Context,
	nameOrAddress string,
	outputs []*model.Output,
	lastOutputCheckBlock uint64,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

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
					AND(table.Output.Index.EQ(uint64Expr(o.Index))),
			)

		sqlStr, args := updStmt.Sql()
		cmd, err := tx.Exec(ctx, sqlStr, args...)
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
			uint64Expr(lastOutputCheckBlock),
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

func (r *PostgresRepository) ListOutputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.OutputFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Output, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

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
			table.Input.EpochIndex,
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.Output.INNER_JOIN(
				table.Application,
				table.Output.InputEpochApplicationID.EQ(table.Application.ID),
			).INNER_JOIN(
				table.Input,
				table.Output.InputIndex.EQ(table.Input.Index).
					AND(table.Output.InputEpochApplicationID.EQ(table.Input.EpochApplicationID)),
			),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.BlockRange != nil {
		conditions = append(conditions, table.Input.BlockNumber.BETWEEN(
			uint64Expr(f.BlockRange.Start),
			uint64Expr(f.BlockRange.End),
		))
		conditions = append(conditions, table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())))
	}

	if f.EpochIndex != nil {
		conditions = append(conditions, table.Input.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
		conditions = append(conditions, table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())))
	}

	if f.InputIndex != nil {
		conditions = append(conditions, table.Output.InputIndex.EQ(uint64Expr(*f.InputIndex)))
	}

	if f.OutputType != nil {
		conditions = append(conditions,
			SubstrBytea(table.Output.RawData, 1, 4).EQ(postgres.Bytea(*f.OutputType)),
		)
	}

	if f.VoucherAddress != nil {
		conditions = append(conditions,
			SubstrBytea(table.Output.RawData, 17, 20).EQ(postgres.Bytea(f.VoucherAddress.Bytes())),
		)
	}

	sel = sel.WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.Output.Index.DESC())
	} else {
		sel = sel.ORDER_BY(table.Output.Index.ASC())
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

	var outputs []*model.Output
	var total uint64
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
			&out.EpochIndex,
			&total,
		)
		if err != nil {
			return nil, 0, err
		}
		outputs = append(outputs, &out)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return outputs, total, nil
}

func (r *PostgresRepository) GetLastOutputBeforeBlock(
	ctx context.Context,
	nameOrAddress string,
	block uint64,
) (*model.Output, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

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
			table.Input.EpochIndex,
		).
		FROM(
			table.Output.INNER_JOIN(
				table.Application,
				table.Output.InputEpochApplicationID.EQ(table.Application.ID),
			).INNER_JOIN(
				table.Input,
				table.Output.InputEpochApplicationID.EQ(table.Input.EpochApplicationID).AND(
					table.Output.InputIndex.EQ(table.Input.Index),
				),
			),
		).
		WHERE(
			postgres.AND(
				whereClause,
				table.Input.BlockNumber.LT(uint64Expr(block)),
				table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())),
			),
		).
		ORDER_BY(table.Output.Index.DESC()).
		LIMIT(1)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var out model.Output
	err := row.Scan(
		&out.InputEpochApplicationID,
		&out.InputIndex,
		&out.Index,
		&out.RawData,
		&out.Hash,
		&out.OutputHashesSiblings,
		&out.ExecutionTransactionHash,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.EpochIndex,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *PostgresRepository) GetNumberOfExecutedOutputs(
	ctx context.Context,
	nameOrAddress string,
) (uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Output.
		SELECT(postgres.COUNT(postgres.STAR)).
		FROM(
			table.Output.
				INNER_JOIN(table.Application,
					table.Output.InputEpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(whereClause.AND(table.Output.ExecutionTransactionHash.IS_NOT_NULL()))

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var count uint64
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
