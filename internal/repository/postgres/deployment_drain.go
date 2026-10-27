// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func deploymentDrainSQL(request repository.DeploymentDrainRequest) (string, []any) {
	predicate := deploymentIdentityPredicate(request.Identity).
		AND(table.Application.ConsensusType.EQ(enum.Consensus.Prt)).
		AND(table.Epoch.ApplicationID.EQ(postgres.Int64(request.Identity.ApplicationID))).
		AND(table.Epoch.Index.EQ(uint64Expr(request.EpochIndex))).
		AND(table.Epoch.FirstBlock.LT_EQ(table.Application.ForecloseBlock)).
		AND(table.Epoch.Status.IN(nonTerminalEpochStatusExpressions...)).
		AND(table.Application.ID.EQ(table.Epoch.ApplicationID)).
		AND(table.Application.ForecloseBlock.GT(uint64Expr(0))).
		AND(table.Epoch.Status.IN(enum.EpochStatus.ClaimComputed, enum.EpochStatus.ClaimSubmitted, enum.EpochStatus.ClaimStaged).
			OR(table.Application.Enabled.EQ(postgres.Bool(true)).
				AND(table.Application.Status.IN(
					append([]postgres.Expression{enum.ApplicationStatus.Ok}, executionStopStatusExpressions...)...)).
				AND(table.Epoch.Status.NOT_EQ(enum.EpochStatus.InputsProcessed).
					OR(table.Epoch.LastBlock.GT_EQ(table.Application.ForecloseBlock)).
					OR(table.Application.Status.IN(executionStopStatusExpressions...).
						AND(table.Epoch.Index.GT_EQ(table.Application.ProblemEpochIndex))))))
	return table.Epoch.UPDATE(table.Epoch.Status).SET(enum.EpochStatus.ClaimForeclosed).
		FROM(table.Application).WHERE(predicate).Sql()
}

// ForecloseDeploymentEpoch sets CLAIM_FORECLOSED at the selected generation and generation tier.
// It locks the epoch, then control with NOWAIT so application deletion cannot form a lock cycle.
func (r *PostgresRepository) ForecloseDeploymentEpoch(
	ctx context.Context, request repository.DeploymentDrainRequest,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer rollbackDeliveryTx(tx)
	if err := setLocalLockTimeout(ctx, tx); err != nil {
		return err
	}
	epoch := table.Epoch
	lockEpoch := epoch.SELECT(epoch.ApplicationID).
		WHERE(epoch.ApplicationID.EQ(postgres.Int64(request.Identity.ApplicationID)).
			AND(epoch.Index.EQ(uint64Expr(request.EpochIndex))).
			AND(epoch.Status.IN(nonTerminalEpochStatusExpressions...))).FOR(postgres.NO_KEY_UPDATE().OF(epoch))
	query, args := lockEpoch.Sql()
	var applicationID int64
	if err := tx.QueryRow(ctx, query, args...).Scan(&applicationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.ErrNoUpdate
		}
		return err
	}
	// No Ensure, application write, or application row lock is permitted after epoch ownership.
	control := table.DeploymentAdmissionState
	lockControl := deploymentSelect(request.Identity).
		WHERE(deploymentIdentityPredicate(request.Identity).
			AND(control.ResetGeneration.EQ(postgres.Int64(request.ResetGeneration))).
			AND(control.ConfirmedTier.EQ(postgres.String(string(model.DeploymentTierGeneration))))).
		FOR(postgres.SHARE().OF(control).NOWAIT())
	state, err := readDeploymentState(ctx, tx, lockControl)
	if err != nil {
		return err
	}
	if state == nil {
		return repository.ErrNoUpdate
	}
	query, args = deploymentDrainSQL(request)
	updated, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if updated.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return tx.Commit(ctx)
}
