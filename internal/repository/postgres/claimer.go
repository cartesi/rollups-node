// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

// Retrieve the claim of each application with the smallest index.
// The query may return either 0 or 1 entries per application.
//
// The returned model.Application is partially populated: the SELECT omits
// LastEpochCheckBlock, LastTournamentCheckBlock, LastForecloseCheckBlock,
// AccountsDriveProvedBlock, AccountsDriveProvedTransaction, and
// AccountsDriveMerkleRoot. Callers within the claimer pipeline only need
// the identity / consensus / status / foreclose-marker fields surfaced here;
// callers that need the omitted fields must use GetApplication instead.
func (r *PostgresRepository) selectOldestClaimPerApp(
	ctx context.Context,
	tx pgx.Tx,
	epochStatus model.EpochStatus,
) (
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	if (epochStatus != model.EpochStatus_ClaimSubmitted) &&
		(epochStatus != model.EpochStatus_ClaimComputed) &&
		(epochStatus != model.EpochStatus_ClaimStaged) {
		return nil, nil, fmt.Errorf("invalid epoch status: %v", epochStatus)
	}

	// NOTE(mpolitzer): DISTINCT ON is a postgres extension. To implement
	// this in SQLite there is an alternative using GROUP BY and HAVING
	// clauses instead.
	stmt := table.Epoch.SELECT(
		table.Epoch.ApplicationID,
		table.Epoch.Index,
		table.Epoch.FirstBlock,
		table.Epoch.LastBlock,
		table.Epoch.MachineHash,
		table.Epoch.OutputsMerkleRoot,
		table.Epoch.OutputsMerkleProof,
		table.Epoch.ClaimTransactionHash,
		table.Epoch.Status,
		table.Epoch.StagedAtBlock,
		table.Epoch.VirtualIndex,
		table.Epoch.CreatedAt,
		table.Epoch.UpdatedAt,

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
		table.Application.LastInputCheckBlock,
		table.Application.LastOutputCheckBlock,
		table.Application.ProcessedInputs,
		table.Application.ForecloseBlock,
		table.Application.ForecloseTransaction,
		table.Application.CreatedAt,
		table.Application.UpdatedAt,
	).
		DISTINCT(table.Epoch.ApplicationID).
		FROM(
			table.Epoch.
				INNER_JOIN(
					table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			table.Epoch.Status.EQ(postgres.NewEnumValue(epochStatus.String())).
				AND(table.Application.Enabled.EQ(postgres.Bool(true))).
				AND(claimableOrForeclosedApplication()).
				AND(table.Application.ConsensusType.NOT_EQ(enum.Consensus.Prt)),
		).
		ORDER_BY(
			table.Epoch.ApplicationID,
			table.Epoch.Index.ASC(),
		)

	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("querying oldest claim per app (status=%v): %w", epochStatus, err)
	}
	defer rows.Close()

	epochs := map[int64]*model.Epoch{}
	applications := map[int64]*model.Application{}
	for rows.Next() {
		var application model.Application
		var epoch model.Epoch
		err := rows.Scan(
			&epoch.ApplicationID,
			&epoch.Index,
			&epoch.FirstBlock,
			&epoch.LastBlock,
			&epoch.MachineHash,
			&epoch.OutputsMerkleRoot,
			&epoch.OutputsMerkleProof,
			&epoch.ClaimTransactionHash,
			&epoch.Status,
			&epoch.StagedAtBlock,
			&epoch.VirtualIndex,
			&epoch.CreatedAt,
			&epoch.UpdatedAt,

			&application.ID,
			&application.Name,
			&application.IApplicationAddress,
			&application.IConsensusAddress,
			&application.IInputBoxAddress,
			&application.TemplateHash,
			&application.TemplateURI,
			&application.EpochLength,
			&application.ClaimStagingPeriod,
			&application.WithdrawalConfig.Guardian,
			&application.WithdrawalConfig.Log2LeavesPerAccount,
			&application.WithdrawalConfig.Log2MaxNumOfAccounts,
			&application.WithdrawalConfig.AccountsDriveStartIndex,
			&application.WithdrawalConfig.WithdrawalOutputBuilder,
			&application.DataAvailability,
			&application.ConsensusType,
			&application.Enabled,
			&application.Status,
			&application.Reason,
			&application.IInputBoxBlock,
			&application.LastInputCheckBlock,
			&application.LastOutputCheckBlock,
			&application.ProcessedInputs,
			&application.ForecloseBlock,
			&application.ForecloseTransaction,
			&application.CreatedAt,
			&application.UpdatedAt,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("scanning epoch/application row: %w", err)
		}
		epochs[application.ID] = &epoch
		applications[application.ID] = &application
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating oldest claim rows: %w", err)
	}
	return epochs, applications, nil
}

