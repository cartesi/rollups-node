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
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.State,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.ProcessedInputs,
		).
		VALUES(
			app.Name,
			app.IApplicationAddress,
			app.IConsensusAddress,
			app.IInputBoxAddress,
			app.TemplateHash,
			app.TemplateURI,
			app.EpochLength,
			app.DataAvailability,
			app.ConsensusType,
			app.State,
			app.IInputBoxBlock,
			app.LastEpochCheckBlock,
			app.LastInputCheckBlock,
			app.LastOutputCheckBlock,
			app.LastTournamentCheckBlock,
			app.ProcessedInputs,
		).
		RETURNING(table.Application.ID)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}

	sqlStr, args := insertStmt.Sql()
	var newID int64
	err = tx.QueryRow(ctx, sqlStr, args...).Scan(&newID)
	if err != nil {
		return 0, errors.Join(fmt.Errorf("unable to create database application: %w", err), tx.Rollback(ctx))
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
		return 0, errors.Join(err, tx.Rollback(ctx))
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, errors.Join(err, tx.Rollback(ctx))
	}
	return newID, nil
}

// GetApplication retrieves one application by ID, optionally loading status & execution parameters.
func (r *PostgresRepository) GetApplication(
	ctx context.Context,
	nameOrAddress string,
) (*model.Application, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

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
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.State,
			table.Application.Reason,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.ProcessedInputs,
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
	err = row.Scan(
		&app.ID,
		&app.Name,
		&app.IApplicationAddress,
		&app.IConsensusAddress,
		&app.IInputBoxAddress,
		&app.TemplateHash,
		&app.TemplateURI,
		&app.EpochLength,
		&app.DataAvailability,
		&app.ConsensusType,
		&app.State,
		&app.Reason,
		&app.IInputBoxBlock,
		&app.LastEpochCheckBlock,
		&app.LastInputCheckBlock,
		&app.LastOutputCheckBlock,
		&app.LastTournamentCheckBlock,
		&app.ProcessedInputs,
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

// GetProcessedInputs retrieves the ProcessedInputs field from a application by Name or address.
func (r *PostgresRepository) GetProcessedInputs(
	ctx context.Context,
	nameOrAddress string,
) (uint64, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return 0, err
	}

	stmt := table.Application.
		SELECT(table.Application.ProcessedInputs).
		WHERE(whereClause)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var processedInputs uint64
	err = row.Scan(&processedInputs)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrNotFound
	}
	return processedInputs, err
}

// UpdateApplication updates an existing application row.
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
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.State,
			table.Application.Reason,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.ProcessedInputs,
		).
		SET(
			app.Name,
			app.IApplicationAddress,
			app.IConsensusAddress,
			app.IInputBoxAddress,
			app.TemplateHash,
			app.TemplateURI,
			app.EpochLength,
			app.DataAvailability,
			app.ConsensusType,
			app.State,
			app.Reason,
			app.IInputBoxBlock,
			app.LastEpochCheckBlock,
			app.LastInputCheckBlock,
			app.LastOutputCheckBlock,
			app.LastTournamentCheckBlock,
			app.ProcessedInputs,
		).
		WHERE(table.Application.ID.EQ(postgres.Int(app.ID)))

	sqlStr, args := updateStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) UpdateApplicationState(
	ctx context.Context,
	appID int64,
	state model.ApplicationState,
	reason *string,
) error {

	updateStmt := table.Application.
		UPDATE(
			table.Application.State,
			table.Application.Reason,
		).
		SET(
			state,
			reason,
		).
		WHERE(table.Application.ID.EQ(postgres.Int(appID)))

	sqlStr, args := updateStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
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
			postgres.RawFloat(fmt.Sprintf("%d", blockNumber)),
		).
		WHERE(table.Application.ID.IN(ids...))

	sqlStr, args := updateStmt.Sql()
	_, err = r.db.Exec(ctx, sqlStr, args...)
	return err
}

// GetLastSnapshot retrieves the most recent input with a snapshot for the given application
func (r *PostgresRepository) GetLastSnapshot(ctx context.Context, nameOrAddress string) (*model.Input, error) {
	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
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
			table.Input.TransactionReference,
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
	err = row.Scan(
		&inp.EpochApplicationID,
		&inp.EpochIndex,
		&inp.Index,
		&inp.BlockNumber,
		&inp.RawData,
		&inp.Status,
		&inp.MachineHash,
		&inp.OutputsHash,
		&inp.TransactionReference,
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
			table.Application.DataAvailability,
			table.Application.ConsensusType,
			table.Application.State,
			table.Application.Reason,
			table.Application.IinputboxBlock,
			table.Application.LastEpochCheckBlock,
			table.Application.LastInputCheckBlock,
			table.Application.LastOutputCheckBlock,
			table.Application.LastTournamentCheckBlock,
			table.Application.ProcessedInputs,
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
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.Application.INNER_JOIN(
				table.ExecutionParameters,
				table.ExecutionParameters.ApplicationID.EQ(table.Application.ID),
			),
		)

	conditions := []postgres.BoolExpression{}
	if f.State != nil {
		conditions = append(conditions, table.Application.State.EQ(postgres.NewEnumValue(f.State.String())))
	}
	if f.DataAvailability != nil {
		conditions = append(conditions,
			SubstrBytea(table.Application.DataAvailability, 1, 4).EQ(postgres.Bytea(f.DataAvailability[:])), //nolint:mnd
		)
	}
	if f.ConsensusType != nil {
		conditions = append(conditions, table.Application.ConsensusType.EQ(postgres.NewEnumValue(f.ConsensusType.String())))
	}

	if len(conditions) > 0 {
		sel = sel.WHERE(postgres.AND(conditions...))
	}

	if descending {
		sel = sel.ORDER_BY(table.Application.Name.DESC())
	} else {
		sel = sel.ORDER_BY(table.Application.Name.ASC())
	}

	// Apply pagination
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

	var apps []*model.Application
	var total uint64
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
			&app.DataAvailability,
			&app.ConsensusType,
			&app.State,
			&app.Reason,
			&app.IInputBoxBlock,
			&app.LastEpochCheckBlock,
			&app.LastInputCheckBlock,
			&app.LastOutputCheckBlock,
			&app.LastTournamentCheckBlock,
			&app.ProcessedInputs,
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
			&total,
		)
		if err != nil {
			return nil, 0, err
		}
		apps = append(apps, &app)
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
		return sql.ErrNoRows
	}
	return nil
}
