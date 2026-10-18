// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func (r *PostgresRepository) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	applicationID int64,
	index uint64,
	transactionHash common.Hash,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

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

// RefreshClaimObservation corrects same-content re-inclusion metadata without
// rolling claim progress back or replacing a concurrent observation.
func (r *PostgresRepository) RefreshClaimObservation(
	ctx context.Context, expected *model.Epoch, transactionHash *common.Hash, stagedAtBlock *uint64,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	if expected == nil || expected.MachineHash == nil || expected.TxBufferDataBlock == nil ||
		(expected.Status != model.EpochStatus_ClaimSubmitted && expected.Status != model.EpochStatus_ClaimStaged) ||
		(expected.Status == model.EpochStatus_ClaimStaged && stagedAtBlock == nil) ||
		(transactionHash != nil && *transactionHash == (common.Hash{})) {
		return fmt.Errorf("claim observation refresh requires an identified submitted or staged claim")
	}
	status := enum.EpochStatus.ClaimSubmitted
	if expected.Status == model.EpochStatus_ClaimStaged {
		status = enum.EpochStatus.ClaimStaged
	}
	conditions := []postgres.BoolExpression{
		table.Epoch.ApplicationID.EQ(postgres.Int64(expected.ApplicationID)),
		table.Epoch.Index.EQ(uint64Expr(expected.Index)),
		table.Epoch.Status.EQ(status),
		table.Epoch.LastBlock.EQ(uint64Expr(expected.LastBlock)),
		table.Epoch.MachineHash.EQ(postgres.Bytea(expected.MachineHash.Bytes())),
		table.Epoch.TxBufferDataBlock.EQ(postgres.Bytea(expected.TxBufferDataBlock.Bytes())),
		table.Application.ID.EQ(table.Epoch.ApplicationID),
		table.Application.ConsensusType.IN(enum.Consensus.Authority, enum.Consensus.Quorum),
	}
	if expected.ClaimTransactionHash == nil {
		conditions = append(conditions, table.Epoch.ClaimTransactionHash.IS_NULL())
	} else {
		conditions = append(conditions, table.Epoch.ClaimTransactionHash.EQ(postgres.Bytea(expected.ClaimTransactionHash.Bytes())))
	}
	if expected.StagedAtBlock == nil {
		conditions = append(conditions, table.Epoch.StagedAtBlock.IS_NULL())
	} else {
		conditions = append(conditions, table.Epoch.StagedAtBlock.EQ(uint64Expr(*expected.StagedAtBlock)))
	}
	stmt := table.Epoch.UPDATE(table.Epoch.ClaimTransactionHash).SET(hashToBytes(transactionHash)).
		FROM(table.Application).WHERE(postgres.AND(conditions...))
	if stagedAtBlock != nil {
		stmt = table.Epoch.UPDATE(table.Epoch.ClaimTransactionHash, table.Epoch.StagedAtBlock).
			SET(hashToBytes(transactionHash), uint64Expr(*stagedAtBlock)).FROM(table.Application).WHERE(postgres.AND(conditions...))
	}
	query, args := stmt.Sql()
	command, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("refreshing claim observation (app=%d, index=%d): %w", expected.ApplicationID, expected.Index, err)
	}
	if command.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// UpdateEpochWithAcceptedClaim transitions an epoch to CLAIM_ACCEPTED. The
