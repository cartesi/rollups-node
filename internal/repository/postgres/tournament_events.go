// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

// StoreTournamentEvents publishes event facts and pinned current snapshots in
// one application-wide transaction. Parents must precede their children.
func (r *PostgresRepository) StoreTournamentEvents(
	ctx context.Context, appID int64, batches []*repository.TournamentEventBatch, lastBlock uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tournament event window: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Keep the application aggregate lock before every child-row lock.
	lockStmt := table.Application.SELECT(table.Application.ID).
		WHERE(table.Application.ID.EQ(postgres.Int64(appID))).FOR(postgres.NO_KEY_UPDATE())
	lockSQL, lockArgs := lockStmt.Sql()
	var lockedID int64
	if err := tx.QueryRow(ctx, lockSQL, lockArgs...).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("locking tournament application %d: %w", appID, err)
	}
	for _, batch := range batches {
		if err := validateTournamentBatch(appID, batch, lastBlock); err != nil {
			return err
		}
		if err := storeTournamentProjection(ctx, tx, appID, batch.Tournament); err != nil {
			return err
		}
		if err := insertCommitments(ctx, tx, appID, batch.Commitments); err != nil {
			return fmt.Errorf("storing tournament commitments: %w", err)
		}
		if err := insertMatches(ctx, tx, appID, batch.Matches); err != nil {
			return fmt.Errorf("storing tournament matches: %w", err)
		}
		if err := insertMatchAdvanced(ctx, tx, appID, batch.MatchAdvances); err != nil {
			return fmt.Errorf("storing tournament match advances: %w", err)
		}
		if err := insertBondEvents(ctx, tx, appID, batch.BondEvents); err != nil {
			return fmt.Errorf("storing tournament bonds: %w", err)
		}
	}
	if err := updateLastProcessedBlock(ctx, tx, appID, lastBlock); err != nil {
		return fmt.Errorf("storing tournament checkpoint: %w", err)
	}
	return tx.Commit(ctx)
}

func validateTournamentBatch(appID int64, batch *repository.TournamentEventBatch, lastBlock uint64) error {
	if batch == nil || batch.Tournament == nil {
		return fmt.Errorf("nil tournament batch for application %d", appID)
	}
	t := batch.Tournament
	if t.ApplicationID != appID || t.Snapshot.AsOfBlock != lastBlock {
		return fmt.Errorf("tournament projection does not match application %d at block %d", appID, lastBlock)
	}
	for _, v := range batch.Commitments {
		if v == nil || v.ApplicationID != appID || v.EpochIndex != t.EpochIndex ||
			v.TournamentAddress != t.Address || v.Snapshot.AsOfBlock != lastBlock {
			return fmt.Errorf("commitment does not belong to tournament %s at block %d", t.Address, lastBlock)
		}
	}
	for _, v := range batch.Matches {
		if v == nil || v.ApplicationID != appID || v.EpochIndex != t.EpochIndex ||
			v.TournamentAddress != t.Address || v.Snapshot.AsOfBlock != lastBlock {
			return fmt.Errorf("match does not belong to tournament %s at block %d", t.Address, lastBlock)
		}
	}
	for _, v := range batch.MatchAdvances {
		if v == nil || v.ApplicationID != appID || v.EpochIndex != t.EpochIndex ||
			v.TournamentAddress != t.Address || v.BlockNumber > lastBlock {
			return fmt.Errorf("advance does not belong to tournament %s at block %d", t.Address, lastBlock)
		}
	}
	for _, v := range batch.BondEvents {
		if v == nil || v.ApplicationID != appID || v.EpochIndex != t.EpochIndex ||
			v.TournamentAddress != t.Address || v.BlockNumber > lastBlock {
			return fmt.Errorf("bond event does not belong to tournament %s at block %d", t.Address, lastBlock)
		}
	}
	return nil
}