// Retrieve the newest claim barrier of each application.
func (r *PostgresRepository) selectNewestClaimBarrierPerApp(
	ctx context.Context,
	tx pgx.Tx,
	statuses ...model.EpochStatus,
) (
	map[int64]*model.Epoch,
	error,
) {
	if len(statuses) == 0 {
		return nil, fmt.Errorf("selecting newest claim barrier: no statuses provided")
	}
	statusExprs := make([]postgres.Expression, 0, len(statuses))
	for _, status := range statuses {
		statusExprs = append(statusExprs, postgres.NewEnumValue(status.String()))
	}

	// NOTE(mpolitzer): DISTINCT ON is a postgres extension. To implement
	// this in SQLite there is an alternative using GROUP BY and HAVING
	// clauses instead.
	stmt := table.Epoch.SELECT(
		table.Epoch.ApplicationID,
		table.Epoch.Index,
		table.Epoch.FirstBlock,
		table.Epoch.LastBlock,
		table.Epoch.MachineHash,
		table.Epoch.OutputsMerkleRoot,
		table.Epoch.OutputsMerkleProof,
		table.Epoch.ClaimTransactionHash,
		table.Epoch.Status,
		table.Epoch.StagedAtBlock,
		table.Epoch.VirtualIndex,
		table.Epoch.CreatedAt,
		table.Epoch.UpdatedAt,
	).
		DISTINCT(table.Epoch.ApplicationID).
		FROM(
			table.Epoch.
				INNER_JOIN(
					table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			table.Epoch.Status.IN(statusExprs...).
				AND(table.Application.Enabled.EQ(postgres.Bool(true))).
				AND(claimableOrForeclosedApplication()).
				AND(table.Application.ConsensusType.NOT_EQ(enum.Consensus.Prt)),
		).
		ORDER_BY(
			table.Epoch.ApplicationID,
			table.Epoch.Index.DESC(),
		)

	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("querying newest claim barrier per app: %w", err)
	}
	defer rows.Close()

	epochs := map[int64]*model.Epoch{}
	for rows.Next() {
		var epoch model.Epoch
		err := rows.Scan(
			&epoch.ApplicationID,
			&epoch.Index,
			&epoch.FirstBlock,
			&epoch.LastBlock,
			&epoch.MachineHash,
			&epoch.OutputsMerkleRoot,
			&epoch.OutputsMerkleProof,
			&epoch.ClaimTransactionHash,
			&epoch.Status,
			&epoch.StagedAtBlock,
			&epoch.VirtualIndex,
			&epoch.CreatedAt,
			&epoch.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning claim barrier epoch row: %w", err)
		}
		epochs[epoch.ApplicationID] = &epoch
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating claim barrier rows: %w", err)
	}
	return epochs, nil
}

func claimableOrForeclosedApplication() postgres.BoolExpression {
	// Health gate only: a claim-eligible application has node-health OK.
	// Foreclosure is orthogonal (foreclose_block); a foreclosed-but-healthy
	// application is still OK and is handled by the caller's drain logic.
	return table.Application.Status.EQ(enum.ApplicationStatus.Ok)
}

func (r *PostgresRepository) SelectClaimsToSubmitPerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("beginning read-only transaction for submitted claims: %w", err)
	}
	// Read-only tx: rollback releases the snapshot, equivalent to commit.
	defer tx.Rollback(ctx) //nolint:errcheck

	computed, applications, err := r.selectOldestClaimPerApp(ctx, tx, model.EpochStatus_ClaimComputed)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("selecting oldest computed claim per app: %w", err)
	}

	barriers, err := r.selectNewestClaimBarrierPerApp(
		ctx,
		tx,
		model.EpochStatus_ClaimAccepted,
		model.EpochStatus_ClaimSubmitted,
		model.EpochStatus_ClaimStaged,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("selecting newest claim barrier per app: %w", err)
	}

	return barriers, computed, applications, err
}

func (r *PostgresRepository) SelectClaimsToStagePerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("beginning read-only transaction for accepted claims: %w", err)
	}
	// Read-only tx: rollback releases the snapshot, equivalent to commit.
	defer tx.Rollback(ctx) //nolint:errcheck

	submitted, applications, err := r.selectOldestClaimPerApp(ctx, tx, model.EpochStatus_ClaimSubmitted)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("selecting oldest submitted claim per app: %w", err)
	}

	accepted, err := r.selectNewestClaimBarrierPerApp(ctx, tx, model.EpochStatus_ClaimAccepted)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("selecting newest accepted claim per app: %w", err)
	}

	return accepted, submitted, applications, err
}

