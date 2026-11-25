// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
)

func (r *PostgresRepository) GetInput(
	ctx context.Context,
	nameOrAddress string,
	inputIndex uint64,
) (*model.Input, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	attributed := table.Input.
		SELECT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.ExceptionData,
			table.Input.MachineHash,
			table.Input.TxBufferDataBlock,
			table.Input.TransactionHash,
			table.Input.LogIndex,
			table.Input.SnapshotURI,
			table.Input.CreatedAt,
			table.Input.UpdatedAt,
			postgres.Bool(false),
		).
		FROM(
			table.Input.
				INNER_JOIN(table.Application,
					table.Input.EpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Input.Index.EQ(uint64Expr(inputIndex))),
		)

	observed := table.ObservedInput.
		SELECT(
			table.Application.ID,
			uint64Expr(0),
			table.ObservedInput.Index,
			table.ObservedInput.BlockNumber,
			table.ObservedInput.RawData,
			postgres.NewEnumValue(model.InputCompletionStatus_None.String()),
			postgres.NULL,
			postgres.NULL,
			postgres.NULL,
			table.ObservedInput.TransactionHash,
			table.ObservedInput.LogIndex,
			postgres.NULL,
			postgres.NULL,
			postgres.NULL,
			postgres.Bool(true),
		).
		FROM(
			table.ObservedInput.
				INNER_JOIN(table.Application,
					table.ObservedInput.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.ObservedInput.Index.EQ(uint64Expr(inputIndex))),
		)

	sqlStr, args := postgres.UNION_ALL(attributed, observed).LIMIT(1).Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var inp model.Input
	var createdAt, updatedAt *time.Time
	err := row.Scan(
		&inp.EpochApplicationID,
		&inp.EpochIndex,
		&inp.Index,
		&inp.BlockNumber,
		&inp.RawData,
		&inp.Status,
		&inp.ExceptionData,
		&inp.MachineHash,
		&inp.TxBufferDataBlock,
		&inp.TransactionHash,
		&inp.LogIndex,
		&inp.SnapshotURI,
		&createdAt,
		&updatedAt,
		&inp.Unattributed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	setInputTimestamps(&inp, createdAt, updatedAt)
	return &inp, nil
}

func (r *PostgresRepository) GetLastInput(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
) (*model.Input, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Input.
		SELECT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.ExceptionData,
			table.Input.MachineHash,
			table.Input.TxBufferDataBlock,
			table.Input.TransactionHash,
			table.Input.LogIndex,
			table.Input.SnapshotURI,
			table.Input.CreatedAt,
			table.Input.UpdatedAt,
		).
		FROM(
			table.Input.
				INNER_JOIN(table.Application,
					table.Input.EpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Input.EpochIndex.EQ(uint64Expr(epochIndex))),
		).
		ORDER_BY(table.Input.Index.DESC()).
		LIMIT(1)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var inp model.Input
	err := row.Scan(
		&inp.EpochApplicationID,
		&inp.EpochIndex,
		&inp.Index,
		&inp.BlockNumber,
		&inp.RawData,
		&inp.Status,
		&inp.ExceptionData,
		&inp.MachineHash,
		&inp.TxBufferDataBlock,
		&inp.TransactionHash,
		&inp.LogIndex,
		&inp.SnapshotURI,
		&inp.CreatedAt,
		&inp.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inp, nil
}

func (r *PostgresRepository) GetLastProcessedInput(
	ctx context.Context,
	nameOrAddress string,
) (*model.Input, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Input.
		SELECT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.ExceptionData,
			table.Input.MachineHash,
			table.Input.TxBufferDataBlock,
			table.Input.TransactionHash,
			table.Input.LogIndex,
			table.Input.SnapshotURI,
			table.Input.CreatedAt,
			table.Input.UpdatedAt,
		).
		FROM(
			table.Input.
				INNER_JOIN(table.Application,
					table.Input.EpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Input.Status.NOT_EQ(postgres.NewEnumValue(model.InputCompletionStatus_None.String()))),
		).
		ORDER_BY(table.Input.Index.DESC()).
		LIMIT(1)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var inp model.Input
	err := row.Scan(
		&inp.EpochApplicationID,
		&inp.EpochIndex,
		&inp.Index,
		&inp.BlockNumber,
		&inp.RawData,
		&inp.Status,
		&inp.ExceptionData,
		&inp.MachineHash,
		&inp.TxBufferDataBlock,
		&inp.TransactionHash,
		&inp.LogIndex,
		&inp.SnapshotURI,
		&inp.CreatedAt,
		&inp.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inp, nil
}

const (
	inputSenderOffset = 81
	inputSenderLength = 20
)

