// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

	sel := table.Input.
		SELECT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.MachineHash,
			table.Input.OutputsHash,
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
				AND(table.Input.Index.EQ(uint64Expr(inputIndex))),
		)

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
		&inp.MachineHash,
		&inp.OutputsHash,
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
			table.Input.MachineHash,
			table.Input.OutputsHash,
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
		&inp.MachineHash,
		&inp.OutputsHash,
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
			table.Input.MachineHash,
			table.Input.OutputsHash,
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
		&inp.MachineHash,
		&inp.OutputsHash,
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
			SubstrBytea(table.Input.RawData, 81, 20).EQ(postgres.Bytea(f.Sender.Bytes())),
		)
	}
	if f.TransactionHash != nil {
		conditions = append(conditions,
			table.Input.TransactionHash.EQ(postgres.Bytea(f.TransactionHash.Bytes())),
		)
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
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.Input.
		SELECT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.MachineHash,
			table.Input.OutputsHash,
			table.Input.TransactionHash,
			table.Input.LogIndex,
			table.Input.SnapshotURI,
			table.Input.CreatedAt,
			table.Input.UpdatedAt,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

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
		err := rows.Scan(
			&in.EpochApplicationID,
			&in.EpochIndex,
			&in.Index,
			&in.BlockNumber,
			&in.RawData,
			&in.Status,
			&in.MachineHash,
			&in.OutputsHash,
			&in.TransactionHash,
			&in.LogIndex,
			&in.SnapshotURI,
			&in.CreatedAt,
			&in.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		inputs = append(inputs, &in)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return inputs, total, nil
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