// source state may be CLAIM_SUBMITTED, CLAIM_STAGED, or CLAIM_COMPUTED — the
// trigger enforces validity per the v3 state machine:
//
//   - CLAIM_STAGED → CLAIM_ACCEPTED is the normal v3 path after the staging
//     period elapses and the consensus accepts the staged result.
//   - CLAIM_COMPUTED → CLAIM_ACCEPTED is the deep reader-mode catch-up path,
//     including a PRT node that first observes an already accepted result.
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
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

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
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	updStmt := table.Epoch.
		UPDATE(table.Epoch.Status).
		SET(postgres.NewEnumValue(model.EpochStatus_ClaimForeclosed.String())).
		FROM(table.Application).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(index))).
				AND(table.Epoch.Status.IN(nonTerminalEpochStatusExpressions...)).
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
// lost the applicable consensus/dispute process. Quorum rejection is only a
// normal outcome before the local claim has staged; once CLAIM_STAGED is
// recorded, a different staged or accepted claim for the same epoch would
// violate the contract's single-staged claim invariant.
//
// Unlike an executed-output byte mismatch, losing consensus does not prove the
// node's stored history is corrupt. An existing terminal machine outcome is
// therefore preserved while the epoch rejection is still attempted. For a
// non-terminal application, the DIVERGED halt runs even when the epoch reject
// matched no row. The returned flags identify the writes that committed.
func (r *PostgresRepository) RejectEpochAndSetApplicationDiverged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	reason string,
) (repository.RejectEpochAndDivergeResult, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	var result repository.RejectEpochAndDivergeResult
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("beginning transaction for rejected claim update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Take the shared aggregate lock before updating the epoch. SELECT FOR NO KEY
	// UPDATE reads the current status without changing the application row, but
	// its transaction-scoped row lock conflicts with concurrent application
	// updates. Matching StoreAdvanceResult and CreateEpochsAndInputs' Application
	// -> child-row order prevents an Epoch -> Application deadlock cycle.
	//
	// Commit or rollback releases the lock. PostgreSQL also aborts the
	// transaction after detecting a lost node connection, and crash recovery
	// discards it after a database restart. Detection can be delayed by a network
	// failure, but the lock is not persisted independently of the transaction.
	lockAppStmt := table.Application.
		SELECT(table.Application.Status).
		WHERE(table.Application.ID.EQ(postgres.Int64(applicationID))).
		FOR(postgres.NO_KEY_UPDATE())
	lockAppSQL, lockAppArgs := lockAppStmt.Sql()
	var applicationStatus model.ApplicationStatus
	if err := tx.QueryRow(ctx, lockAppSQL, lockAppArgs...).Scan(&applicationStatus); errors.Is(err, pgx.ErrNoRows) {
		return result, repository.ErrNotFound
	} else if err != nil {
		return result, fmt.Errorf("locking application for rejected claim update (app=%d, index=%d): %w", applicationID, index, err)
	}

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
	// CLAIM_SUBMITTED is left untouched. For a non-terminal application, the
	// DIVERGED transition below is independent of whether this update matched.
	sqlStr, args := rejectStmt.Sql()
	rejectCmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return result, fmt.Errorf("executing rejected claim update (app=%d, index=%d): %w", applicationID, index, err)
	}
	result.EpochRejected = rejectCmd.RowsAffected() == 1

	// Preserve an already-terminal cause. The best-effort epoch rejection still
	// commits, but a later claim observation must not overwrite a stronger or
	// earlier durable terminal status.
	if !applicationStatus.IsTerminal() {
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
			return repository.RejectEpochAndDivergeResult{}, fmt.Errorf(
				"executing diverged application update (app=%d, index=%d): %w",
				applicationID, index, err)
		}
		if cmd.RowsAffected() == 0 {
			return repository.RejectEpochAndDivergeResult{}, repository.ErrNotFound
		}
		result.ApplicationDiverged = true
	}

	if err := tx.Commit(ctx); err != nil {
		return repository.RejectEpochAndDivergeResult{}, fmt.Errorf(
			"committing rejected claim update (app=%d, index=%d): %w",
			applicationID, index, err)
	}
	return result, nil
}

// UpdateEpochToStaged transitions an epoch from CLAIM_SUBMITTED to
// CLAIM_STAGED, recording the on-chain staging block.
func (r *PostgresRepository) UpdateEpochToStaged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

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

