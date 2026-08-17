// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func getEpochNextVirtualIndex(
	ctx context.Context,
	tx pgx.Tx,
	nameOrAddress string,
) (uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	query := table.Epoch.SELECT(
		postgres.COALESCE(
			postgres.Float(1).ADD(postgres.MAXf(table.Epoch.VirtualIndex)),
			postgres.Float(0),
		),
	).FROM(
		table.Epoch.INNER_JOIN(table.Application, table.Epoch.ApplicationID.EQ(table.Application.ID)),
	).WHERE(
		whereClause,
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get the next epoch virtual index: %w", err)
	}
	return currentIndex, nil
}

func orderEpochs(epochInputsMap map[*model.Epoch][]*model.Input) []*model.Epoch {
	epochs := make([]*model.Epoch, 0, len(epochInputsMap))
	for e := range epochInputsMap {
		epochs = append(epochs, e)
	}

	sort.Slice(epochs, func(i, j int) bool {
		return epochs[i].FirstBlock < epochs[j].FirstBlock
	})

	return epochs
}

func (r *PostgresRepository) CreateEpochsAndInputs(
	ctx context.Context,
	nameOrAddress string,
	epochInputsMap map[*model.Epoch][]*model.Input,
	blockNumber uint64,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	epochInsertStmt := table.Epoch.INSERT(
		table.Epoch.ApplicationID,
		table.Epoch.Index,
		table.Epoch.FirstBlock,
		table.Epoch.LastBlock,
		table.Epoch.InputIndexLowerBound,
		table.Epoch.InputIndexUpperBound,
		table.Epoch.TournamentAddress,
		table.Epoch.Status,
		table.Epoch.VirtualIndex,
	)

	inputInsertStmt := table.Input.
		INSERT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.TransactionHash,
			table.Input.LogIndex,
		)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Participate in the same Application -> child-row order as
	// StoreAdvanceResult and RejectEpochAndSetApplicationDiverged. SELECT FOR NO
	// KEY UPDATE does not change the application row; it takes a
	// transaction-scoped row lock that conflicts with application updates. L1
	// ingestion holds it while upserting epochs and inputs and advancing the
	// input-scan cursor, so another multi-row writer for this application waits
	// before touching a child row instead of forming a child -> Application
	// deadlock cycle.
	//
	// Commit or rollback releases the lock. A lost node connection aborts the
	// transaction once PostgreSQL detects it (which a network failure can
	// delay), and a PostgreSQL restart discards the uncommitted transaction
	// during crash recovery. The lock is not persisted independently.
	appLockStmt := table.Application.
		SELECT(table.Application.ID).
		WHERE(whereClause).
		FOR(postgres.NO_KEY_UPDATE())
	appLockSQL, appLockArgs := appLockStmt.Sql()
	var appID int64
	if err := tx.QueryRow(ctx, appLockSQL, appLockArgs...).Scan(&appID); errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return err
	}

	epochs := orderEpochs(epochInputsMap)
	for _, epoch := range epochs {
		inputs := epochInputsMap[epoch]

		nextVirtualIndex, err := getEpochNextVirtualIndex(ctx, tx, nameOrAddress)
		if err != nil {
			return err
		}

		var tournamentAddress postgres.ByteaExpression
		if epoch.TournamentAddress != nil {
			tournamentAddress = postgres.Bytea(epoch.TournamentAddress.Bytes())
		} else {
			tournamentAddress = postgres.ByteaExp(postgres.NULL)
		}
		epochSelectQuery := table.Application.SELECT(
			table.Application.ID,
			uint64Expr(epoch.Index),
			uint64Expr(epoch.FirstBlock),
			uint64Expr(epoch.LastBlock),
			uint64Expr(epoch.InputIndexLowerBound),
			uint64Expr(epoch.InputIndexUpperBound),
			tournamentAddress,
			postgres.NewEnumValue(epoch.Status.String()),
			uint64Expr(nextVirtualIndex),
		).WHERE(
			whereClause,
		)

		// Guard: only update epoch fields when the existing row is still OPEN.
		// Once an epoch is sealed (CLOSED) or beyond, its status, block range,
		// input bounds, and tournament address are finalized and must not be
		// overwritten by crash-recovery re-processing.
		isOpen := table.Epoch.Status.EQ(
			postgres.NewEnumValue(model.EpochStatus_Open.String()),
		)

		sqlStr, args := epochInsertStmt.QUERY(epochSelectQuery).
			ON_CONFLICT(table.Epoch.ApplicationID, table.Epoch.Index).
			DO_UPDATE(postgres.SET(
				table.Epoch.Status.SET(postgres.StringExp(
					postgres.CASE().
						WHEN(isOpen).THEN(table.Epoch.EXCLUDED.Status).
						ELSE(table.Epoch.Status),
				)),
				table.Epoch.LastBlock.SET(postgres.FloatExp(
					postgres.CASE().
						WHEN(isOpen).THEN(table.Epoch.EXCLUDED.LastBlock).
						ELSE(table.Epoch.LastBlock),
				)),
				table.Epoch.InputIndexUpperBound.SET(postgres.FloatExp(
					postgres.CASE().
						WHEN(isOpen).THEN(table.Epoch.EXCLUDED.InputIndexUpperBound).
						ELSE(table.Epoch.InputIndexUpperBound),
				)),
				table.Epoch.TournamentAddress.SET(postgres.ByteaExp(
					postgres.CASE().
						WHEN(isOpen).THEN(table.Epoch.EXCLUDED.TournamentAddress).
						ELSE(table.Epoch.TournamentAddress),
				)),
			)).Sql()
		_, err = tx.Exec(ctx, sqlStr, args...)

		if err != nil {
			return err
		}

		if len(inputs) > 0 {
			batch := &pgx.Batch{}
			for _, input := range inputs {
				inputSelectQuery := table.Application.SELECT(
					table.Application.ID,
					uint64Expr(epoch.Index),
					uint64Expr(input.Index),
					uint64Expr(input.BlockNumber),
					postgres.Bytea(input.RawData),
					postgres.NewEnumValue(input.Status.String()),
					postgres.Bytea(input.TransactionHash.Bytes()),
					uint64Expr(input.LogIndex),
				).WHERE(
					whereClause,
				)

				sqlStr, args := inputInsertStmt.QUERY(inputSelectQuery).
					ON_CONFLICT(table.Input.EpochApplicationID, table.Input.Index).
					DO_NOTHING().Sql()
				batch.Queue(sqlStr, args...)
			}

			br := tx.SendBatch(ctx, batch)
			for range inputs {
				_, err := br.Exec()
				if err != nil {
					br.Close()
					return wrapInputLogIdentityConflict(err)
				}
			}
			if err := br.Close(); err != nil {
				return wrapInputLogIdentityConflict(err)
			}
		}
	}

	// Update last processed block
	appUpdateStmt := table.Application.
		UPDATE(
			table.Application.LastInputCheckBlock,
		).
		SET(
			uint64Expr(blockNumber),
		).
		WHERE(whereClause)

	sqlStr, args := appUpdateStmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// sqlstateUniqueViolation is the PostgreSQL error code for unique constraint
