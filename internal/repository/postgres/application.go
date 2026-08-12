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

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

// ------------------------ ApplicationRepository Methods ------------------------ //

func (r *PostgresRepository) CreateApplication(
	ctx context.Context,
	app *model.Application,
	withExecutionParameters bool,
) (int64, error) {

	insertStmt := table.Application.
		INSERT(
			table.Application.Name,
			table.Application.IapplicationAddress,
			table.Application.IconsensusAddress,
			table.Application.IinputboxAddress,
			table.Application.TemplateHash,
			table.Application.TemplateURI,
			table.Application.EpochLength,
			table.Application.ClaimStagingPeriod,
			table.Application.WithdrawalGuardian,
			table.Application.WithdrawalLog2LeavesPerAccount,
			table.Application.WithdrawalLog2MaxNumOfAccounts,
			table.Application.WithdrawalAccountsDriveStartIndex,
			table.Application.WithdrawalOutputBuilder,
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.Enabled,
			table.Application.Status,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.LastForecloseCheckBlock,
			table.Application.LastAccountsDriveProvedCheckBlock,
			table.Application.LastWithdrawalCheckBlock,
			table.Application.ProcessedInputs,
			table.Application.ForecloseBlock,
			table.Application.ForecloseTransaction,
			table.Application.AccountsDriveProvedBlock,
			table.Application.AccountsDriveProvedTransaction,
			table.Application.AccountsDriveMerkleRoot,
		).
		VALUES(
			app.Name,
			app.IApplicationAddress,
			app.IConsensusAddress,
			app.IInputBoxAddress,
			app.TemplateHash,
			app.TemplateURI,
			app.EpochLength,
			app.ClaimStagingPeriod,
			app.WithdrawalConfig.Guardian,
			app.WithdrawalConfig.Log2LeavesPerAccount,
			app.WithdrawalConfig.Log2MaxNumOfAccounts,
			app.WithdrawalConfig.AccountsDriveStartIndex,
			app.WithdrawalConfig.WithdrawalOutputBuilder,
			app.DataAvailability,
			app.ConsensusType,
			app.Enabled,
			app.Status,
			app.IInputBoxBlock,
			app.LastEpochCheckBlock,
			app.LastInputCheckBlock,
			app.LastOutputCheckBlock,
			app.LastTournamentCheckBlock,
			app.LastForecloseCheckBlock,
			app.LastAccountsDriveProvedCheckBlock,
			app.LastWithdrawalCheckBlock,
			app.ProcessedInputs,
			app.ForecloseBlock,
			app.ForecloseTransaction,
			app.AccountsDriveProvedBlock,
			app.AccountsDriveProvedTransaction,
			app.AccountsDriveMerkleRoot,
		).
		RETURNING(table.Application.ID)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	sqlStr, args := insertStmt.Sql()
	var newID int64
	err = tx.QueryRow(ctx, sqlStr, args...).Scan(&newID)
	if err != nil {
		return 0, fmt.Errorf("unable to create database application: %w", err)
	}

	if !withExecutionParameters {
		sqlStr, args = table.ExecutionParameters.
			INSERT(
				table.ExecutionParameters.ApplicationID,
			).
			VALUES(
				newID,
			).Sql()
	} else {
		sqlStr, args = table.ExecutionParameters.
			INSERT(
				table.ExecutionParameters.ApplicationID,
				table.ExecutionParameters.SnapshotPolicy,
				table.ExecutionParameters.AdvanceIncCycles,
				table.ExecutionParameters.AdvanceMaxCycles,
				table.ExecutionParameters.InspectIncCycles,
				table.ExecutionParameters.InspectMaxCycles,
				table.ExecutionParameters.AdvanceIncDeadline,
				table.ExecutionParameters.AdvanceMaxDeadline,
				table.ExecutionParameters.InspectIncDeadline,
				table.ExecutionParameters.InspectMaxDeadline,
				table.ExecutionParameters.LoadDeadline,
				table.ExecutionParameters.StoreDeadline,
				table.ExecutionParameters.FastDeadline,
				table.ExecutionParameters.MaxConcurrentInspects,
			).
			VALUES(
				newID,
				app.ExecutionParameters.SnapshotPolicy,
				app.ExecutionParameters.AdvanceIncCycles,
				app.ExecutionParameters.AdvanceMaxCycles,
				app.ExecutionParameters.InspectIncCycles,
				app.ExecutionParameters.InspectMaxCycles,
				app.ExecutionParameters.AdvanceIncDeadline,
				app.ExecutionParameters.AdvanceMaxDeadline,
				app.ExecutionParameters.InspectIncDeadline,
				app.ExecutionParameters.InspectMaxDeadline,
				app.ExecutionParameters.LoadDeadline,
				app.ExecutionParameters.StoreDeadline,
				app.ExecutionParameters.FastDeadline,
				app.ExecutionParameters.MaxConcurrentInspects,
			).Sql()
	}

	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return 0, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, err
	}
	return newID, nil
}

