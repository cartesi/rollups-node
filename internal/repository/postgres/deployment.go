// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func deploymentIdentityPredicate(key repository.DeploymentIdentity) postgres.BoolExpression {
	return table.Application.ID.EQ(postgres.Int64(key.ApplicationID)).
		AND(table.Application.IapplicationAddress.EQ(postgres.Bytea(key.ApplicationAddress.Bytes()))).
		AND(table.Application.IconsensusAddress.EQ(postgres.Bytea(key.ConsensusAddress.Bytes())))
}

func deploymentSelect(key repository.DeploymentIdentity) postgres.SelectStatement {
	control := table.DeploymentAdmissionState
	return control.SELECT(control.AllColumns).
		FROM(control.INNER_JOIN(table.Application, control.ApplicationID.EQ(table.Application.ID))).
		WHERE(deploymentIdentityPredicate(key))
}

func readDeploymentState(ctx context.Context, queryer rowQueryer, stmt postgres.SelectStatement) (*model.DeploymentAdmissionState, error) {
	query, args := stmt.Sql()
	return scanDeploymentState(queryer.QueryRow(ctx, query, args...))
}

func scanDeploymentState(row pgx.Row) (*model.DeploymentAdmissionState, error) {
	var value model.DeploymentAdmissionState
	if err := row.Scan(&value.ApplicationID, &value.ResetGeneration, &value.ConfirmedTier, &value.DeploymentReason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &value, nil
}

func (r *PostgresRepository) GetDeploymentAdmissionState(
	ctx context.Context, key repository.DeploymentIdentity,
) (*model.DeploymentAdmissionState, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	return readDeploymentState(ctx, r.db, deploymentSelect(key))
}

func (r *PostgresRepository) BeginDeploymentStartup(
	ctx context.Context, key repository.DeploymentIdentity, expectedGeneration int64,
) (*model.DeploymentAdmissionState, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer rollbackDeliveryTx(tx)
	control := table.DeploymentAdmissionState
	query, args := control.UPDATE(control.ResetGeneration).SET(control.ResetGeneration.ADD(postgres.Int64(1))).
		WHERE(control.ApplicationID.EQ(postgres.Int64(key.ApplicationID)).
			AND(control.ResetGeneration.EQ(postgres.Int64(expectedGeneration))).
			AND(postgres.EXISTS(table.Application.SELECT(table.Application.ID).WHERE(deploymentIdentityPredicate(key))))).
		RETURNING(control.AllColumns).Sql()
	state, err := scanDeploymentState(tx.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, repository.ErrNoUpdate
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return state, nil
}

func (r *PostgresRepository) AcknowledgeDeploymentAssessment(
	ctx context.Context, request repository.DeploymentAssessment,
) (*model.DeploymentAdmissionState, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer rollbackDeliveryTx(tx)
	control := table.DeploymentAdmissionState
	stmt := deploymentSelect(request.Identity).
		WHERE(deploymentIdentityPredicate(request.Identity).AND(control.ResetGeneration.EQ(postgres.Int64(request.ResetGeneration)))).
		FOR(postgres.NO_KEY_UPDATE().OF(control))
	before, err := readDeploymentState(ctx, tx, stmt)
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, repository.ErrNoUpdate
	}
	var tier, reason any
	change := before.ConfirmedTier != nil
	if request.ConfirmedTier != nil {
		tier, reason = string(*request.ConfirmedTier), request.Reason
		change = before.ConfirmedTier == nil || *before.ConfirmedTier != *request.ConfirmedTier ||
			*before.DeploymentReason != request.Reason
	}
	if change {
		query, args := control.UPDATE(control.ConfirmedTier, control.DeploymentReason).SET(tier, reason).
			WHERE(control.ApplicationID.EQ(postgres.Int64(request.Identity.ApplicationID))).Sql()
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return before, nil
}

// All reads share one repeatable-read snapshot. The transaction closes before return.
func (r *PostgresRepository) GetDeploymentCompletionState(
	ctx context.Context, key repository.DeploymentIdentity, index uint64,
) (*repository.DeploymentCompletionState, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer rollbackDeliveryTx(tx)
	completion, err := readEpochCompletion(ctx, tx, key.ApplicationAddress.Hex(), index)
	if err != nil || completion == nil || completion.Application == nil {
		return nil, err
	}
	app := completion.Application
	if app.ID != key.ApplicationID || app.IConsensusAddress != key.ConsensusAddress {
		return nil, repository.ErrNoUpdate
	}
	control, err := readDeploymentState(ctx, tx, table.DeploymentAdmissionState.SELECT(table.DeploymentAdmissionState.AllColumns).
		WHERE(table.DeploymentAdmissionState.ApplicationID.EQ(postgres.Int64(app.ID))))
	if err != nil {
		return nil, err
	}
	value := &repository.DeploymentCompletionState{
		Completion: completion,
		Deployment: control,
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return value, nil
}