// violations (SQLSTATE 23505).
const sqlstateUniqueViolation = "23505"

// inputLogIdentityConstraint is the unique constraint enforcing the L1 log
// identity of an input; see the input table in the initial schema migration.
const inputLogIdentityConstraint = "input_application_id_tx_hash_log_index_unique"

// wrapInputLogIdentityConflict tags unique violations of the input L1 log
// identity constraint with repository.ErrInputLogIdentityConflict so callers
// can distinguish stored-state divergence (unrecoverable by retry) from
// transient insert failures. Conflicts on the input primary key never reach
// this point — they are absorbed by the insert's ON CONFLICT DO NOTHING.
func wrapInputLogIdentityConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == sqlstateUniqueViolation &&
		pgErr.ConstraintName == inputLogIdentityConstraint {
		return fmt.Errorf("%w: %w", repository.ErrInputLogIdentityConflict, err)
	}
	return err
}

func (r *PostgresRepository) GetEpoch(
	ctx context.Context,
	nameOrAddress string,
	index uint64,
) (*model.Epoch, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	stmt := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.InputIndexLowerBound,
			table.Epoch.InputIndexUpperBound,
			table.Epoch.MachineHash,
			table.Epoch.TxBufferDataBlock,
			table.Epoch.TxBufferProof,
			table.Epoch.IflagsYDataBlock,
			table.Epoch.IflagsYProof,
			table.Epoch.HtifTohostDataBlock,
			table.Epoch.HtifTohostProof,
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
			table.Epoch.VirtualIndex,
			table.Epoch.CreatedAt,
			table.Epoch.UpdatedAt,
		).
		FROM(
			table.Epoch.
				INNER_JOIN(table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Epoch.Index.EQ(uint64Expr(index))),
		)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var ep model.Epoch
	err := row.Scan(
		&ep.ApplicationID,
		&ep.Index,
		&ep.FirstBlock,
		&ep.LastBlock,
		&ep.InputIndexLowerBound,
		&ep.InputIndexUpperBound,
		&ep.MachineHash,
		&ep.TxBufferDataBlock,
		&ep.TxBufferProof,
		&ep.IflagsYDataBlock,
		&ep.IflagsYProof,
		&ep.HtifTohostDataBlock,
		&ep.HtifTohostProof,
		&ep.Commitment,
		&ep.CommitmentProof,
		&ep.ClaimTransactionHash,
		&ep.TournamentAddress,
		&ep.Status,
		&ep.StagedAtBlock,
		&ep.VirtualIndex,
		&ep.CreatedAt,
		&ep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// HasUndrainedEpochsBeforeBlock returns true while any input belonging to
// appID has block_number <= blockBound and is still status='NONE' (i.e. not
// yet advanced by the machine). PRT uses this to keep its post-foreclosure
// drain pending until all pre-foreclosure inputs have been advanced.
//
// The check is input-level rather than epoch-level for two reasons:
//
//  1. It naturally catches the "straddling open epoch" case: an epoch with
//     first_block < blockBound but last_block >= blockBound still contains
//     pre-foreclosure inputs that must be processed before drain can
//     complete. A predicate on epoch.last_block < blockBound would skip
//     such an epoch.
//  2. It correctly tolerates PRT's empty-epoch invariant — an empty open
//     epoch straddling the foreclosure block has no inputs to wait on, so
//     the gate returns false (whereas a predicate on
//     epoch.first_block <= blockBound would incorrectly stall PRT drain on
//     the empty straddler).
//
// The block bound is inclusive because any valid InputAdded event in the
// Foreclosure block must have executed before Foreclosure; a later same-block
// addInput call would revert and emit no event.
//
// Authority/Quorum also uses the broader
// [PostgresRepository.HasUnreconciledClaimsBeforeBlock] gate so it waits for
// read-only claim reconciliation or CLAIM_FORECLOSED terminalization.
func (r *PostgresRepository) HasUndrainedEpochsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	terminalStatuses := []postgres.Expression{
		enum.EpochStatus.ClaimAccepted,
		enum.EpochStatus.ClaimRejected,
		enum.EpochStatus.ClaimForeclosed,
	}
	stmt := table.Input.
		SELECT(table.Input.Index).
		FROM(
			table.Input.INNER_JOIN(table.Epoch,
				table.Input.EpochApplicationID.EQ(table.Epoch.ApplicationID).
					AND(table.Input.EpochIndex.EQ(table.Epoch.Index)),
			),
		).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int(appID)).
				AND(table.Input.BlockNumber.LT_EQ(uint64Expr(blockBound))).
				AND(table.Input.Status.EQ(enum.InputCompletionStatus.None)).
				AND(table.Epoch.Status.NOT_IN(terminalStatuses...)),
		).
		LIMIT(1)

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// ForecloseUnacceptedEpochsAtOrAfterBlock makes local Authority/Quorum epoch
// rows terminal when their claim cannot be accepted because the application was
// foreclosed before or at the epoch's last block. It leaves earlier epochs alone
// so the claimer can still reconcile claims accepted before foreclosure.
func (r *PostgresRepository) ForecloseUnacceptedEpochsAtOrAfterBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (int64, error) {
	statuses := []postgres.Expression{
		enum.EpochStatus.Open,
		enum.EpochStatus.Closed,
		enum.EpochStatus.InputsProcessed,
		enum.EpochStatus.ClaimComputed,
		enum.EpochStatus.ClaimSubmitted,
		enum.EpochStatus.ClaimStaged,
	}
	updateStmt := table.Epoch.
		UPDATE(table.Epoch.Status).
		SET(enum.EpochStatus.ClaimForeclosed).
		FROM(table.Application).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Epoch.FirstBlock.LT_EQ(uint64Expr(blockBound))).
				AND(table.Epoch.LastBlock.GT_EQ(uint64Expr(blockBound))).
				AND(table.Epoch.Status.IN(statuses...)).
				AND(table.Application.ID.EQ(table.Epoch.ApplicationID)).
				AND(table.Application.ForecloseBlock.GT(uint64Expr(0))).
				AND(table.Application.ConsensusType.NOT_EQ(enum.Consensus.Prt)),
		)

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return 0, fmt.Errorf("foreclosing unaccepted epochs (app=%d, block=%d): %w", appID, blockBound, err)
	}
	return cmd.RowsAffected(), nil
}