// GetApplication retrieves one application by ID, optionally loading status & execution parameters.
func (r *PostgresRepository) GetApplication(
	ctx context.Context,
	nameOrAddress string,
) (*model.Application, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	stmt := table.Application.
		SELECT(
			table.Application.ID,
			table.Application.Name,
			table.Application.IapplicationAddress,
			table.Application.IconsensusAddress,
			table.Application.IinputboxAddress,
			table.Application.TemplateHash,
			table.Application.TemplateURI,
			table.Application.EpochLength,
			table.Application.ClaimStagingPeriod,
			table.Application.WithdrawalGuardian,
			table.Application.WithdrawalLog2LeavesPerAccount,
			table.Application.WithdrawalLog2MaxNumOfAccounts,
			table.Application.WithdrawalAccountsDriveStartIndex,
			table.Application.WithdrawalOutputBuilder,
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.Enabled,
			table.Application.Status,
			table.Application.Reason,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.LastForecloseCheckBlock,
			table.Application.LastAccountsDriveProvedCheckBlock,
			table.Application.LastWithdrawalCheckBlock,
			table.Application.ProcessedInputs,
			table.Application.ForecloseBlock,
			table.Application.ForecloseTransaction,
			table.Application.AccountsDriveProvedBlock,
			table.Application.AccountsDriveProvedTransaction,
			table.Application.AccountsDriveMerkleRoot,
			table.Application.CreatedAt,
			table.Application.UpdatedAt,
			table.ExecutionParameters.ApplicationID,
			table.ExecutionParameters.SnapshotPolicy,
			table.ExecutionParameters.AdvanceIncCycles,
			table.ExecutionParameters.AdvanceMaxCycles,
			table.ExecutionParameters.InspectIncCycles,
			table.ExecutionParameters.InspectMaxCycles,
			table.ExecutionParameters.AdvanceIncDeadline,
			table.ExecutionParameters.AdvanceMaxDeadline,
			table.ExecutionParameters.InspectIncDeadline,
			table.ExecutionParameters.InspectMaxDeadline,
			table.ExecutionParameters.LoadDeadline,
			table.ExecutionParameters.StoreDeadline,
			table.ExecutionParameters.FastDeadline,
			table.ExecutionParameters.MaxConcurrentInspects,
			table.ExecutionParameters.CreatedAt,
			table.ExecutionParameters.UpdatedAt,
		).
		FROM(
			table.Application.INNER_JOIN(
				table.ExecutionParameters,
				table.ExecutionParameters.ApplicationID.EQ(table.Application.ID),
			),
		).
		WHERE(whereClause)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var app model.Application
	err := row.Scan(
		&app.ID,
		&app.Name,
		&app.IApplicationAddress,
		&app.IConsensusAddress,
		&app.IInputBoxAddress,
		&app.TemplateHash,
		&app.TemplateURI,
		&app.EpochLength,
		&app.ClaimStagingPeriod,
		&app.WithdrawalConfig.Guardian,
		&app.WithdrawalConfig.Log2LeavesPerAccount,
		&app.WithdrawalConfig.Log2MaxNumOfAccounts,
		&app.WithdrawalConfig.AccountsDriveStartIndex,
		&app.WithdrawalConfig.WithdrawalOutputBuilder,
		&app.DataAvailability,
		&app.ConsensusType,
		&app.Enabled,
		&app.Status,
		&app.Reason,
		&app.IInputBoxBlock,
		&app.LastEpochCheckBlock,
		&app.LastInputCheckBlock,
		&app.LastOutputCheckBlock,
		&app.LastTournamentCheckBlock,
		&app.LastForecloseCheckBlock,
		&app.LastAccountsDriveProvedCheckBlock,
		&app.LastWithdrawalCheckBlock,
		&app.ProcessedInputs,
		&app.ForecloseBlock,
		&app.ForecloseTransaction,
		&app.AccountsDriveProvedBlock,
		&app.AccountsDriveProvedTransaction,
		&app.AccountsDriveMerkleRoot,
		&app.CreatedAt,
		&app.UpdatedAt,
		&app.ExecutionParameters.ApplicationID,
		&app.ExecutionParameters.SnapshotPolicy,
		&app.ExecutionParameters.AdvanceIncCycles,
		&app.ExecutionParameters.AdvanceMaxCycles,
		&app.ExecutionParameters.InspectIncCycles,
		&app.ExecutionParameters.InspectMaxCycles,
		&app.ExecutionParameters.AdvanceIncDeadline,
		&app.ExecutionParameters.AdvanceMaxDeadline,
		&app.ExecutionParameters.InspectIncDeadline,
		&app.ExecutionParameters.InspectMaxDeadline,
		&app.ExecutionParameters.LoadDeadline,
		&app.ExecutionParameters.StoreDeadline,
		&app.ExecutionParameters.FastDeadline,
		&app.ExecutionParameters.MaxConcurrentInspects,
		&app.ExecutionParameters.CreatedAt,
		&app.ExecutionParameters.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // not found
	}
	if err != nil {
		return nil, err
	}

	return &app, nil
}

