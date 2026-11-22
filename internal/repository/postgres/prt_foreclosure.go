// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
)

func prtProblemCondition() postgres.BoolExpression {
	app := table.Application
	return app.ProblemEpochIndex.IS_NOT_NULL().
		AND(app.ProblemKind.IN(executionStopStatusExpressions...)).
		AND(app.ProblemInputIndex.IS_NOT_NULL())
}

// Application and history admission uses the terminal statement's snapshot.
// It does not cancel an already admitted statement after a concurrent stop.
func prtForeclosureCondition() postgres.BoolExpression {
	app, epoch := table.Application, table.Epoch
	return app.ID.EQ(epoch.ApplicationID).
		AND(app.ConsensusType.EQ(enum.Consensus.Prt)).
		AND(app.Enabled.EQ(postgres.Bool(true))).
		AND(app.Status.IN(append([]postgres.Expression{enum.ApplicationStatus.Ok}, executionStopStatusExpressions...)...)).
		AND(app.ProblemEpochIndex.IS_NULL().AND(app.ProblemKind.IS_NULL()).AND(app.ProblemInputIndex.IS_NULL()).
			AND(app.Status.EQ(enum.ApplicationStatus.Ok)).OR(prtProblemCondition())).
		AND(app.ForecloseBlock.GT(uint64Expr(0))).
		AND(app.LastEpochCheckBlock.GT_EQ(app.ForecloseBlock)).
		AND(app.LastInputCheckBlock.GT_EQ(app.ForecloseBlock)).
		AND(epoch.FirstBlock.LT_EQ(app.ForecloseBlock)).
		AND(epoch.Status.IN(nonTerminalEpochStatusExpressions...))
}

func prtComputedCondition() postgres.BoolExpression {
	app, epoch := table.Application, table.Epoch
	return epoch.Status.EQ(enum.EpochStatus.ClaimComputed).
		AND(epoch.TournamentAddress.IS_NOT_NULL()).
		AND(epoch.ClaimValidated.EQ(postgres.Bool(true))).
		AND(epoch.MachineHash.IS_NOT_NULL()).
		AND(epoch.Commitment.IS_NOT_NULL()).
		AND(epoch.CommitmentProof.IS_NOT_NULL()).
		AND(app.ProblemEpochIndex.IS_NULL().AND(app.Status.EQ(enum.ApplicationStatus.Ok)).
			OR(prtProblemCondition().AND(epoch.Index.LT(app.ProblemEpochIndex))))
}

// The bounded input rule matches HasUndrainedEpochsBeforeBlock. A positive
// input stop permits its non-executable suffix; pending earlier work does not.
func prtUncomputedExitCondition() postgres.BoolExpression {
	app, epoch, input := table.Application, table.Epoch, table.Input
	pending := input.SELECT(input.Index).WHERE(
		input.EpochApplicationID.EQ(epoch.ApplicationID).
			AND(input.EpochIndex.EQ(epoch.Index)).
			AND(input.BlockNumber.LT_EQ(app.ForecloseBlock)).
			AND(input.Status.EQ(enum.InputCompletionStatus.None)).
			AND(app.Status.NOT_IN(executionStopStatusExpressions...).OR(input.Index.LT(app.ProcessedInputs))))
	inputStop := app.ProblemEpochIndex.IS_NOT_NULL().
		AND(app.ProblemKind.IN(executionStopStatusExpressions...)).
		AND(app.ProblemInputIndex.IS_NOT_NULL()).
		AND(epoch.Index.GT_EQ(app.ProblemEpochIndex)).
		AND(epoch.Status.IN(enum.EpochStatus.Open, enum.EpochStatus.Closed, enum.EpochStatus.InputsProcessed))
	openBoundary := epoch.Status.EQ(enum.EpochStatus.Open).AND(epoch.TournamentAddress.IS_NULL())
	return inputStop.OR(openBoundary).AND(postgres.NOT(postgres.EXISTS(pending)))
}

func prtTerminalObservationCondition() postgres.BoolExpression {
	return table.Epoch.Status.IN(enum.EpochStatus.ClaimSubmitted, enum.EpochStatus.ClaimStaged)
}

func prtForeclosureSQL(applicationID int64, index, observedHead uint64) (string, []any) {
	app, epoch, root := table.Application, table.Epoch, table.Tournaments
	terminalRoot := root.SELECT(root.Address).WHERE(
		root.ApplicationID.EQ(epoch.ApplicationID).
			AND(root.EpochIndex.EQ(epoch.Index)).
			AND(root.Address.EQ(epoch.TournamentAddress)).
			AND(root.Level.EQ(postgres.Int(0))).
			AND(root.FinishedAtBlock.GT(uint64Expr(0))).
			AND(root.FinishedAtBlock.LT_EQ(root.AsOfBlock)).
			AND(root.AsOfBlock.LT_EQ(uint64Expr(observedHead))).
			AND(root.Standing.EQ(enum.TournamentStandingState.RootFailed).
				OR(root.Standing.EQ(enum.TournamentStandingState.RootWinner).AND(root.FinalStateHash.EQ(epoch.MachineHash)))))
	predicate := epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
		AND(epoch.Index.EQ(uint64Expr(index))).
		AND(prtForeclosureCondition()).
		AND(app.ForecloseBlock.LT_EQ(uint64Expr(observedHead))).
		AND(epoch.ClaimTransactionHash.IS_NULL()).
		AND(prtComputedCondition().AND(postgres.EXISTS(terminalRoot)).
			OR(prtUncomputedExitCondition()).OR(prtTerminalObservationCondition()))
	return epoch.UPDATE(epoch.Status).SET(enum.EpochStatus.ClaimForeclosed).
		FROM(app).WHERE(predicate).Sql()
}

// ForeclosePRTEpoch retains local producers until the target has a positive
// terminal branch. The SQL compares the current target with durable root facts;
// neither an old discovery page nor a volatile comparison flag authorizes it.
func (r *PostgresRepository) ForeclosePRTEpoch(
	ctx context.Context, applicationID int64, index, observedHead uint64,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	query, args := prtForeclosureSQL(applicationID, index, observedHead)
	updated, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("foreclosing PRT epoch (application=%d, epoch=%d): %w", applicationID, index, err)
	}
	if updated.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}