func tournamentUpsert(v *Tournament, applicationID any) postgres.InsertStatement {
	values := tournamentValues(v)
	values[0] = applicationID
	return table.Tournaments.INSERT(tournamentColumns).ON_CONFLICT(
		table.Tournaments.ApplicationID, table.Tournaments.EpochIndex, table.Tournaments.Address,
	).DO_UPDATE(postgres.SET(
		table.Tournaments.AsOfBlock.SET(table.Tournaments.EXCLUDED.AsOfBlock),
		table.Tournaments.Standing.SET(table.Tournaments.EXCLUDED.Standing),
		table.Tournaments.AcceptsJoins.SET(table.Tournaments.EXCLUDED.AcceptsJoins),
		table.Tournaments.Candidate.SET(table.Tournaments.EXCLUDED.Candidate),
		table.Tournaments.WinnerCommitment.SET(table.Tournaments.EXCLUDED.WinnerCommitment),
		table.Tournaments.FinalStateHash.SET(table.Tournaments.EXCLUDED.FinalStateHash),
		table.Tournaments.FinishedAtBlock.SET(table.Tournaments.EXCLUDED.FinishedAtBlock),
		table.Tournaments.ParentCommitment.SET(table.Tournaments.EXCLUDED.ParentCommitment),
		table.Tournaments.WinnerExpiresAt.SET(table.Tournaments.EXCLUDED.WinnerExpiresAt),
		table.Tournaments.InnerDisposition.SET(table.Tournaments.EXCLUDED.InnerDisposition),
		table.Tournaments.InnerParentCommitment.SET(table.Tournaments.EXCLUDED.InnerParentCommitment),
		table.Tournaments.InnerPausedAllowance.SET(table.Tournaments.EXCLUDED.InnerPausedAllowance),
		table.Tournaments.BondDisposition.SET(table.Tournaments.EXCLUDED.BondDisposition),
		table.Tournaments.BondClaimer.SET(table.Tournaments.EXCLUDED.BondClaimer),
		table.Tournaments.BondPayment.SET(table.Tournaments.EXCLUDED.BondPayment),
	).WHERE(postgres.AND(
		table.Tournaments.EpochIndex.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.EpochIndex),
		table.Tournaments.Address.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.Address),
		table.Tournaments.ParentTournamentAddress.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.ParentTournamentAddress),
		table.Tournaments.ParentMatchIDHash.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.ParentMatchIDHash),
		table.Tournaments.MaxLevel.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.MaxLevel),
		table.Tournaments.Level.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.Level),
		table.Tournaments.Log2step.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.Log2step),
		table.Tournaments.Height.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.Height),
		table.Tournaments.InitialHash.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.InitialHash),
		table.Tournaments.BaseCycle.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.BaseCycle),
		table.Tournaments.Kind.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.Kind),
		table.Tournaments.StartInstant.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.StartInstant),
		table.Tournaments.Allowance.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.Allowance),
		table.Tournaments.CreationBlockNumber.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.CreationBlockNumber),
		table.Tournaments.CreationTxHash.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.CreationTxHash),
		table.Tournaments.CreationLogIndex.IS_NOT_DISTINCT_FROM(table.Tournaments.EXCLUDED.CreationLogIndex),
		table.Tournaments.AsOfBlock.LT_EQ(table.Tournaments.EXCLUDED.AsOfBlock),
	))).VALUES(values[0], values[1:]...)
}

func storeTournamentProjection(ctx context.Context, tx pgx.Tx, appID int64, v *Tournament) error {
	stmt := tournamentUpsert(v, appID)
	sqlStr, args := stmt.Sql()
	command, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("storing tournament %s: %w", v.Address, err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: tournament %s", repository.ErrTournamentEventConflict, v.Address)
	}
	return nil
}