func (r *PostgresRepository) ListInputs(
	ctx context.Context,
	nameOrAddress string,
	f repository.InputFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Input, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.Input.
		INNER_JOIN(table.Application,
			table.Input.EpochApplicationID.EQ(table.Application.ID),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.IndexRange != nil {
		conditions = append(conditions,
			table.Input.Index.GT_EQ(uint64Expr(f.IndexRange.Start)),
			table.Input.Index.LT_EQ(uint64Expr(f.IndexRange.End)),
		)
	}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.Input.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
	}
	if f.Status != nil {
		conditions = append(conditions, table.Input.Status.EQ(postgres.NewEnumValue(f.Status.String())))
	}
	if f.NotStatus != nil {
		conditions = append(conditions, table.Input.Status.NOT_EQ(postgres.NewEnumValue(f.NotStatus.String())))
	}
	if f.Sender != nil {
		conditions = append(conditions,
			SubstrBytea(table.Input.RawData, inputSenderOffset, inputSenderLength).EQ(postgres.Bytea(f.Sender.Bytes())),
		)
	}
	if f.TransactionHash != nil {
		conditions = append(conditions,
			table.Input.TransactionHash.EQ(postgres.Bytea(f.TransactionHash.Bytes())),
		)
	}

	observedFromClause := table.ObservedInput.
		INNER_JOIN(table.Application,
			table.ObservedInput.ApplicationID.EQ(table.Application.ID),
		)
	observedConditions, includeObserved := observedInputConditions(whereClause, f)
	if !includeObserved {
		observedConditions = []postgres.BoolExpression{postgres.Bool(false)}
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.Input.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if includeObserved {
		observedCountStmt := table.ObservedInput.SELECT(postgres.COUNT(postgres.STAR)).
			FROM(observedFromClause).WHERE(postgres.AND(observedConditions...))
		observedTotal, err := countFromTx(ctx, tx, observedCountStmt)
		if err != nil {
			return nil, 0, err
		}
		total += observedTotal
	}
	if total == 0 {
		return nil, 0, nil
	}

	attributedSel := table.Input.
		SELECT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.ExceptionData,
			table.Input.MachineHash,
			table.Input.TxBufferDataBlock,
			table.Input.TransactionHash,
			table.Input.LogIndex,
			table.Input.SnapshotURI,
			table.Input.CreatedAt,
			table.Input.UpdatedAt,
			postgres.Bool(false),
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	observedSel := table.ObservedInput.
		SELECT(
			table.Application.ID,
			uint64Expr(0),
			table.ObservedInput.Index,
			table.ObservedInput.BlockNumber,
			table.ObservedInput.RawData,
			postgres.NewEnumValue(model.InputCompletionStatus_None.String()),
			postgres.NULL,
			postgres.NULL,
			postgres.NULL,
			table.ObservedInput.TransactionHash,
			table.ObservedInput.LogIndex,
			postgres.NULL,
			postgres.NULL,
			postgres.NULL,
			postgres.Bool(true),
		).
		FROM(observedFromClause).
		WHERE(postgres.AND(observedConditions...))

	sel := postgres.UNION_ALL(attributedSel, observedSel)
	if descending {
		sel = sel.ORDER_BY(table.Input.Index.DESC())
	} else {
		sel = sel.ORDER_BY(table.Input.Index.ASC())
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

	var inputs []*model.Input
	for rows.Next() {
		var in model.Input
		var createdAt, updatedAt *time.Time
		err := rows.Scan(
			&in.EpochApplicationID,
			&in.EpochIndex,
			&in.Index,
			&in.BlockNumber,
			&in.RawData,
			&in.Status,
			&in.ExceptionData,
			&in.MachineHash,
			&in.TxBufferDataBlock,
			&in.TransactionHash,
			&in.LogIndex,
			&in.SnapshotURI,
			&createdAt,
			&updatedAt,
			&in.Unattributed,
		)
		if err != nil {
			return nil, 0, err
		}
		setInputTimestamps(&in, createdAt, updatedAt)
		inputs = append(inputs, &in)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return inputs, total, nil
}

// observedInputConditions mirrors the attributed-input filters for the
// observed-but-unattributed rows. An epoch filter cannot match these rows, and
// their status is always NONE until they are attributed.
func observedInputConditions(
	whereClause postgres.BoolExpression,
	f repository.InputFilter,
) ([]postgres.BoolExpression, bool) {
	if f.EpochIndex != nil {
		return nil, false
	}
	if f.Status != nil && *f.Status != model.InputCompletionStatus_None {
		return nil, false
	}
	if f.NotStatus != nil && *f.NotStatus == model.InputCompletionStatus_None {
		return nil, false
	}

	conditions := []postgres.BoolExpression{whereClause}
	if f.IndexRange != nil {
		conditions = append(conditions,
			table.ObservedInput.Index.GT_EQ(uint64Expr(f.IndexRange.Start)),
			table.ObservedInput.Index.LT_EQ(uint64Expr(f.IndexRange.End)),
		)
	}
	if f.Sender != nil {
		conditions = append(conditions,
			SubstrBytea(table.ObservedInput.RawData, inputSenderOffset, inputSenderLength).EQ(postgres.Bytea(f.Sender.Bytes())),
		)
	}
	if f.TransactionHash != nil {
		conditions = append(conditions,
			table.ObservedInput.TransactionHash.EQ(postgres.Bytea(f.TransactionHash.Bytes())),
		)
	}
	return conditions, true
}

func setInputTimestamps(input *model.Input, createdAt, updatedAt *time.Time) {
	if createdAt != nil {
		input.CreatedAt = *createdAt
	}
	if updatedAt != nil {
		input.UpdatedAt = *updatedAt
	}
}

func (r *PostgresRepository) GetNumberOfInputs(
	ctx context.Context,
	nameOrAddress string,
) (uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Input.
		SELECT(postgres.COUNT(postgres.STAR)).
		FROM(
			table.Input.
				INNER_JOIN(table.Application,
					table.Input.EpochApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(whereClause)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var count uint64
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) UpdateInputSnapshotURI(ctx context.Context, appId int64, inputIndex uint64, snapshotURI string) error {
	updStmt := table.Input.
		UPDATE(
			table.Input.SnapshotURI,
		).
		SET(
			snapshotURI,
		).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appId)).
				AND(table.Input.Index.EQ(uint64Expr(inputIndex))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("no input found with appId %d and index %d", appId, inputIndex)
	}
	return nil
}
