// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/appstatus"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

// Execution stops preserve a valid earlier prefix; FAILED/DC do not grant
// permission to discard work, even when a previous stop boundary is retained.
var executionStopStatusExpressions = []postgres.Expression{
	enum.ApplicationStatus.GuestException, enum.ApplicationStatus.MachineHalted,
	enum.ApplicationStatus.McycleOverflow, enum.ApplicationStatus.UnexpectedYield,
	enum.ApplicationStatus.InvalidOutputsRoot,
}

type rowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func decodeApplicationProblem(epoch *uint64, kind *model.ApplicationStatus, input *uint64) (*model.ApplicationProblem, error) {
	if epoch == nil {
		if kind != nil || input != nil {
			return nil, fmt.Errorf("epoch stop boundary is missing its epoch")
		}
		return nil, nil
	}
	if kind == nil || !kind.IsExecutionTerminal() && *kind != model.ApplicationStatus_InvalidOutputsRoot ||
		kind.IsExecutionTerminal() && input == nil {
		return nil, fmt.Errorf("epoch %d has inconsistent stop metadata", *epoch)
	}
	return &model.ApplicationProblem{EpochIndex: *epoch, Kind: *kind, InputIndex: input}, nil
}

func (r *PostgresRepository) GetEpochCompletionState(
	ctx context.Context, nameOrAddress string, index uint64,
) (*repository.EpochCompletionState, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer rollbackDeliveryTx(tx)
	app, problem, err := getApplication(ctx, tx, nameOrAddress)
	if err != nil || app == nil {
		return nil, err
	}
	epoch, err := getEpoch(ctx, tx, nameOrAddress, index)
	if err != nil {
		return nil, err
	}
	return &repository.EpochCompletionState{Application: app, Problem: problem, Epoch: epoch}, nil
}

// A boundary writer locks the application before the epoch. Its status and
// cutoff checks must remain current until the new finding commits.
func lockEpochCompletion(ctx context.Context, tx pgx.Tx, appID int64, index uint64) (*repository.EpochCompletionState, error) {
	stmt := table.Application.SELECT(table.Application.Status,
		table.Application.ProblemEpochIndex, table.Application.ProblemKind, table.Application.ProblemInputIndex).
		WHERE(table.Application.ID.EQ(postgres.Int64(appID))).FOR(postgres.NO_KEY_UPDATE())
	query, args := stmt.Sql()
	app := &model.Application{ID: appID}
	var problemEpoch, problemInput *uint64
	var problemKind *model.ApplicationStatus
	if err := tx.QueryRow(ctx, query, args...).Scan(&app.Status, &problemEpoch, &problemKind, &problemInput); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNoUpdate
		}
		return nil, err
	}
	problem, err := decodeApplicationProblem(problemEpoch, problemKind, problemInput)
	if err != nil {
		return nil, err
	}
	epoch := &model.Epoch{ApplicationID: appID, Index: index}
	stmt = table.Epoch.SELECT(table.Epoch.Status).
		WHERE(table.Epoch.ApplicationID.EQ(postgres.Int64(appID)).AND(table.Epoch.Index.EQ(uint64Expr(index)))).
		FOR(postgres.UPDATE())
	query, args = stmt.Sql()
	if err := tx.QueryRow(ctx, query, args...).Scan(&epoch.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNoUpdate
		}
		return nil, err
	}
	if epoch.Status == model.EpochStatus_ClaimForeclosed {
		return nil, repository.ErrEpochForeclosed
	}
	if epoch.Status != model.EpochStatus_InputsProcessed {
		return nil, repository.ErrNoUpdate
	}
	return &repository.EpochCompletionState{Application: app, Problem: problem, Epoch: epoch}, nil
}

// recordApplicationProblem runs only while its caller holds the application
// row lock. Replace the entire typed source only for a strictly earlier epoch.
func recordApplicationProblem(ctx context.Context, tx pgx.Tx, appID int64, p model.ApplicationProblem) error {
	var input any
	if p.InputIndex != nil {
		input = uint64Expr(*p.InputIndex)
	}
	stmt := table.Application.UPDATE(table.Application.ProblemEpochIndex, table.Application.ProblemKind,
		table.Application.ProblemInputIndex).SET(uint64Expr(p.EpochIndex), p.Kind, input).
		WHERE(table.Application.ID.EQ(postgres.Int64(appID)).AND(
			table.Application.ProblemEpochIndex.IS_NULL().OR(table.Application.ProblemEpochIndex.GT(uint64Expr(p.EpochIndex)))))
	query, args := stmt.Sql()
	_, err := tx.Exec(ctx, query, args...)
	return err
}

func (r *PostgresRepository) StoreInvalidOutputsRoot(ctx context.Context, appID int64, index uint64, reason string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackDeliveryTx(tx)
	state, err := lockEpochCompletion(ctx, tx, appID, index)
	if err != nil {
		return err
	}
	status := state.Application.Status
	if status != model.ApplicationStatus_OK && !status.IsExecutionTerminal() && status != model.ApplicationStatus_InvalidOutputsRoot {
		return fmt.Errorf("%w: application %d has status %s", repository.ErrApplicationNotRunnable, appID, status)
	}
	// Repeated evidence at the cutoff is idempotent. A later stale validation
	// cannot replace an earlier finding or clear a concurrent FAILED/DC stop.
	if state.Problem == nil && status != model.ApplicationStatus_OK {
		return fmt.Errorf("application %d is missing its epoch stop boundary", appID)
	}
	if state.Problem != nil && index > state.Problem.EpochIndex {
		return fmt.Errorf("%w: epoch %d is after the stop boundary", repository.ErrApplicationNotRunnable, index)
	}
	if err := recordApplicationProblem(ctx, tx, appID, model.ApplicationProblem{
		EpochIndex: index, Kind: model.ApplicationStatus_InvalidOutputsRoot,
	}); err != nil {
		return err
	}
	if status == model.ApplicationStatus_OK {
		stmt := table.Application.UPDATE(table.Application.Status, table.Application.Reason).
			SET(model.ApplicationStatus_InvalidOutputsRoot, appstatus.NormalizeReason(reason)).
			WHERE(table.Application.ID.EQ(postgres.Int64(appID)))
		query, args := stmt.Sql()
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