func insertCommitments(ctx context.Context, tx pgx.Tx, appID int64, values []*Commitment) error {
	if len(values) == 0 {
		return nil
	}
	stmt := table.Commitments.INSERT(commitmentColumns).ON_CONFLICT(
		table.Commitments.ApplicationID, table.Commitments.EpochIndex, table.Commitments.TournamentAddress, table.Commitments.Commitment,
	).DO_UPDATE(postgres.SET(
		table.Commitments.AsOfBlock.SET(table.Commitments.EXCLUDED.AsOfBlock),
		table.Commitments.Claimer.SET(table.Commitments.EXCLUDED.Claimer),
		table.Commitments.ClockRunning.SET(table.Commitments.EXCLUDED.ClockRunning),
		table.Commitments.ClockDeadline.SET(table.Commitments.EXCLUDED.ClockDeadline),
		table.Commitments.ClockAllowance.SET(table.Commitments.EXCLUDED.ClockAllowance),
	).WHERE(postgres.AND(
		table.Commitments.EpochIndex.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.EpochIndex),
		table.Commitments.TournamentAddress.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.TournamentAddress),
		table.Commitments.Commitment.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.Commitment),
		table.Commitments.FinalStateHash.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.FinalStateHash),
		table.Commitments.SubmitterAddress.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.SubmitterAddress),
		table.Commitments.BlockNumber.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.BlockNumber),
		table.Commitments.TxHash.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.TxHash),
		table.Commitments.LogIndex.IS_NOT_DISTINCT_FROM(table.Commitments.EXCLUDED.LogIndex),
		table.Commitments.AsOfBlock.LT_EQ(table.Commitments.EXCLUDED.AsOfBlock),
	)))
	for _, v := range values {
		if v == nil || v.ApplicationID != appID {
			return fmt.Errorf("invalid commitment application identity")
		}
		row := commitmentValues(v)
		stmt = stmt.VALUES(row[0], row[1:]...)
	}
	sqlStr, args := stmt.Sql()
	command, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() != int64(len(values)) {
		return fmt.Errorf("%w: commitment window", repository.ErrTournamentEventConflict)
	}
	return nil
}