// UpdateEpochStagingBlock records same-content re-inclusion without replacing
// an accepted epoch or a staging observation committed by another writer.
func (r *PostgresRepository) UpdateEpochStagingBlock(
	ctx context.Context, applicationID int64, index, expectedOldBlock, newBlock uint64,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	stmt := table.Epoch.UPDATE(table.Epoch.StagedAtBlock).SET(uint64Expr(newBlock)).FROM(table.Application).WHERE(
		table.Epoch.ApplicationID.EQ(postgres.Int64(applicationID)).
			AND(table.Epoch.Index.EQ(uint64Expr(index))).
			AND(table.Epoch.Status.EQ(enum.EpochStatus.ClaimStaged)).
			AND(table.Epoch.StagedAtBlock.EQ(uint64Expr(expectedOldBlock))).
			AND(table.Application.ID.EQ(table.Epoch.ApplicationID)).
			AND(table.Application.ConsensusType.EQ(enum.Consensus.Prt)))
	query, args := stmt.Sql()
	command, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("refreshing epoch staging block (app=%d, index=%d): %w", applicationID, index, err)
	}
	if command.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

// UpdateEpochReconciledStaged transitions an epoch from CLAIM_COMPUTED to
// CLAIM_STAGED without setting a claim_transaction_hash. Used by the
// chain reconciliation path when the chain has already staged the local
// result. Authority/Quorum and PRT can use this path after a restart or in
// reader mode.
func (r *PostgresRepository) UpdateEpochReconciledStaged(
	ctx context.Context,
	applicationID int64,
	index uint64,
	stagedAtBlock uint64,
) error {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

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

// selectClaimsPage visits one next application, then reads its oldest row plus
// a resumed later window. A busy app cannot keep the next app off the next pass.
func (r *PostgresRepository) selectClaimsPage(
	ctx context.Context, cursor repository.ClaimCursor, limit uint64, status model.EpochStatus,
) (repository.ClaimPage, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()

	if cursor.AfterApplicationID < 0 || limit < 2 || limit > math.MaxInt64 {
		return repository.ClaimPage{}, fmt.Errorf("claim page requires a nonnegative app cursor and a limit of at least two")
	}
	filter := repository.ApplicationFilter{Enabled: new(true),
		ConsensusTypes: []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum}}
	// Rotate over applications with work in this source. Observers keep every
	// health status; publisher cursors exclude applications that cannot send.
	next, err := r.nextClaimApplication(ctx, cursor.AfterApplicationID, status, cursor.HealthyOnly)
	if err != nil {
		return repository.ClaimPage{}, err
	}
	if next == 0 && cursor.AfterApplicationID != 0 {
		next, err = r.nextClaimApplication(ctx, 0, status, cursor.HealthyOnly)
		if err != nil {
			return repository.ClaimPage{}, err
		}
	}
	if next == 0 {
		return repository.ClaimPage{}, nil
	}
	apps, err := r.ListApplicationsByID(ctx, filter, next-1, next, 1)
	if err != nil || len(apps) == 0 {
		// Deletion or disabling between these reads simply leaves this pass empty.
		return repository.ClaimPage{ApplicationID: next}, err
	}
	app := apps[0]
	page := repository.ClaimPage{ApplicationID: app.ID}
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return page, err
	}
	defer rollbackDeliveryTx(tx)
	oldest, err := claimWindowRows(ctx, tx, app.ID, status, nil, 1)
	if err != nil {
		return page, err
	}
	if len(oldest) == 0 {
		return page, nil
	}
	page.Work = append(page.Work, repository.ClaimWork{Application: app, Epoch: oldest[0]})
	after := oldest[0].Index
	if saved, ok := cursor.Epochs[app.ID]; ok && saved > after {
		after = saved
	}
	later, err := claimWindowRows(ctx, tx, app.ID, status, &after, int64(limit))
	if err != nil {
		return repository.ClaimPage{}, err
	}
	if uint64(len(later)) >= limit {
		later = later[:limit-1]
		page.NextEpoch = new(later[len(later)-1].Index)
	}
	for _, epoch := range later {
		page.Work = append(page.Work, repository.ClaimWork{Application: app, Epoch: epoch})
	}
	return page, nil
}