// HasUnreconciledClaimsBeforeBlock returns true while any epoch for appID
// has first_block <= blockBound AND status in OPEN/CLOSED/INPUTS_PROCESSED or
// CLAIM_COMPUTED/CLAIM_SUBMITTED/CLAIM_STAGED. The extra states ensure the
// Authority/Quorum claimer's foreclosure drain waits for the read-only
// CLAIM_COMPUTED → CLAIM_ACCEPTED reconciliation path, or the
// CLAIM_* → CLAIM_FORECLOSED terminalization path, to finish. Otherwise a
// new-node bootstrap against an already-foreclosed app could drain before
// mirroring pre-foreclosure on-chain state into the local DB.
//
// The predicate is `first_block <= blockBound` (not `last_block < blockBound`)
// to catch straddling epochs: an epoch that started before the foreclosure
// block but extends past it is still pre-foreclosure work the claimer must
// drive to CLAIM_ACCEPTED or CLAIM_FORECLOSED. The inclusive bound catches a
// valid same-block input that executed before Foreclosure. Authority/Quorum
// never creates empty epoch rows, so `first_block <= blockBound` does not
// introduce false positives.
// unreconciledEpochStatuses are the epoch statuses that still need claim work —
// every EpochStatus except the terminal CLAIM_ACCEPTED / CLAIM_REJECTED /
// CLAIM_FORECLOSED. It drives HasUnreconciledClaimsBeforeBlock's filter and
// MUST stay in sync with the partial-index predicate of "epoch_unreconciled_idx"
// in 000001_create_initial_schema.up.sql; if the two drift, the query silently
// stops matching the index and falls back to a full table scan.
// TestUnreconciledEpochStatusesAreNonTerminal guards this set when a new
// EpochStatus is added.
var unreconciledEpochStatuses = []model.EpochStatus{
	model.EpochStatus_Open,
	model.EpochStatus_Closed,
	model.EpochStatus_InputsProcessed,
	model.EpochStatus_ClaimComputed,
	model.EpochStatus_ClaimSubmitted,
	model.EpochStatus_ClaimStaged,
}