func (r *PostgresRepository) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	applicationID int64,
	index uint64,
	transactionHash common.Hash,
) error {
	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.ClaimTransactionHash,
			table.Epoch.Status,
		).
		SET(
			transactionHash,
			postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing update for submitted claim (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// UpdateEpochWithAcceptedClaim transitions an epoch to CLAIM_ACCEPTED. The
// source state may be CLAIM_SUBMITTED, CLAIM_STAGED, or CLAIM_COMPUTED — the
// trigger enforces validity per the v3 state machine:
//
//   - CLAIM_STAGED → CLAIM_ACCEPTED is the normal v3 path (after the staging
//     period elapses and acceptClaim is called).
//   - CLAIM_COMPUTED → CLAIM_ACCEPTED is the deep reader-mode catch-up path
//     (also PRT's terminal transition; the trigger forbids PRT from STAGED).
//   - CLAIM_SUBMITTED → CLAIM_ACCEPTED is permitted by the trigger but not
//     reached by the v3 happy path. Kept for resilience.
//
// staged_at_block is intentionally left untouched: it is a permanent fact
// (the chain block at which staging happened), kept across the transition
// to ACCEPTED for audit/forensics — same convention as
// claim_transaction_hash. The relaxed staged_requires_block CHECK permits
// this.
//
// txHash is optional:
//   - When non-nil, claim_transaction_hash is set to the supplied value.
//     This is the catch-up path: an epoch coming directly from
//     CLAIM_COMPUTED never went through CLAIM_SUBMITTED, so the column was
//     never populated. Callers that observed the ClaimAccepted event pass
//     the event's tx hash here for forensic continuity.
//   - When nil, claim_transaction_hash is left untouched. This is the
//     normal-flow path: the column was set during the CLAIM_SUBMITTED
//     transition and carries through the rest of the FSM.
func (r *PostgresRepository) UpdateEpochWithAcceptedClaim(
	ctx context.Context,
	applicationID int64,
	index uint64,
	txHash *common.Hash,
) error {
	whereClause := table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
		AND(table.Epoch.Index.EQ(uint64Expr(index))).
		AND(table.Epoch.Status.IN(
			postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()),
			postgres.NewEnumValue(model.EpochStatus_ClaimStaged.String()),
			postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()),
		))

	var updStmt postgres.UpdateStatement
	if txHash == nil {
		updStmt = table.Epoch.
			UPDATE(table.Epoch.Status).
			SET(postgres.NewEnumValue(model.EpochStatus_ClaimAccepted.String())).
			FROM(table.Application).
			WHERE(whereClause)
	} else {
		updStmt = table.Epoch.
			UPDATE(table.Epoch.Status, table.Epoch.ClaimTransactionHash).
			SET(postgres.NewEnumValue(model.EpochStatus_ClaimAccepted.String()), *txHash).
			FROM(table.Application).
			WHERE(whereClause)
	}

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing update for accepted claim (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// UpdateEpochWithForeclosedClaim transitions a single epoch's non-terminal claim
// status to CLAIM_FORECLOSED after application foreclosure makes the claim path
// impossible on chain. It is keyed by (application, epoch index) and applies to
// any consensus type: the Authority/Quorum claimer terminalizes a claim it can
// no longer submit, and the PRT drain terminalizes a pre-foreclosure epoch the
// tournament never accepted. Earlier non-terminal states are allowed because an
// epoch that overlaps the foreclosure block may never reach a computable claim.
func (r *PostgresRepository) UpdateEpochWithForeclosedClaim(
	ctx context.Context,
	applicationID int64,
	index uint64,
) error {
	updStmt := table.Epoch.
		UPDATE(table.Epoch.Status).
		SET(postgres.NewEnumValue(model.EpochStatus_ClaimForeclosed.String())).
		FROM(table.Application).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.IN(
					postgres.NewEnumValue(model.EpochStatus_Open.String()),
					postgres.NewEnumValue(model.EpochStatus_Closed.String()),
					postgres.NewEnumValue(model.EpochStatus_InputsProcessed.String()),
					postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()),
					postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()),
					postgres.NewEnumValue(model.EpochStatus_ClaimStaged.String()),
				)).
				AND(table.Application.ID.EQ(table.Epoch.ApplicationID)).
				AND(table.Application.ForecloseBlock.GT(uint64Expr(0))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing update for foreclosed claim (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// RejectEpochAndSetApplicationDiverged atomically records that the local claim
// lost the applicable consensus/dispute process and halts the application as
// DIVERGED. Quorum rejection is only a normal outcome before the local claim
// has staged; once CLAIM_STAGED is recorded, a different staged or accepted
// claim for the same epoch would violate the contract's single-staged claim
// invariant. The epoch reject and the application halt share one transaction,
// and the halt runs even when the epoch reject matched no row, so a detected
// divergence can never leave the application runnable.
func (r *PostgresRepository) RejectEpochAndSetApplicationDiverged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	reason string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction for rejected claim update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rejectStmt := table.Epoch.
		UPDATE(table.Epoch.Status).
		SET(postgres.NewEnumValue(model.EpochStatus_ClaimRejected.String())).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.IN(
					postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()),
					postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()),
				)),
		)

	// The epoch reject is best-effort: an epoch outside CLAIM_COMPUTED/
	// CLAIM_SUBMITTED is left untouched. The application halt below does not
	// depend on it — a detected divergence always stops the application.
	sqlStr, args := rejectStmt.Sql()
	if _, err := tx.Exec(ctx, sqlStr, args...); err != nil {
		return fmt.Errorf("executing rejected claim update (app=%d, index=%d): %w", applicationID, index, err)
	}

	appStmt := table.Application.
		UPDATE(
			table.Application.Status,
			table.Application.Reason,
		).
		SET(
			model.ApplicationStatus_Diverged,
			&reason,
		).
		WHERE(table.Application.ID.EQ(postgres.Int64(applicationID)))

	sqlStr, args = appStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing diverged application update (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return tx.Commit(ctx)
}

