// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

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
	if kind == nil || input == nil || !kind.IsExecutionTerminal() && *kind != model.ApplicationStatus_InvalidOutputsRoot {
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
	return readEpochCompletion(ctx, tx, nameOrAddress, index)
}

func readEpochCompletion(ctx context.Context, tx pgx.Tx, nameOrAddress string, index uint64) (*repository.EpochCompletionState, error) {
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
