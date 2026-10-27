// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

var (
	delegateCallVoucherSelector = []byte{0x10, 0x32, 0x1e, 0x8b}
	voucherSelector             = []byte{0x23, 0x7a, 0x81, 0x6f}
)

func (r *PostgresRepository) GetEpochOutputRange(
	ctx context.Context, appID int64, epochIndex uint64,
) (repository.EpochOutputRange, error) {
	stmt := table.Output.SELECT(
		postgres.COUNT(postgres.STAR),
		postgres.COALESCE(postgres.MIN(table.Output.Index), postgres.Int64(0)),
		postgres.COALESCE(postgres.MAX(table.Output.Index), postgres.Int64(0)),
	).FROM(table.Output.INNER_JOIN(table.Input,
		table.Output.InputEpochApplicationID.EQ(table.Input.EpochApplicationID).
			AND(table.Output.InputIndex.EQ(table.Input.Index)),
	)).WHERE(postgres.AND(
		table.Output.InputEpochApplicationID.EQ(postgres.Int64(appID)),
		table.Input.EpochIndex.EQ(uint64Expr(epochIndex)),
		table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String())),
	))
	query, args := stmt.Sql()
	var result repository.EpochOutputRange
	err := r.db.QueryRow(ctx, query, args...).Scan(&result.Count, &result.First, &result.Last)
	return result, err
}

type outputQueryer interface {
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
}

func (r *PostgresRepository) ScanOutputs(
	ctx context.Context, appID int64, first, last uint64, fn repository.OutputScanFunc,
) error {
	return scanOutputs(ctx, r.db, appID, first, last, fn)
}

func scanOutputs(
	ctx context.Context, queryer outputQueryer, appID int64, first, last uint64, fn repository.OutputScanFunc,
) error {
	span := last - first
	if first > last || span >= repository.MaxOutputRangeRows {
		return fmt.Errorf("output scan range must contain between 1 and %d indexes", repository.MaxOutputRangeRows)
	}
	stmt := table.Output.SELECT(postgres.CAST(table.Output.Index).AS_BIGINT(), table.Output.RawData).
		WHERE(postgres.AND(
			table.Output.InputEpochApplicationID.EQ(postgres.Int64(appID)),
			table.Output.Index.BETWEEN(uint64Expr(first), uint64Expr(last)),
		)).ORDER_BY(table.Output.Index.ASC()).LIMIT(int64(span) + 1)
	query, args := stmt.Sql()
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var index uint64
		var rawData []byte
		if err := rows.Scan(&index, &rawData); err != nil {
			return err
		}
		if err := fn(index, rawData); err != nil {
			return err
		}
	}
	return rows.Err()
}

func outputExecutionCondition(executed bool) postgres.BoolExpression {
	if executed {
		return table.Output.ExecutionTransactionHash.IS_NOT_NULL()
	}
	return table.Output.ExecutionTransactionHash.IS_NULL()
}

func outputTypesCondition(selectors [][]byte) postgres.BoolExpression {
	values := make([]postgres.Expression, 0, len(selectors))
	for _, selector := range selectors {
		values = append(values, ByteaLiteral(selector))
	}
	return SubstrBytea(table.Output.RawData, 1, 4).IN(values...)
}

// outputVoucherTypesCondition uses literals so PostgreSQL can prove
// that the condition implies output_pending_voucher_idx's predicate even when
// pgx executes the query with a generic prepared plan.
func outputVoucherTypesCondition() postgres.BoolExpression {
	return outputTypesCondition(
		[][]byte{
			voucherSelector,
			delegateCallVoucherSelector,
		},
	)
}

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
) (err error) {

	defer func() {
		var failure *pgconn.PgError
		if errors.As(err, &failure) && failure.Code == "55P03" {
			err = fmt.Errorf("%w: %w", repository.ErrLockTimeout, err)
		}
	}()

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := setLocalLockTimeout(ctx, tx); err != nil {
		return fmt.Errorf("setting output execution lock timeout: %w", err)
	}
	orderedOutputs := slices.Clone(outputs)
	slices.SortFunc(orderedOutputs, func(a, b *model.Output) int {
		return cmp.Compare(a.Index, b.Index)
	})
	for _, o := range orderedOutputs {
		if o.ExecutionTransactionHash == nil {
			return fmt.Errorf(
				"output ExecutionTransactionHash must be not nil when updating app %s output %d",
				nameOrAddress, o.Index,
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
			return err
		}
		if cmd.RowsAffected() != 1 {
			return fmt.Errorf("no row affected when updating app %s output %d", nameOrAddress, o.Index)
		}
	}

	// Advance the output scan cursor without moving it backwards.
	appUpdateStmt := table.Application.
		UPDATE(
			table.Application.LastOutputCheckBlock,
		).
		SET(
			uint64Expr(lastOutputCheckBlock),
		).
		WHERE(
			whereClause.AND(table.Application.LastOutputCheckBlock.LT(uint64Expr(lastOutputCheckBlock))),
		)

	sqlStr, args := appUpdateStmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListOutputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.OutputFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Output, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.Output.INNER_JOIN(
		table.Application,
		table.Output.InputEpochApplicationID.EQ(table.Application.ID),
	).INNER_JOIN(
		table.Input,
		table.Output.InputIndex.EQ(table.Input.Index).
			AND(table.Output.InputEpochApplicationID.EQ(table.Input.EpochApplicationID)),
	)

	conditions := []postgres.BoolExpression{whereClause}
	if f.IndexRange != nil {
		conditions = append(conditions,
			table.Output.Index.GT_EQ(uint64Expr(f.IndexRange.Start)),
			table.Output.Index.LT_EQ(uint64Expr(f.IndexRange.End)),
		)
	}
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

	if f.OutputType != nil && len(*f.OutputType) > 0 {
		conditions = append(conditions, outputTypesCondition(*f.OutputType))
	}

	if f.Executed != nil {
		conditions = append(conditions, outputExecutionCondition(*f.Executed))
	}

	if f.VoucherAddress != nil {
		// A destination-address filter is only meaningful for output methods
		// that carry a destination (Voucher, DelegateCallVoucher); bytes 17..36
		// of other outputs are arbitrary payload. The selector IN list, with
		// inline literals, is also what lets the planner prove the partial
		// predicate of output_raw_data_address_idx.
		conditions = append(conditions,
			outputVoucherTypesCondition(),
			SubstrBytea(table.Output.RawData, 17, 20).EQ(postgres.Bytea(f.VoucherAddress.Bytes())),
		)
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.Output.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
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
			table.Input.EpochIndex,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

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
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
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
			&out.EpochIndex,
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

func (r *PostgresRepository) GetNumberOfPendingExecutableOutputs(
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
		WHERE(
			whereClause.
				AND(outputExecutionCondition(false)).
				AND(outputVoucherTypesCondition()),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var count uint64
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