func (r *PostgresRepository) nextClaimApplication(
	ctx context.Context, after int64, status model.EpochStatus, healthyOnly bool,
) (int64, error) {
	pending := table.Epoch.SELECT(table.Epoch.ApplicationID).WHERE(table.Epoch.ApplicationID.EQ(table.Application.ID).
		AND(table.Epoch.Status.EQ(postgres.NewEnumValue(status.String()))))
	conditions := []postgres.BoolExpression{
		table.Application.Enabled.EQ(postgres.Bool(true)),
		table.Application.ConsensusType.IN(enum.Consensus.Authority, enum.Consensus.Quorum),
		table.Application.ID.GT(postgres.Int64(after)),
		postgres.EXISTS(pending),
	}
	if healthyOnly {
		conditions = append(conditions, table.Application.Status.EQ(enum.ApplicationStatus.Ok),
			table.Application.ForecloseBlock.EQ(uint64Expr(0)))
	}
	stmt := table.Application.SELECT(table.Application.ID).WHERE(postgres.AND(conditions...)).
		ORDER_BY(table.Application.ID.ASC()).LIMIT(1)
	query, args := stmt.Sql()
	var id int64
	if err := r.db.QueryRow(ctx, query, args...).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("selecting next claim application: %w", err)
	}
	return id, nil
}

func claimWindowRows(
	ctx context.Context, tx pgx.Tx, appID int64, status model.EpochStatus, after *uint64, limit int64,
) ([]*model.Epoch, error) {
	conditions := []postgres.BoolExpression{table.Epoch.ApplicationID.EQ(postgres.Int64(appID)),
		table.Epoch.Status.EQ(postgres.NewEnumValue(status.String()))}
	if after != nil {
		conditions = append(conditions, table.Epoch.Index.GT(uint64Expr(*after)))
	}
	stmt := table.Epoch.SELECT(table.Epoch.ApplicationID, table.Epoch.Index, table.Epoch.FirstBlock, table.Epoch.LastBlock,
		table.Epoch.MachineHash, table.Epoch.TxBufferDataBlock, table.Epoch.TxBufferProof,
		table.Epoch.IflagsYDataBlock, table.Epoch.IflagsYProof, table.Epoch.HtifTohostDataBlock,
		table.Epoch.HtifTohostProof, table.Epoch.ClaimTransactionHash, table.Epoch.Status,
		table.Epoch.StagedAtBlock, table.Epoch.VirtualIndex, table.Epoch.CreatedAt, table.Epoch.UpdatedAt).
		WHERE(postgres.AND(conditions...)).ORDER_BY(table.Epoch.Index.ASC()).LIMIT(limit)
	query, args := stmt.Sql()
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading claim window for app %d: %w", appID, err)
	}
	defer rows.Close()
	var result []*model.Epoch
	for rows.Next() {
		epoch := new(model.Epoch)
		if err := rows.Scan(&epoch.ApplicationID, &epoch.Index, &epoch.FirstBlock, &epoch.LastBlock,
			&epoch.MachineHash, &epoch.TxBufferDataBlock, &epoch.TxBufferProof,
			&epoch.IflagsYDataBlock, &epoch.IflagsYProof, &epoch.HtifTohostDataBlock, &epoch.HtifTohostProof,
			&epoch.ClaimTransactionHash, &epoch.Status, &epoch.StagedAtBlock, &epoch.VirtualIndex,
			&epoch.CreatedAt, &epoch.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, epoch)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) SelectClaimsToSubmitPage(
	ctx context.Context, cursor repository.ClaimCursor, limit uint64,
) (repository.ClaimPage, error) {
	return r.selectClaimsPage(ctx, cursor, limit, model.EpochStatus_ClaimComputed)
}

func (r *PostgresRepository) SelectClaimsToStagePage(
	ctx context.Context, cursor repository.ClaimCursor, limit uint64,
) (repository.ClaimPage, error) {
	return r.selectClaimsPage(ctx, cursor, limit, model.EpochStatus_ClaimSubmitted)
}

func (r *PostgresRepository) SelectClaimsToAcceptPage(
	ctx context.Context, cursor repository.ClaimCursor, limit uint64,
) (repository.ClaimPage, error) {
	return r.selectClaimsPage(ctx, cursor, limit, model.EpochStatus_ClaimStaged)
}