// GetProcessedInputCount retrieves the ProcessedInputs field from an application by Name or address.
func (r *PostgresRepository) GetProcessedInputCount(
	ctx context.Context,
	nameOrAddress string,
) (uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	stmt := table.Application.
		SELECT(table.Application.ProcessedInputs).
		WHERE(whereClause)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var processedInputs uint64
	err := row.Scan(&processedInputs)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrNotFound
	}
	return processedInputs, err
}

// UpdateApplication updates application configuration fields.
//
// Status, operator intent, scanner cursors, processed input counters, and
// foreclosure columns are deliberately excluded. Dedicated methods own those
// fields so a stale in-memory application cannot rewind service progress or
// move the app back into normal work while changing unrelated configuration.
func (r *PostgresRepository) UpdateApplication(
	ctx context.Context,
	app *model.Application,
) error {

	updateStmt := table.Application.
		UPDATE(
			table.Application.Name,
			table.Application.IapplicationAddress,
			table.Application.IconsensusAddress,
			table.Application.IinputboxAddress,
			table.Application.TemplateHash,
			table.Application.TemplateURI,
			table.Application.EpochLength,
			table.Application.ClaimStagingPeriod,
			table.Application.WithdrawalGuardian,
			table.Application.WithdrawalLog2LeavesPerAccount,
			table.Application.WithdrawalLog2MaxNumOfAccounts,
			table.Application.WithdrawalAccountsDriveStartIndex,
			table.Application.WithdrawalOutputBuilder,
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.IinputboxBlock,
		).
		SET(
			app.Name,
			app.IApplicationAddress,
			app.IConsensusAddress,
			app.IInputBoxAddress,
			app.TemplateHash,
			app.TemplateURI,
			app.EpochLength,
			app.ClaimStagingPeriod,
			app.WithdrawalConfig.Guardian,
			app.WithdrawalConfig.Log2LeavesPerAccount,
			app.WithdrawalConfig.Log2MaxNumOfAccounts,
			app.WithdrawalConfig.AccountsDriveStartIndex,
			app.WithdrawalConfig.WithdrawalOutputBuilder,
			app.DataAvailability,
			app.ConsensusType,
			app.IInputBoxBlock,
		).
		WHERE(table.Application.ID.EQ(postgres.Int(app.ID)))

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// UpdateApplicationEnabled changes only the operator intent bit. It must not
// touch service-owned scanner cursors or status fields.
func (r *PostgresRepository) UpdateApplicationEnabled(
	ctx context.Context,
	appID int64,
	enabled bool,
) error {
	updateStmt := table.Application.
		UPDATE(table.Application.Enabled).
		SET(enabled).
		WHERE(table.Application.ID.EQ(postgres.Int(appID)))

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// EnableApplicationAndClearFailed re-enables an application and clears FAILED
// in one statement. DIVERGED/CORRUPTED statuses are left unchanged; enabling a
// foreclosed app records operator intent but does not make it executable.
func (r *PostgresRepository) EnableApplicationAndClearFailed(
	ctx context.Context,
	appID int64,
) error {
	updateStmt := table.Application.
		UPDATE(
			table.Application.Enabled,
			table.Application.Status,
			table.Application.Reason,
		).
		SET(
			true,
			postgres.CASE().
				WHEN(table.Application.Status.EQ(postgres.NewEnumValue(model.ApplicationStatus_Failed.String()))).
				THEN(postgres.NewEnumValue(model.ApplicationStatus_OK.String())).
				ELSE(table.Application.Status),
			postgres.CASE().
				WHEN(table.Application.Status.EQ(postgres.NewEnumValue(model.ApplicationStatus_Failed.String()))).
				THEN(postgres.NULL).
				ELSE(table.Application.Reason),
		).
		WHERE(table.Application.ID.EQ(postgres.Int(appID)))

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// UpdateApplicationLastForecloseCheckBlock advances the per-app record
// of how far the Foreclosure-event search has scanned. The clause
// `WHERE last_foreclose_check_block < blockNumber` makes the write
// strictly monotonic: out-of-order or duplicate observations from a slow
// tick cannot rewind the value and re-cause a long-window rescan. A no-op
// (0 rows affected) is not an error — it just means the caller's view is
// stale.
func (r *PostgresRepository) UpdateApplicationLastForecloseCheckBlock(
	ctx context.Context,
	appID int64,
	blockNumber uint64,
) error {
	updateStmt := table.Application.
		UPDATE(table.Application.LastForecloseCheckBlock).
		SET(uint64Expr(blockNumber)).
		WHERE(
			table.Application.ID.EQ(postgres.Int(appID)).
				AND(table.Application.LastForecloseCheckBlock.LT(uint64Expr(blockNumber))),
		)

	sqlStr, args := updateStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

// UpdateApplicationForeclosure records the one-shot Foreclosure() event and
// advances last_foreclose_check_block in the same
// transaction. If the marker was already recorded, this is an idempotent no-op.
func (r *PostgresRepository) UpdateApplicationForeclosure(
	ctx context.Context,
	appID int64,
	block uint64,
	txHash common.Hash,
	blockNumber uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	cmd, err := tx.Exec(ctx, `
UPDATE "application"
SET
    "foreclose_block" = $1,
    "foreclose_transaction" = $2
WHERE "id" = $3 AND "foreclose_block" = 0
`, block, txHash, appID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		probeStmt := table.Application.
			SELECT(table.Application.ID).
			WHERE(table.Application.ID.EQ(postgres.Int(appID)))
		psql, pargs := probeStmt.Sql()
		var dummy int64
		err = tx.QueryRow(ctx, psql, pargs...).Scan(&dummy)
		if errors.Is(err, sql.ErrNoRows) {
			return repository.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("probing application existence (id=%d): %w", appID, err)
		}
		return tx.Commit(ctx)
	}

	cursorStmt := table.Application.
		UPDATE(table.Application.LastForecloseCheckBlock).
		SET(uint64Expr(blockNumber)).
		WHERE(
			table.Application.ID.EQ(postgres.Int(appID)).
				AND(table.Application.LastForecloseCheckBlock.LT(uint64Expr(blockNumber))),
		)
	sqlStr, args := cursorStmt.Sql()
	if _, err := tx.Exec(ctx, sqlStr, args...); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateAccountsDriveProved records the one-shot drive-prove transition and
// advances the scanner cursor in the same
// transaction. If the marker was already recorded, this is an idempotent no-op.
func (r *PostgresRepository) UpdateAccountsDriveProved(
	ctx context.Context,
	appID int64,
	block uint64,
	txHash common.Hash,
	root common.Hash,
	blockNumber uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	updateStmt := table.Application.
		UPDATE(
			table.Application.AccountsDriveProvedBlock,
			table.Application.AccountsDriveProvedTransaction,
			table.Application.AccountsDriveMerkleRoot,
		).
		SET(
			block,
			&txHash,
			&root,
		).
		WHERE(
			table.Application.ID.EQ(postgres.Int(appID)).
				AND(table.Application.AccountsDriveProvedBlock.EQ(uint64Expr(0))),
		)

	sqlStr, args := updateStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		probeStmt := table.Application.
			SELECT(table.Application.ID).
			WHERE(table.Application.ID.EQ(postgres.Int(appID)))
		psql, pargs := probeStmt.Sql()
		var dummy int64
		err = tx.QueryRow(ctx, psql, pargs...).Scan(&dummy)
		if errors.Is(err, sql.ErrNoRows) {
			return repository.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("probing application existence (id=%d): %w", appID, err)
		}
		return tx.Commit(ctx)
	}

	cursorStmt := table.Application.
		UPDATE(table.Application.LastAccountsDriveProvedCheckBlock).
		SET(uint64Expr(blockNumber)).
		WHERE(
			table.Application.ID.EQ(postgres.Int(appID)).
				AND(table.Application.LastAccountsDriveProvedCheckBlock.LT(uint64Expr(blockNumber))),
		)
	sqlStr, args = cursorStmt.Sql()
	if _, err := tx.Exec(ctx, sqlStr, args...); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateApplicationLastAccountsDriveProvedCheckBlock advances the per-app
// scanner cursor for the getAccountsDriveMerkleRoot().wasProved observer.
// Strictly monotonic — mirrors UpdateApplicationLastForecloseCheckBlock.
func (r *PostgresRepository) UpdateApplicationLastAccountsDriveProvedCheckBlock(
	ctx context.Context,
	appID int64,
	blockNumber uint64,
) error {
	updateStmt := table.Application.
		UPDATE(table.Application.LastAccountsDriveProvedCheckBlock).
		SET(uint64Expr(blockNumber)).
		WHERE(
			table.Application.ID.EQ(postgres.Int(appID)).
				AND(table.Application.LastAccountsDriveProvedCheckBlock.LT(uint64Expr(blockNumber))),
		)

	sqlStr, args := updateStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) UpdateApplicationStatus(
	ctx context.Context,
	appID int64,
	status model.ApplicationStatus,
	reason *string,
) error {

	updateStmt := table.Application.
		UPDATE(
			table.Application.Status,
			table.Application.Reason,
		).
		SET(
			status,
			reason,
		).
		WHERE(table.Application.ID.EQ(postgres.Int(appID)))

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func getColumnForEvent(event model.MonitoredEvent) (postgres.ColumnFloat, error) {
	switch event {
	case model.MonitoredEvent_EpochSealed:
		return table.Application.LastEpochCheckBlock, nil
	case model.MonitoredEvent_InputAdded:
		return table.Application.LastInputCheckBlock, nil
	case model.MonitoredEvent_OutputExecuted:
		return table.Application.LastOutputCheckBlock, nil
	case model.MonitoredEvent_CommitmentJoined:
		fallthrough
	case model.MonitoredEvent_MatchAdvanced:
		fallthrough
	case model.MonitoredEvent_MatchCreated:
		fallthrough
	case model.MonitoredEvent_MatchDeleted:
		fallthrough
	case model.MonitoredEvent_NewInnerTournament:
		return table.Application.LastTournamentCheckBlock, nil
	case model.MonitoredEvent_ClaimSubmitted:
		fallthrough
	case model.MonitoredEvent_ClaimAccepted:
		fallthrough
	default:
		return nil, fmt.Errorf("invalid monitored event type: %v", event)
	}
}

func (r *PostgresRepository) GetEventLastCheckBlock(
	ctx context.Context,
	appID int64,
	event model.MonitoredEvent,
) (uint64, error) {
	column, err := getColumnForEvent(event)
	if err != nil {
		return 0, err
	}

	stmt := table.Application.SELECT(column).WHERE(
		table.Application.ID.EQ(postgres.Int(appID)),
	)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var eventBlock uint64
	err = row.Scan(&eventBlock)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrNotFound
	}
	return eventBlock, err
}

func (r *PostgresRepository) UpdateEventLastCheckBlock(
	ctx context.Context,
	appIDs []int64,
	event model.MonitoredEvent,
	blockNumber uint64,
) error {
	column, err := getColumnForEvent(event)
	if err != nil {
		return err
	}

	if len(appIDs) == 0 {
		return nil
	}

	ids := make([]postgres.Expression, len(appIDs))
	for i, id := range appIDs {
		ids[i] = postgres.Int(id)
	}

	updateStmt := table.Application.
		UPDATE(
			column,
		).
		SET(
			uint64Expr(blockNumber),
		).
		WHERE(table.Application.ID.IN(ids...))

	sqlStr, args := updateStmt.Sql()
	_, err = r.db.Exec(ctx, sqlStr, args...)
	return err
}

// GetLastSnapshot retrieves the most recent input with a snapshot for the given application
func (r *PostgresRepository) GetLastSnapshot(ctx context.Context, nameOrAddress string) (*model.Input, error) {
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
				AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()))).
				AND(table.Input.SnapshotURI.IS_NOT_NULL()),
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

// DeleteApplication removes the row from "application" by ID.
func (r *PostgresRepository) DeleteApplication(
	ctx context.Context,
	id int64,
) error {
	delStmt := table.Application.
		DELETE().
		WHERE(table.Application.ID.EQ(postgres.Int(id)))

	sqlStr, args := delStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() != 1 {
		return fmt.Errorf("application with ID %d not found", id)
	}
	return nil
}

// ListApplications queries multiple apps with optional filters & pagination.
func (r *PostgresRepository) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Application, uint64, error) {

	fromClause := table.Application.INNER_JOIN(
		table.ExecutionParameters,
		table.ExecutionParameters.ApplicationID.EQ(table.Application.ID),
	)

	conditions := []postgres.BoolExpression{}
	if f.Enabled != nil {
		conditions = append(conditions, table.Application.Enabled.EQ(postgres.Bool(*f.Enabled)))
	}
	if f.Status != nil {
		conditions = append(conditions, table.Application.Status.EQ(postgres.NewEnumValue(f.Status.String())))
	}
	if len(f.Statuses) > 0 {
		statuses := make([]postgres.Expression, len(f.Statuses))
		for i, status := range f.Statuses {
			statuses[i] = postgres.NewEnumValue(status.String())
		}
		conditions = append(conditions, table.Application.Status.IN(statuses...))
	}
	if f.DataAvailability != nil {
		conditions = append(conditions,
			SubstrBytea(table.Application.DataAvailability, 1, 4).EQ(postgres.Bytea(f.DataAvailability[:])), //nolint:mnd
		)
	}
	if f.ConsensusType != nil {
		conditions = append(conditions, table.Application.ConsensusType.EQ(postgres.NewEnumValue(f.ConsensusType.String())))
	}
	if len(f.ConsensusTypes) > 0 {
		consensusTypes := make([]postgres.Expression, len(f.ConsensusTypes))
		for i, consensusType := range f.ConsensusTypes {
			consensusTypes[i] = postgres.NewEnumValue(consensusType.String())
		}
		conditions = append(conditions, table.Application.ConsensusType.IN(consensusTypes...))
	}
	if f.ForeclosureRecorded != nil {
		if *f.ForeclosureRecorded {
			conditions = append(conditions, table.Application.ForecloseBlock.GT(uint64Expr(0)))
		} else {
			conditions = append(conditions, table.Application.ForecloseBlock.EQ(uint64Expr(0)))
		}
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.Application.SELECT(postgres.COUNT(postgres.STAR)).FROM(fromClause)
	if len(conditions) > 0 {
		countStmt = countStmt.WHERE(postgres.AND(conditions...))
	}
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.Application.
		SELECT(
			table.Application.ID,
			table.Application.Name,
			table.Application.IapplicationAddress,
			table.Application.IconsensusAddress,
			table.Application.IinputboxAddress,
			table.Application.TemplateHash,
			table.Application.TemplateURI,
			table.Application.EpochLength,
			table.Application.ClaimStagingPeriod,
			table.Application.WithdrawalGuardian,
			table.Application.WithdrawalLog2LeavesPerAccount,
			table.Application.WithdrawalLog2MaxNumOfAccounts,
			table.Application.WithdrawalAccountsDriveStartIndex,
			table.Application.WithdrawalOutputBuilder,
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.Enabled,
			table.Application.Status,
			table.Application.Reason,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.LastForecloseCheckBlock,
			table.Application.LastAccountsDriveProvedCheckBlock,
			table.Application.LastWithdrawalCheckBlock,
			table.Application.ProcessedInputs,
			table.Application.ForecloseBlock,
			table.Application.ForecloseTransaction,
			table.Application.AccountsDriveProvedBlock,
			table.Application.AccountsDriveProvedTransaction,
			table.Application.AccountsDriveMerkleRoot,
			table.Application.CreatedAt,
			table.Application.UpdatedAt,
			table.ExecutionParameters.ApplicationID,
			table.ExecutionParameters.SnapshotPolicy,
			table.ExecutionParameters.AdvanceIncCycles,
			table.ExecutionParameters.AdvanceMaxCycles,
			table.ExecutionParameters.InspectIncCycles,
			table.ExecutionParameters.InspectMaxCycles,
			table.ExecutionParameters.AdvanceIncDeadline,
			table.ExecutionParameters.AdvanceMaxDeadline,
			table.ExecutionParameters.InspectIncDeadline,
			table.ExecutionParameters.InspectMaxDeadline,
			table.ExecutionParameters.LoadDeadline,
			table.ExecutionParameters.StoreDeadline,
			table.ExecutionParameters.FastDeadline,
			table.ExecutionParameters.MaxConcurrentInspects,
			table.ExecutionParameters.CreatedAt,
			table.ExecutionParameters.UpdatedAt,
		).
		FROM(fromClause)

	if len(conditions) > 0 {
		sel = sel.WHERE(postgres.AND(conditions...))
	}

	if descending {
		sel = sel.ORDER_BY(table.Application.Name.DESC())
	} else {
		sel = sel.ORDER_BY(table.Application.Name.ASC())
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

	var apps []*model.Application
	for rows.Next() {
		var app model.Application
		err := rows.Scan(
			&app.ID,
			&app.Name,
			&app.IApplicationAddress,
			&app.IConsensusAddress,
			&app.IInputBoxAddress,
			&app.TemplateHash,
			&app.TemplateURI,
			&app.EpochLength,
			&app.ClaimStagingPeriod,
			&app.WithdrawalConfig.Guardian,
			&app.WithdrawalConfig.Log2LeavesPerAccount,
			&app.WithdrawalConfig.Log2MaxNumOfAccounts,
			&app.WithdrawalConfig.AccountsDriveStartIndex,
			&app.WithdrawalConfig.WithdrawalOutputBuilder,
			&app.DataAvailability,
			&app.ConsensusType,
			&app.Enabled,
			&app.Status,
			&app.Reason,
			&app.IInputBoxBlock,
			&app.LastEpochCheckBlock,
			&app.LastInputCheckBlock,
			&app.LastOutputCheckBlock,
			&app.LastTournamentCheckBlock,
			&app.LastForecloseCheckBlock,
			&app.LastAccountsDriveProvedCheckBlock,
			&app.LastWithdrawalCheckBlock,
			&app.ProcessedInputs,
			&app.ForecloseBlock,
			&app.ForecloseTransaction,
			&app.AccountsDriveProvedBlock,
			&app.AccountsDriveProvedTransaction,
			&app.AccountsDriveMerkleRoot,
			&app.CreatedAt,
			&app.UpdatedAt,
			&app.ExecutionParameters.ApplicationID,
			&app.ExecutionParameters.SnapshotPolicy,
			&app.ExecutionParameters.AdvanceIncCycles,
			&app.ExecutionParameters.AdvanceMaxCycles,
			&app.ExecutionParameters.InspectIncCycles,
			&app.ExecutionParameters.InspectMaxCycles,
			&app.ExecutionParameters.AdvanceIncDeadline,
			&app.ExecutionParameters.AdvanceMaxDeadline,
			&app.ExecutionParameters.InspectIncDeadline,
			&app.ExecutionParameters.InspectMaxDeadline,
			&app.ExecutionParameters.LoadDeadline,
			&app.ExecutionParameters.StoreDeadline,
			&app.ExecutionParameters.FastDeadline,
			&app.ExecutionParameters.MaxConcurrentInspects,
			&app.ExecutionParameters.CreatedAt,
			&app.ExecutionParameters.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		apps = append(apps, &app)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return apps, total, nil
}

func (r *PostgresRepository) GetExecutionParameters(
	ctx context.Context,
	applicationID int64,
) (*model.ExecutionParameters, error) {

	stmt := table.ExecutionParameters.
		SELECT(
			table.ExecutionParameters.ApplicationID,
			table.ExecutionParameters.SnapshotPolicy,
			table.ExecutionParameters.AdvanceIncCycles,
			table.ExecutionParameters.AdvanceMaxCycles,
			table.ExecutionParameters.InspectIncCycles,
			table.ExecutionParameters.InspectMaxCycles,
			table.ExecutionParameters.AdvanceIncDeadline,
			table.ExecutionParameters.AdvanceMaxDeadline,
			table.ExecutionParameters.InspectIncDeadline,
			table.ExecutionParameters.InspectMaxDeadline,
			table.ExecutionParameters.LoadDeadline,
			table.ExecutionParameters.StoreDeadline,
			table.ExecutionParameters.FastDeadline,
			table.ExecutionParameters.MaxConcurrentInspects,
			table.ExecutionParameters.CreatedAt,
			table.ExecutionParameters.UpdatedAt,
		).
		WHERE(table.ExecutionParameters.ApplicationID.EQ(postgres.Int(applicationID)))

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var ep model.ExecutionParameters
	err := row.Scan(
		&ep.ApplicationID,
		&ep.SnapshotPolicy,
		&ep.AdvanceIncCycles,
		&ep.AdvanceMaxCycles,
		&ep.InspectIncCycles,
		&ep.InspectMaxCycles,
		&ep.AdvanceIncDeadline,
		&ep.AdvanceMaxDeadline,
		&ep.InspectIncDeadline,
		&ep.InspectMaxDeadline,
		&ep.LoadDeadline,
		&ep.StoreDeadline,
		&ep.FastDeadline,
		&ep.MaxConcurrentInspects,
		&ep.CreatedAt,
		&ep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // not found
	}
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

func (r *PostgresRepository) UpdateExecutionParameters(
	ctx context.Context,
	ep *model.ExecutionParameters,
) error {

	upd := table.ExecutionParameters.
		UPDATE(
			table.ExecutionParameters.SnapshotPolicy,
			table.ExecutionParameters.AdvanceIncCycles,
			table.ExecutionParameters.AdvanceMaxCycles,
			table.ExecutionParameters.InspectIncCycles,
			table.ExecutionParameters.InspectMaxCycles,
			table.ExecutionParameters.AdvanceIncDeadline,
			table.ExecutionParameters.AdvanceMaxDeadline,
			table.ExecutionParameters.InspectIncDeadline,
			table.ExecutionParameters.InspectMaxDeadline,
			table.ExecutionParameters.LoadDeadline,
			table.ExecutionParameters.StoreDeadline,
			table.ExecutionParameters.FastDeadline,
			table.ExecutionParameters.MaxConcurrentInspects,
		).
		SET(
			ep.SnapshotPolicy,
			ep.AdvanceIncCycles,
			ep.AdvanceMaxCycles,
			ep.InspectIncCycles,
			ep.InspectMaxCycles,
			ep.AdvanceIncDeadline,
			ep.AdvanceMaxDeadline,
			ep.InspectIncDeadline,
			ep.InspectMaxDeadline,
			ep.LoadDeadline,
			ep.StoreDeadline,
			ep.FastDeadline,
			ep.MaxConcurrentInspects,
		).
		WHERE(table.ExecutionParameters.ApplicationID.EQ(postgres.Int(ep.ApplicationID)))

	sqlStr, args := upd.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}