func (r *PostgresRepository) HasUnreconciledClaimsBeforeBlock(
	ctx context.Context,
	appID int64,
	blockBound uint64,
) (bool, error) {
	statuses := make([]postgres.Expression, len(unreconciledEpochStatuses))
	for i, s := range unreconciledEpochStatuses {
		statuses[i] = postgres.NewEnumValue(string(s))
	}
	stmt := table.Epoch.
		SELECT(table.Epoch.Index).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int(appID)).
				AND(table.Epoch.FirstBlock.LT_EQ(uint64Expr(blockBound))).
				AND(table.Epoch.Status.IN(statuses...)),
		).
		LIMIT(1)

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

func (r *PostgresRepository) GetLastAcceptedEpochIndex(
	ctx context.Context,
	nameOrAddress string,
) (uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	stmt := table.Epoch.
		SELECT(
			table.Epoch.Index,
		).
		FROM(
			table.Epoch.
				INNER_JOIN(table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Epoch.Status.EQ(enum.EpochStatus.ClaimAccepted)),
		).
		ORDER_BY(table.Epoch.Index.DESC()).
		LIMIT(1)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var index uint64
	err := row.Scan(
		&index,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return index, nil
}

func (r *PostgresRepository) GetLastNonOpenEpoch(
	ctx context.Context,
	nameOrAddress string,
) (*model.Epoch, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	stmt := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.InputIndexLowerBound,
			table.Epoch.InputIndexUpperBound,
			table.Epoch.MachineHash,
			table.Epoch.TxBufferDataBlock,
			table.Epoch.TxBufferProof,
			table.Epoch.IflagsYDataBlock,
			table.Epoch.IflagsYProof,
			table.Epoch.HtifTohostDataBlock,
			table.Epoch.HtifTohostProof,
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
			table.Epoch.VirtualIndex,
			table.Epoch.CreatedAt,
			table.Epoch.UpdatedAt,
		).
		FROM(
			table.Epoch.
				INNER_JOIN(table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Epoch.Status.NOT_EQ(enum.EpochStatus.Open)),
		).
		ORDER_BY(table.Epoch.Index.DESC()).
		LIMIT(1)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var ep model.Epoch
	err := row.Scan(
		&ep.ApplicationID,
		&ep.Index,
		&ep.FirstBlock,
		&ep.LastBlock,
		&ep.InputIndexLowerBound,
		&ep.InputIndexUpperBound,
		&ep.MachineHash,
		&ep.TxBufferDataBlock,
		&ep.TxBufferProof,
		&ep.IflagsYDataBlock,
		&ep.IflagsYProof,
		&ep.HtifTohostDataBlock,
		&ep.HtifTohostProof,
		&ep.Commitment,
		&ep.CommitmentProof,
		&ep.ClaimTransactionHash,
		&ep.TournamentAddress,
		&ep.Status,
		&ep.StagedAtBlock,
		&ep.VirtualIndex,
		&ep.CreatedAt,
		&ep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

func (r *PostgresRepository) GetEpochByVirtualIndex(
	ctx context.Context,
	nameOrAddress string,
	index uint64,
) (*model.Epoch, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	stmt := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.InputIndexLowerBound,
			table.Epoch.InputIndexUpperBound,
			table.Epoch.MachineHash,
			table.Epoch.TxBufferDataBlock,
			table.Epoch.TxBufferProof,
			table.Epoch.IflagsYDataBlock,
			table.Epoch.IflagsYProof,
			table.Epoch.HtifTohostDataBlock,
			table.Epoch.HtifTohostProof,
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
			table.Epoch.VirtualIndex,
			table.Epoch.CreatedAt,
			table.Epoch.UpdatedAt,
		).
		FROM(
			table.Epoch.
				INNER_JOIN(table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Epoch.VirtualIndex.EQ(uint64Expr(index))),
		)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var ep model.Epoch
	err := row.Scan(
		&ep.ApplicationID,
		&ep.Index,
		&ep.FirstBlock,
		&ep.LastBlock,
		&ep.InputIndexLowerBound,
		&ep.InputIndexUpperBound,
		&ep.MachineHash,
		&ep.TxBufferDataBlock,
		&ep.TxBufferProof,
		&ep.IflagsYDataBlock,
		&ep.IflagsYProof,
		&ep.HtifTohostDataBlock,
		&ep.HtifTohostProof,
		&ep.Commitment,
		&ep.CommitmentProof,
		&ep.ClaimTransactionHash,
		&ep.TournamentAddress,
		&ep.Status,
		&ep.StagedAtBlock,
		&ep.VirtualIndex,
		&ep.CreatedAt,
		&ep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

func (r *PostgresRepository) UpdateEpochClaimTransactionHash(
	ctx context.Context,
	nameOrAddress string,
	e *model.Epoch,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.ClaimTransactionHash,
		).
		SET(
			e.ClaimTransactionHash,
		).
		FROM(
			table.Application,
		).
		WHERE(
			whereClause.
				AND(table.Epoch.ApplicationID.EQ(table.Application.ID)).
				AND(table.Epoch.Index.EQ(uint64Expr(e.Index))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateEpochStatus(
	ctx context.Context,
	nameOrAddress string,
	e *model.Epoch,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.Status,
		).
		SET(
			e.Status,
		).
		FROM(
			table.Application,
		).
		WHERE(
			whereClause.
				AND(table.Epoch.ApplicationID.EQ(table.Application.ID)).
				AND(table.Epoch.Index.EQ(uint64Expr(e.Index))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateEpochInputsProcessed(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
	proof *model.StateProof,
) error {
	if !proof.IsComplete() {
		return repository.ErrInvalidStateProof
	}

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	// Subquery to check if the previous epoch is not open or closed
	prevTable := table.Epoch.AS("prev")
	prevSub := prevTable.SELECT(
		prevTable.Status.NOT_IN(enum.EpochStatus.Open, enum.EpochStatus.Closed),
	).WHERE(postgres.AND(
		prevTable.ApplicationID.EQ(table.Epoch.ApplicationID),
		prevTable.Index.EQ(table.Epoch.Index.SUB(postgres.Int64(1))),
	))

	// Condition using COALESCE for the previous epoch (returns TRUE if no previous epoch exists)
	prevCondition := postgres.BoolExp(postgres.COALESCE(prevSub, postgres.Bool(true)))

	// Condition for inputs: either no inputs expected or all inputs are present and processed
	hasNoInputs := table.Epoch.InputIndexUpperBound.EQ(table.Epoch.InputIndexLowerBound)

	// Subquery to count total inputs for the epoch
	totalInputsSub := postgres.FloatExp(table.Input.SELECT(postgres.COUNT(postgres.STAR)).
		WHERE(postgres.AND(
			table.Input.EpochApplicationID.EQ(table.Epoch.ApplicationID),
			table.Input.EpochIndex.EQ(table.Epoch.Index),
		)))

	// Subquery to count pending inputs (status = 'None')
	pendingInputsSub := postgres.IntExp(table.Input.SELECT(postgres.COUNT(postgres.STAR)).
		WHERE(postgres.AND(
			table.Input.EpochApplicationID.EQ(table.Epoch.ApplicationID),
			table.Input.EpochIndex.EQ(table.Epoch.Index),
			table.Input.Status.EQ(enum.InputCompletionStatus.None),
		)))

	allInputsPresentAndProcessed := postgres.AND(
		totalInputsSub.EQ(table.Epoch.InputIndexUpperBound.SUB(table.Epoch.InputIndexLowerBound)),
		pendingInputsSub.EQ(postgres.Int64(0)),
	)

	inputsCondition := hasNoInputs.OR(allInputsPresentAndProcessed)

	// Publish the final state proof and status atomically. Readers can never
	// observe INPUTS_PROCESSED without all three canonical proof leaves.
	updateStmt := table.Epoch.UPDATE(
		table.Epoch.Status,
		table.Epoch.TxBufferDataBlock,
		table.Epoch.TxBufferProof,
		table.Epoch.MachineHash,
		table.Epoch.IflagsYDataBlock,
		table.Epoch.IflagsYProof,
		table.Epoch.HtifTohostDataBlock,
		table.Epoch.HtifTohostProof,
	).
		SET(
			enum.EpochStatus.InputsProcessed,
			proof.TxBufferDataBlock[:],
			encodeSiblings(proof.TxBufferProof),
			proof.MachineHash[:],
			proof.IflagsYDataBlock[:],
			encodeSiblings(proof.IflagsYProof),
			proof.HtifTohostDataBlock[:],
			encodeSiblings(proof.HtifTohostProof),
		).
		FROM(table.Application).
		WHERE(postgres.AND(
			table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_Closed.String())),
			table.Epoch.ApplicationID.EQ(table.Application.ID),
			table.Application.Status.EQ(enum.ApplicationStatus.Ok),
			table.Epoch.Index.EQ(uint64Expr(epochIndex)),
			whereClause,
			prevCondition,
			inputsCondition,
		)).
		RETURNING(table.Epoch.Index)

	// Execute the update and capture the returned indexes
	sqlStr, args := updateStmt.Sql()

	var index uint64
	err := r.db.QueryRow(ctx, sqlStr, args...).Scan(&index)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.ErrNoUpdate
		}
		return err
	}
	if index != epochIndex {
		// should not happen
		return fmt.Errorf("updated epoch index mismatch: expected %d, got %d", epochIndex, index)
	}
	return nil
}

func (r *PostgresRepository) ListEpochs(
	ctx context.Context,
	nameOrAddress string,
	f repository.EpochFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Epoch, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.Epoch.
		INNER_JOIN(table.Application,
			table.Epoch.ApplicationID.EQ(table.Application.ID),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.IndexRange != nil {
		conditions = append(conditions,
			table.Epoch.Index.GT_EQ(uint64Expr(f.IndexRange.Start)),
			table.Epoch.Index.LT_EQ(uint64Expr(f.IndexRange.End)),
		)
	}
	if len(f.Status) > 0 {
		statuses := make([]postgres.Expression, 0, len(f.Status))
		for _, status := range f.Status {
			statuses = append(statuses, postgres.NewEnumValue(status.String()))
		}
		conditions = append(conditions, table.Epoch.Status.IN(statuses...))
	}

	if f.BeforeBlock != nil {
		conditions = append(conditions, table.Epoch.LastBlock.LT(uint64Expr(*f.BeforeBlock)))
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.Epoch.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.InputIndexLowerBound,
			table.Epoch.InputIndexUpperBound,
			table.Epoch.MachineHash,
			table.Epoch.TxBufferDataBlock,
			table.Epoch.IflagsYDataBlock,
			table.Epoch.HtifTohostDataBlock,
			table.Epoch.Commitment,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
			table.Epoch.StagedAtBlock,
			table.Epoch.VirtualIndex,
			table.Epoch.CreatedAt,
			table.Epoch.UpdatedAt,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.Epoch.Index.DESC())
	} else {
		sel = sel.ORDER_BY(table.Epoch.Index.ASC())
	}

	// pagination
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

	var epochs []*model.Epoch
	for rows.Next() {
		var ep model.Epoch
		err := rows.Scan(
			&ep.ApplicationID,
			&ep.Index,
			&ep.FirstBlock,
			&ep.LastBlock,
			&ep.InputIndexLowerBound,
			&ep.InputIndexUpperBound,
			&ep.MachineHash,
			&ep.TxBufferDataBlock,
			&ep.IflagsYDataBlock,
			&ep.HtifTohostDataBlock,
			&ep.Commitment,
			&ep.ClaimTransactionHash,
			&ep.TournamentAddress,
			&ep.Status,
			&ep.StagedAtBlock,
			&ep.VirtualIndex,
			&ep.CreatedAt,
			&ep.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		epochs = append(epochs, &ep)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return epochs, total, nil
}
