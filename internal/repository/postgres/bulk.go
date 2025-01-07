// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func encodeSiblings(outputHashesSiblings []common.Hash) ([]byte, error) {
	// 1) Make a slice of []byte
	arr := make([][]byte, 0, len(outputHashesSiblings))
	for _, h := range outputHashesSiblings {
		// h is [32]byte
		// we must copy it into a slice of bytes
		copyH := make([]byte, len(h))
		copy(copyH, h[:])
		arr = append(arr, copyH)
	}

	// 2) Use pgtype.ByteaArray and call Set with [][]byte
	var siblings pgtype.ByteaArray
	if err := siblings.Set(arr); err != nil {
		return nil, fmt.Errorf("failed to set ByteaArray: %w", err)
	}

	// 3) Encode it as text (the Postgres array string, e.g. '{\\x...,\\x..., ...}')
	encoded, err := siblings.EncodeText(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ByteaArray: %w", err)
	}

	return encoded, nil
}

func getOutputNextIndex(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
) (uint64, error) {

	query := table.Output.SELECT(
		postgres.COALESCE(
			postgres.Float(1).ADD(postgres.MAXf(table.Output.Index)),
			postgres.Float(0),
		),
	).FROM(
		table.Output.INNER_JOIN(table.Input, table.Input.EpochApplicationID.EQ(table.Output.InputEpochApplicationID).
			AND(table.Input.Index.EQ(table.Output.InputIndex))),
	).WHERE(
		table.Output.InputEpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		err = fmt.Errorf("failed to get the next output index: %w", err)
		return 0, errors.Join(err, tx.Rollback(ctx))
	}
	return currentIndex, nil
}

func getReportNextIndex(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
) (uint64, error) {

	query := table.Report.SELECT(
		postgres.COALESCE(
			postgres.Float(1).ADD(postgres.MAXf(table.Report.Index)),
			postgres.Float(0),
		),
	).FROM(
		table.Report.INNER_JOIN(table.Input, table.Input.EpochApplicationID.EQ(table.Report.InputEpochApplicationID)),
	).WHERE(
		table.Report.InputEpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		err = fmt.Errorf("failed to get the next report index: %w", err)
		return 0, errors.Join(err, tx.Rollback(ctx))
	}
	return currentIndex, nil
}

func insertOutputs(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	dataArray [][]byte,
) error {
	if len(dataArray) < 1 {
		return nil
	}

	nextIndex, err := getOutputNextIndex(ctx, tx, appID)
	if err != nil {
		return err
	}

	stmt := table.Output.INSERT(
		table.Output.InputEpochApplicationID,
		table.Output.InputIndex,
		table.Output.Index,
		table.Output.RawData,
	)
	for i, data := range dataArray {
		stmt = stmt.VALUES(
			appID,
			inputIndex,
			nextIndex+uint64(i),
			data,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return nil
}

func insertReports(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	dataArray [][]byte,
) error {
	if len(dataArray) < 1 {
		return nil
	}

	nextIndex, err := getReportNextIndex(ctx, tx, appID)
	if err != nil {
		return err
	}

	stmt := table.Report.INSERT(
		table.Report.InputEpochApplicationID,
		table.Report.InputIndex,
		table.Report.Index,
		table.Report.RawData,
	)
	for i, data := range dataArray {
		stmt = stmt.VALUES(
			appID,
			inputIndex,
			nextIndex+uint64(i),
			data,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return nil
}

func updateInput(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	status model.InputCompletionStatus,
	outputsHash common.Hash,
	machineHash common.Hash,
) error {

	updStmt := table.Input.
		UPDATE(
			table.Input.Status,
			table.Input.MachineHash,
			table.Input.OutputsHash,
		).
		SET(
			status,
			machineHash,
			outputsHash,
		).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Input.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", inputIndex)))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func updateApp(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
) error {

	updStmt := table.Application.
		UPDATE(
			table.Application.ProcessedInputs,
		).
		SET(
			postgres.RawFloat(fmt.Sprintf("%d", inputIndex+1)),
		).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *postgresRepository) StoreAdvanceResult(
	ctx context.Context,
	appID int64,
	res *model.AdvanceResult,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}

	err = insertOutputs(ctx, tx, appID, res.InputIndex, res.Outputs)
	if err != nil {
		return err
	}

	err = insertReports(ctx, tx, appID, res.InputIndex, res.Reports)
	if err != nil {
		return err
	}

	err = updateInput(ctx, tx, appID, res.InputIndex, res.Status, res.OutputsHash, *res.MachineHash)
	if err != nil {
		return err
	}

	err = updateApp(ctx, tx, appID, res.InputIndex)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	return nil
}

func updateEpochClaim(
	ctx context.Context,
	tx pgx.Tx,
	e *model.Epoch,
) error {

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.ClaimHash,
			table.Epoch.Status,
		).
		SET(
			e.ClaimHash,
			postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()),
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(e.ApplicationID)).
				AND(table.Epoch.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", e.Index)))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(
			fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err),
			tx.Rollback(ctx),
		)
	}
	if cmd.RowsAffected() != 1 {
		return errors.Join(
			fmt.Errorf("failed to update application %d epoch %d: no rows affected", e.ApplicationID, e.Index),
			tx.Rollback(ctx),
		)
	}
	return nil
}

func updateOutputs(
	ctx context.Context,
	tx pgx.Tx,
	outputs []*model.Output,
) error {
	for _, output := range outputs {
		siblings, err := encodeSiblings(output.OutputHashesSiblings)
		if err != nil {
			return errors.Join(
				fmt.Errorf("failed to serialize outputHashesSiblings for output '%d'. %w", output.Index, err),
				tx.Rollback(ctx),
			)
		}

		updStmt := table.Output.
			UPDATE(
				table.Output.Hash,
				table.Output.OutputHashesSiblings,
			).
			SET(
				output.Hash,
				siblings,
			).
			WHERE(
				table.Output.InputEpochApplicationID.EQ(postgres.Int64(output.InputEpochApplicationID)).
					AND(table.Output.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", output.Index)))),
			)

		sqlStr, args := updStmt.Sql()
		cmd, err := tx.Exec(ctx, sqlStr, args...)
		if err != nil {
			return errors.Join(
				fmt.Errorf("failed to insert proof for output '%d'. %w", output.Index, err),
				tx.Rollback(ctx),
			)
		}
		if cmd.RowsAffected() == 0 {
			return errors.Join(
				fmt.Errorf(
					"failed to insert proof for output '%d'. No rows affected",
					output.Index,
				),
				tx.Rollback(ctx),
			)
		}
	}
	return nil
}

func (r *postgresRepository) StoreClaimAndProofs(ctx context.Context, epoch *model.Epoch, outputs []*model.Output) error {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err)
	}

	err = updateEpochClaim(ctx, tx, epoch)
	if err != nil {
		return err
	}

	err = updateOutputs(ctx, tx, outputs)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(
			fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err),
			tx.Rollback(ctx),
		)
	}
	return nil
}