// UpdateEpochToStaged transitions an epoch from CLAIM_SUBMITTED to
// CLAIM_STAGED, recording the on-chain staging block.
func (r *PostgresRepository) UpdateEpochToStaged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
		).
		SET(
			postgres.NewEnumValue(model.EpochStatus_ClaimStaged.String()),
			uint64Expr(stagedAtBlock),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.EQ(
					postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing update to staged (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// UpdateEpochThroughStaging atomically transitions an epoch from
// CLAIM_COMPUTED → CLAIM_SUBMITTED → CLAIM_STAGED in a single transaction.
// Used by the Authority/Quorum-deciding fast-path where the submit-tx
// receipt contains both ClaimSubmitted and ClaimStaged events; the trigger
// permits both legs and we record both transitions atomically so that a
// crash between them cannot leave the DB inconsistent with the chain.
func (r *PostgresRepository) UpdateEpochThroughStaging(
	ctx context.Context,
	applicationID int64,
	index uint64,
	transactionHash common.Hash,
	stagedAtBlock uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction for through-staging update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	submitStmt := table.Epoch.
		UPDATE(
			table.Epoch.ClaimTransactionHash,
			table.Epoch.Status,
		).
		SET(
			transactionHash,
			postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.EQ(
					postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()))),
		)
	sqlStr, args := submitStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing through-staging submit leg (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}

	stageStmt := table.Epoch.
		UPDATE(
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
		).
		SET(
			postgres.NewEnumValue(model.EpochStatus_ClaimStaged.String()),
			uint64Expr(stagedAtBlock),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.EQ(
					postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()))),
		)
	sqlStr, args = stageStmt.Sql()
	cmd, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing through-staging stage leg (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}

	return tx.Commit(ctx)
}

// UpdateEpochReconciledStaged transitions an epoch from CLAIM_COMPUTED to
// CLAIM_STAGED without setting a claim_transaction_hash. Used by the
// pre-submit reconciliation path when getClaim() reveals the chain has
// already staged our claim (e.g., across a restart or in reader mode).
func (r *PostgresRepository) UpdateEpochReconciledStaged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
		).
		SET(
			postgres.NewEnumValue(model.EpochStatus_ClaimStaged.String()),
			uint64Expr(stagedAtBlock),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.EQ(
					postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("executing reconciled-staged update (app=%d, index=%d): %w", applicationID, index, err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// SelectClaimsToAcceptPerApp returns, for each Authority/Quorum application
// with at least one CLAIM_STAGED epoch:
//   - the oldest CLAIM_STAGED epoch (the next one waiting to be accepted),
//   - the newest already-accepted epoch (for cross-checks),
//   - the application row.
//
// Used by the claimer's accept stages to drive CLAIM_STAGED → CLAIM_ACCEPTED
// transitions.
func (r *PostgresRepository) SelectClaimsToAcceptPerApp(ctx context.Context) (
	map[int64]*model.Epoch,
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("beginning read-only transaction for staged claims: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	staged, applications, err := r.selectOldestClaimPerApp(ctx, tx, model.EpochStatus_ClaimStaged)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("selecting oldest staged claim per app: %w", err)
	}

	accepted, err := r.selectNewestClaimBarrierPerApp(ctx, tx, model.EpochStatus_ClaimAccepted)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("selecting newest accepted claim per app: %w", err)
	}

	return accepted, staged, applications, err
}