func insertMatches(ctx context.Context, tx pgx.Tx, appID int64, values []*Match) error {
	if len(values) == 0 {
		return nil
	}
	stmt := table.Matches.INSERT(matchColumns).ON_CONFLICT(
		table.Matches.ApplicationID, table.Matches.EpochIndex, table.Matches.TournamentAddress, table.Matches.IDHash,
	).DO_UPDATE(postgres.SET(
		table.Matches.SealEliminableAt.SET(table.Matches.EXCLUDED.SealEliminableAt),
		table.Matches.SealBlockNumber.SET(table.Matches.EXCLUDED.SealBlockNumber),
		table.Matches.SealTxHash.SET(table.Matches.EXCLUDED.SealTxHash),
		table.Matches.SealLogIndex.SET(table.Matches.EXCLUDED.SealLogIndex),
		table.Matches.AsOfBlock.SET(table.Matches.EXCLUDED.AsOfBlock),
		table.Matches.Phase.SET(table.Matches.EXCLUDED.Phase),
		table.Matches.TimeoutOutcome.SET(table.Matches.EXCLUDED.TimeoutOutcome),
		table.Matches.DeferredCharge.SET(table.Matches.EXCLUDED.DeferredCharge),
		table.Matches.RevealingParent.SET(table.Matches.EXCLUDED.RevealingParent),
		table.Matches.WaitingLeft.SET(table.Matches.EXCLUDED.WaitingLeft),
		table.Matches.WaitingRight.SET(table.Matches.EXCLUDED.WaitingRight),
		table.Matches.SegmentStartPosition.SET(table.Matches.EXCLUDED.SegmentStartPosition),
		table.Matches.SegmentStartCycle.SET(table.Matches.EXCLUDED.SegmentStartCycle),
		table.Matches.CurrentHeight.SET(table.Matches.EXCLUDED.CurrentHeight),
		table.Matches.Responder.SET(table.Matches.EXCLUDED.Responder),
		table.Matches.AgreeState.SET(table.Matches.EXCLUDED.AgreeState),
		table.Matches.DivergencePosition.SET(table.Matches.EXCLUDED.DivergencePosition),
		table.Matches.DivergenceCycle.SET(table.Matches.EXCLUDED.DivergenceCycle),
		table.Matches.FinalStateOne.SET(table.Matches.EXCLUDED.FinalStateOne),
		table.Matches.FinalStateTwo.SET(table.Matches.EXCLUDED.FinalStateTwo),
		table.Matches.Winner.SET(table.Matches.EXCLUDED.Winner),
		table.Matches.DeletionReason.SET(table.Matches.EXCLUDED.DeletionReason),
		table.Matches.DeletionBlockNumber.SET(table.Matches.EXCLUDED.DeletionBlockNumber),
		table.Matches.DeletionTxHash.SET(table.Matches.EXCLUDED.DeletionTxHash),
		table.Matches.DeletionLogIndex.SET(table.Matches.EXCLUDED.DeletionLogIndex),
	).WHERE(postgres.AND(
		table.Matches.EpochIndex.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.EpochIndex),
		table.Matches.TournamentAddress.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.TournamentAddress),
		table.Matches.IDHash.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.IDHash),
		table.Matches.CommitmentOne.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.CommitmentOne),
		table.Matches.CommitmentTwo.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.CommitmentTwo),
		table.Matches.LeftOfTwo.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.LeftOfTwo),
		table.Matches.BlockNumber.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.BlockNumber),
		table.Matches.TxHash.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.TxHash),
		table.Matches.LogIndex.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.LogIndex),
		table.Matches.EliminableAt.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.EliminableAt),
		table.Matches.AsOfBlock.LT_EQ(table.Matches.EXCLUDED.AsOfBlock),
		table.Matches.SealTxHash.IS_NULL().OR(postgres.AND(
			table.Matches.SealEliminableAt.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.SealEliminableAt),
			table.Matches.SealBlockNumber.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.SealBlockNumber),
			table.Matches.SealTxHash.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.SealTxHash),
			table.Matches.SealLogIndex.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.SealLogIndex),
		)),
		table.Matches.DeletionReason.EQ(postgres.NewEnumValue(MatchDeletionReason_NOT_DELETED.String())).OR(postgres.AND(
			table.Matches.Winner.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.Winner),
			table.Matches.DeletionReason.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.DeletionReason),
			table.Matches.DeletionBlockNumber.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.DeletionBlockNumber),
			table.Matches.DeletionTxHash.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.DeletionTxHash),
			table.Matches.DeletionLogIndex.IS_NOT_DISTINCT_FROM(table.Matches.EXCLUDED.DeletionLogIndex),
		)),
	)))
	for _, v := range values {
		if v == nil || v.ApplicationID != appID {
			return fmt.Errorf("invalid match application identity")
		}
		row := matchValues(v)
		stmt = stmt.VALUES(row[0], row[1:]...)
	}
	sqlStr, args := stmt.Sql()
	command, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() != int64(len(values)) {
		return fmt.Errorf("%w: match window", repository.ErrTournamentEventConflict)
	}
	return nil
}

func insertMatchAdvanced(ctx context.Context, tx pgx.Tx, appID int64, values []*MatchAdvanced) error {
	if len(values) == 0 {
		return nil
	}
	stmt := table.MatchAdvances.INSERT(matchAdvanceColumns)
	for _, v := range values {
		if v == nil || v.ApplicationID != appID {
			return fmt.Errorf("invalid matchAdvance application identity")
		}
		row := matchAdvanceValues(v)
		stmt = stmt.VALUES(row[0], row[1:]...)
	}
	sqlStr, args := stmt.ON_CONFLICT(
		table.MatchAdvances.ApplicationID, table.MatchAdvances.TxHash, table.MatchAdvances.LogIndex,
	).DO_NOTHING().Sql()
	command, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() == int64(len(values)) {
		return nil
	}
	// A duplicate event keeps its original row and timestamps. Check its payload
	// after the insert; a same-key contradiction must roll back the window.
	for _, v := range values {
		sel := table.MatchAdvances.SELECT(matchAdvanceColumns, table.MatchAdvances.CreatedAt, table.MatchAdvances.UpdatedAt).WHERE(
			table.MatchAdvances.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.MatchAdvances.TxHash.EQ(postgres.Bytea(v.TxHash.Bytes()))).
				AND(table.MatchAdvances.LogIndex.EQ(uint64Expr(v.LogIndex))))
		query, params := sel.Sql()
		stored, err := scanMatchAdvanced(tx.QueryRow(ctx, query, params...))
		if err != nil {
			return err
		}
		if !sameMatchAdvancedEvent(stored, v) {
			return fmt.Errorf("%w: transaction %s log %d", repository.ErrTournamentEventConflict, v.TxHash, v.LogIndex)
		}
	}
	return nil
}

func sameMatchAdvancedEvent(a, b *MatchAdvanced) bool {
	return a.ApplicationID == b.ApplicationID &&
		a.EpochIndex == b.EpochIndex &&
		a.TournamentAddress == b.TournamentAddress &&
		a.IDHash == b.IDHash &&
		a.OtherParent == b.OtherParent &&
		a.LeftNode == b.LeftNode &&
		a.SegmentStartPosition == b.SegmentStartPosition &&
		a.EliminableAt == b.EliminableAt &&
		a.BlockNumber == b.BlockNumber &&
		a.TxHash == b.TxHash &&
		a.LogIndex == b.LogIndex
}

func insertBondEvents(ctx context.Context, tx pgx.Tx, appID int64, values []*BondEvent) error {
	if len(values) == 0 {
		return nil
	}
	stmt := table.BondEvents.INSERT(bondEventColumns)
	for _, v := range values {
		if v == nil || v.ApplicationID != appID {
			return fmt.Errorf("invalid bondEvent application identity")
		}
		row := bondEventValues(v)
		stmt = stmt.VALUES(row[0], row[1:]...)
	}
	sqlStr, args := stmt.ON_CONFLICT(table.BondEvents.ApplicationID, table.BondEvents.TxHash, table.BondEvents.LogIndex).DO_NOTHING().Sql()
	command, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() == int64(len(values)) {
		return nil
	}
	// A duplicate event keeps its original row and timestamps. Check its payload
	// after the insert; a same-key contradiction must roll back the window.
	for _, v := range values {
		sel := table.BondEvents.SELECT(bondEventColumns, table.BondEvents.CreatedAt, table.BondEvents.UpdatedAt).WHERE(
			table.BondEvents.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.BondEvents.TxHash.EQ(postgres.Bytea(v.TxHash.Bytes()))).
				AND(table.BondEvents.LogIndex.EQ(uint64Expr(v.LogIndex))))
		query, params := sel.Sql()
		stored, err := scanBondEvent(tx.QueryRow(ctx, query, params...))
		if err != nil {
			return err
		}
		if !sameBondEvent(stored, v) {
			return fmt.Errorf("%w: transaction %s log %d", repository.ErrTournamentEventConflict, v.TxHash, v.LogIndex)
		}
	}
	return nil
}

func sameBondEvent(a, b *BondEvent) bool {
	if a.ApplicationID != b.ApplicationID || a.EpochIndex != b.EpochIndex || a.TournamentAddress != b.TournamentAddress ||
		a.Type != b.Type || a.BlockNumber != b.BlockNumber || a.TxHash != b.TxHash || a.LogIndex != b.LogIndex {
		return false
	}
	if (a.Refund == nil) != (b.Refund == nil) || (a.Recovery == nil) != (b.Recovery == nil) {
		return false
	}
	return (a.Refund == nil || *a.Refund == *b.Refund) && (a.Recovery == nil || *a.Recovery == *b.Recovery)
}
