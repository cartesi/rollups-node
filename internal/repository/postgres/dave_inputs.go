// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

// lockApplicationRow takes the application row lock shared by every multi-row
// writer for an application. It keeps child-row writes in the same
// Application -> child order, avoiding deadlocks. The transaction owns the
// lock until commit or rollback.
func lockApplicationRow(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
) (int64, error) {
	appLockStmt := table.Application.
		SELECT(table.Application.ID).
		WHERE(whereClause).
		FOR(postgres.NO_KEY_UPDATE())
	appLockSQL, appLockArgs := appLockStmt.Sql()
	var appID int64
	if err := tx.QueryRow(ctx, appLockSQL, appLockArgs...).Scan(&appID); errors.Is(err, pgx.ErrNoRows) {
		return 0, repository.ErrNotFound
	} else if err != nil {
		return 0, err
	}
	return appID, nil
}

// upsertEpoch stores one epoch. Once the stored epoch is no longer OPEN, its
// status, block range, input bounds, and tournament address are finalized and
// are preserved; only a crash-recovery re-processing of an OPEN epoch updates
// them.
func upsertEpoch(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
	nameOrAddress string,
	epoch *model.Epoch,
) error {
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

	isOpen := table.Epoch.Status.EQ(
		postgres.NewEnumValue(model.EpochStatus_Open.String()),
	)

	stmt := table.Epoch.INSERT(
		table.Epoch.ApplicationID,
		table.Epoch.Index,
		table.Epoch.FirstBlock,
		table.Epoch.LastBlock,
		table.Epoch.InputIndexLowerBound,
		table.Epoch.InputIndexUpperBound,
		table.Epoch.TournamentAddress,
		table.Epoch.Status,
		table.Epoch.VirtualIndex,
	).QUERY(epochSelectQuery).
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
		))

	sqlStr, args := stmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	return err
}

// insertEpochInputs stores inputs without checking whether an existing row
// under the same index belongs to another epoch. CreateEpochsAndInputs keeps
// its historical behavior on conflict; Dave attribution uses
// insertAttributedInputs, which verifies the identity.
func insertEpochInputs(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
	epoch *model.Epoch,
	inputs []*model.Input,
) error {
	return insertInputs(ctx, tx, whereClause, epoch, inputs, false)
}

// insertAttributedInputs stores inputs for an epoch and verifies that an
// existing row under the same index has the same epoch and L1 log identity.
// The input primary key is global per application, so a silent DO NOTHING on a
// row owned by another epoch would leave execution looking at the wrong range.
func insertAttributedInputs(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
	epoch *model.Epoch,
	inputs []*model.Input,
) error {
	return insertInputs(ctx, tx, whereClause, epoch, inputs, true)
}

func insertInputs(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
	epoch *model.Epoch,
	inputs []*model.Input,
	verifyIdentity bool,
) error {
	if len(inputs) == 0 {
		return nil
	}

	stmt := table.Input.
		INSERT(
			table.Input.EpochApplicationID,
			table.Input.EpochIndex,
			table.Input.Index,
			table.Input.BlockNumber,
			table.Input.RawData,
			table.Input.Status,
			table.Input.ExceptionData,
			table.Input.MachineHash,
			table.Input.TxBufferDataBlock,
			table.Input.TransactionHash,
			table.Input.LogIndex,
		)

	batch := &pgx.Batch{}
	for _, input := range inputs {
		inputSelectQuery := table.Application.SELECT(
			table.Application.ID,
			uint64Expr(epoch.Index),
			uint64Expr(input.Index),
			uint64Expr(input.BlockNumber),
			postgres.Bytea(input.RawData),
			postgres.NewEnumValue(input.Status.String()),
			byteaOrNull(input.ExceptionData),
			hashByteaOrNull(input.MachineHash),
			hashByteaOrNull(input.TxBufferDataBlock),
			postgres.Bytea(input.TransactionHash.Bytes()),
			uint64Expr(input.LogIndex),
		).WHERE(
			whereClause,
		)

		sqlStr, args := stmt.QUERY(inputSelectQuery).
			ON_CONFLICT(table.Input.EpochApplicationID, table.Input.Index).
			DO_NOTHING().Sql()
		batch.Queue(sqlStr, args...)
	}

	br := tx.SendBatch(ctx, batch)
	var existingRows []*model.Input
	for _, input := range inputs {
		tag, err := br.Exec()
		if err != nil {
			br.Close()
			return wrapInputLogIdentityConflict(err)
		}
		if verifyIdentity && tag.RowsAffected() == 0 {
			existingRows = append(existingRows, input)
		}
	}
	if err := br.Close(); err != nil {
		return wrapInputLogIdentityConflict(err)
	}

	// The batch result must be closed before the transaction can run another
	// query. A row that already existed is benign only when it is the same
	// input under the same epoch. Verify them set-based instead of issuing one
	// query per existing row; a no-op re-projection must not scale with the
	// epoch size.
	return verifyInputIdentitiesTx(ctx, tx, epoch.ApplicationID, epoch.Index, existingRows)
}

type inputIdentity struct {
	epochIndex      uint64
	transactionHash [32]byte
	logIndex        uint64
}

// inputIdentityChunkSize keeps the IN list well below the Postgres parameter
// limit while amortizing the round trips.
const inputIdentityChunkSize = 500

func verifyInputIdentitiesTx(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
	inputs []*model.Input,
) error {
	for start := 0; start < len(inputs); start += inputIdentityChunkSize {
		chunk := inputs[start:min(start+inputIdentityChunkSize, len(inputs))]
		indices := make([]postgres.Expression, len(chunk))
		for i, input := range chunk {
			indices[i] = uint64Expr(input.Index)
		}
		stmt := table.Input.SELECT(
			table.Input.Index,
			table.Input.EpochIndex,
			table.Input.TransactionHash,
			table.Input.LogIndex,
		).WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Input.Index.IN(indices...)),
		)
		sqlStr, args := stmt.Sql()
		rows, err := tx.Query(ctx, sqlStr, args...)
		if err != nil {
			return err
		}
		found := make(map[uint64]inputIdentity, len(chunk))
		for rows.Next() {
			var index uint64
			var identity inputIdentity
			var transactionHash []byte
			if err := rows.Scan(&index, &identity.epochIndex,
				&transactionHash, &identity.logIndex); err != nil {
				rows.Close()
				return err
			}
			copy(identity.transactionHash[:], transactionHash)
			found[index] = identity
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, input := range chunk {
			existing, ok := found[input.Index]
			if !ok ||
				existing.epochIndex != epochIndex ||
				existing.transactionHash != input.TransactionHash ||
				existing.logIndex != input.LogIndex {
				return fmt.Errorf("%w: input %d of epoch %d",
					repository.ErrInputLogIdentityConflict, input.Index, epochIndex)
			}
		}
	}
	return nil
}

func deleteObservedInputs(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	lower, upper uint64,
) error {
	if upper <= lower {
		return nil
	}
	stmt := table.ObservedInput.DELETE().WHERE(
		table.ObservedInput.ApplicationID.EQ(postgres.Int64(appID)).
			AND(table.ObservedInput.Index.GT_EQ(uint64Expr(lower))).
			AND(table.ObservedInput.Index.LT(uint64Expr(upper))),
	)
	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

// advanceInputCheckBlock advances the input cursor monotonically. A Dave
// sealed-epoch catch-up may revisit an older block after open-epoch ingestion
// already scanned farther ahead; that historical write must not rewind drain
// readiness.
func advanceInputCheckBlock(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
	blockNumber uint64,
) error {
	stmt := table.Application.
		UPDATE(
			table.Application.LastInputCheckBlock,
		).
		SET(
			uint64Expr(blockNumber),
		).
		WHERE(whereClause.AND(
			table.Application.LastInputCheckBlock.LT(uint64Expr(blockNumber)),
		))
	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

func validateContiguousInputs(
	lower, upper, blockBound uint64,
	inputs []*model.Input,
) error {
	if upper < lower || uint64(len(inputs)) != upper-lower {
		return fmt.Errorf("%w: expected %d inputs in [%d, %d), got %d",
			repository.ErrEpochInputRangeConflict,
			upper-lower, lower, upper, len(inputs))
	}
	for i, input := range inputs {
		if input == nil {
			return fmt.Errorf("%w: input at position %d is nil",
				repository.ErrEpochInputRangeConflict, i)
		}
		if input.Index != lower+uint64(i) {
			return fmt.Errorf("%w: expected index %d at position %d, got %d",
				repository.ErrEpochInputRangeConflict,
				lower+uint64(i), i, input.Index)
		}
		if input.BlockNumber > blockBound {
			return fmt.Errorf("%w: input %d is at block %d, after bound %d",
				repository.ErrEpochInputRangeConflict,
				input.Index, input.BlockNumber, blockBound)
		}
	}
	return nil
}

func storedEpochMatchesRange(stored, epoch *model.Epoch) bool {
	return stored != nil &&
		stored.InputIndexLowerBound == epoch.InputIndexLowerBound &&
		stored.InputIndexUpperBound == epoch.InputIndexUpperBound &&
		stored.LastBlock == epoch.LastBlock
}

func (r *PostgresRepository) AttributeSealedEpoch(
	ctx context.Context,
	nameOrAddress string,
	epoch *model.Epoch,
	inputs []*model.Input,
) error {
	if epoch == nil {
		return errors.New("cannot attribute a nil epoch")
	}
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	appID, err := lockApplicationRow(ctx, tx, whereClause)
	if err != nil {
		return err
	}
	if err := upsertEpoch(ctx, tx, whereClause, nameOrAddress, epoch); err != nil {
		return err
	}
	stored, err := getEpoch(ctx, tx, nameOrAddress, epoch.Index)
	if err != nil {
		return err
	}
	if stored == nil {
		return repository.ErrNotFound
	}
	if !storedEpochMatchesRange(stored, epoch) {
		return fmt.Errorf("%w: stored epoch %d has [%d, %d) at block %d, got [%d, %d) at block %d",
			repository.ErrEpochInputRangeConflict, epoch.Index,
			stored.InputIndexLowerBound, stored.InputIndexUpperBound, stored.LastBlock,
			epoch.InputIndexLowerBound, epoch.InputIndexUpperBound, epoch.LastBlock)
	}
	if err := validateContiguousInputs(stored.InputIndexLowerBound,
		stored.InputIndexUpperBound, stored.LastBlock, inputs); err != nil {
		return err
	}
	epoch.ApplicationID = appID
	if err := insertAttributedInputs(ctx, tx, whereClause, epoch, inputs); err != nil {
		return err
	}
	// A pre-seal OPEN projection may have stored inputs beyond the sealed
	// range. They have not executed yet (a terminal row is rejected below), so
	// return them to the observed set for a later epoch instead of leaving them
	// attached to this one.
	if err := detachSurplusInputs(ctx, tx, whereClause, appID, stored); err != nil {
		return err
	}
	if err := deleteObservedInputs(ctx, tx, appID,
		stored.InputIndexLowerBound, stored.InputIndexUpperBound); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func detachSurplusInputs(
	ctx context.Context,
	tx pgx.Tx,
	whereClause postgres.BoolExpression,
	appID int64,
	epoch *model.Epoch,
) error {
	stmt := table.Input.SELECT(
		table.Input.Index,
		table.Input.BlockNumber,
		table.Input.RawData,
		table.Input.Status,
		table.Input.TransactionHash,
		table.Input.LogIndex,
	).FROM(
		table.Input.INNER_JOIN(table.Application,
			table.Input.EpochApplicationID.EQ(table.Application.ID)),
	).WHERE(
		whereClause.
			AND(table.Input.EpochIndex.EQ(uint64Expr(epoch.Index))).
			AND(table.Input.Index.GT_EQ(uint64Expr(epoch.InputIndexUpperBound))),
	).ORDER_BY(
		table.Input.Index.ASC(),
	)
	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	surplus := make([]*model.Input, 0)
	for rows.Next() {
		input := &model.Input{EpochIndex: epoch.Index}
		var transactionHash []byte
		if err := rows.Scan(
			&input.Index,
			&input.BlockNumber,
			&input.RawData,
			&input.Status,
			&transactionHash,
			&input.LogIndex,
		); err != nil {
			rows.Close()
			return err
		}
		copy(input.TransactionHash[:], transactionHash)
		surplus = append(surplus, input)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(surplus) == 0 {
		return nil
	}
	for _, input := range surplus {
		if input.Status != model.InputCompletionStatus_None {
			return fmt.Errorf("%w: executed input %d is outside sealed epoch %d [%d, %d); operator reset required",
				repository.ErrEpochInputRangeConflict,
				input.Index, epoch.Index,
				epoch.InputIndexLowerBound, epoch.InputIndexUpperBound)
		}
	}
	if err := insertObservedInputs(ctx, tx, appID, surplus); err != nil {
		return err
	}
	deleteStmt := table.Input.DELETE().WHERE(
		table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.Input.EpochIndex.EQ(uint64Expr(epoch.Index))).
			AND(table.Input.Index.GT_EQ(uint64Expr(epoch.InputIndexUpperBound))),
	)
	deleteSQL, deleteArgs := deleteStmt.Sql()
	_, err = tx.Exec(ctx, deleteSQL, deleteArgs...)
	return err
}

func (r *PostgresRepository) ProjectDaveOpenInputs(
	ctx context.Context,
	nameOrAddress string,
	epoch *model.Epoch,
	inputs []*model.Input,
	throughBlock uint64,
) error {
	if epoch == nil {
		return errors.New("cannot project a nil epoch")
	}
	if epoch.LastBlock != throughBlock {
		return fmt.Errorf("%w: epoch %d last block is %d, projection is through %d",
			repository.ErrEpochInputRangeConflict, epoch.Index, epoch.LastBlock, throughBlock)
	}
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	appID, err := lockApplicationRow(ctx, tx, whereClause)
	if err != nil {
		return err
	}

	// Callers send only the newly observed prefix, so the stored upper bound
	// read under the application lock is the lower bound of the delta. The
	// first projection of an epoch has no stored row and starts at the epoch's
	// lower bound.
	stored, err := getEpoch(ctx, tx, nameOrAddress, epoch.Index)
	if err != nil {
		return err
	}
	previousUpper := epoch.InputIndexLowerBound
	if stored != nil {
		if stored.Status != model.EpochStatus_Open {
			return fmt.Errorf("%w: epoch %d is %s, not OPEN",
				repository.ErrNoUpdate, epoch.Index, stored.Status)
		}
		if stored.InputIndexLowerBound != epoch.InputIndexLowerBound {
			return fmt.Errorf("%w: stored epoch %d lower bound is %d, got %d",
				repository.ErrEpochInputRangeConflict, epoch.Index,
				stored.InputIndexLowerBound, epoch.InputIndexLowerBound)
		}
		if epoch.InputIndexUpperBound < stored.InputIndexUpperBound {
			return fmt.Errorf("%w: stored epoch %d upper bound is %d, projection would narrow it to %d",
				repository.ErrEpochInputRangeConflict, epoch.Index,
				stored.InputIndexUpperBound, epoch.InputIndexUpperBound)
		}
		if stored.LastBlock > throughBlock {
			return fmt.Errorf("%w: stored epoch %d last block is %d, projection is through %d",
				repository.ErrEpochInputRangeConflict, epoch.Index,
				stored.LastBlock, throughBlock)
		}
		previousUpper = stored.InputIndexUpperBound
	}
	if err := validateContiguousInputs(previousUpper,
		epoch.InputIndexUpperBound, throughBlock, inputs); err != nil {
		return err
	}

	if err := upsertEpoch(ctx, tx, whereClause, nameOrAddress, epoch); err != nil {
		return err
	}
	epoch.ApplicationID = appID
	if err := insertAttributedInputs(ctx, tx, whereClause, epoch, inputs); err != nil {
		return err
	}
	if err := detachSurplusInputs(ctx, tx, whereClause, appID, epoch); err != nil {
		return err
	}
	if err := deleteObservedInputs(ctx, tx, appID,
		epoch.InputIndexLowerBound, epoch.InputIndexUpperBound); err != nil {
		return err
	}
	if err := advanceInputCheckBlock(ctx, tx, whereClause, throughBlock); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Observation
// ---------------------------------------------------------------------------

func (r *PostgresRepository) ObserveDaveInputs(
	ctx context.Context,
	nameOrAddress string,
	throughBlock, nextInputIndex uint64,
	inputs []*model.Input,
) error {
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	appID, err := lockApplicationRow(ctx, tx, whereClause)
	if err != nil {
		return err
	}
	current, err := getInputObservationTx(ctx, tx, appID)
	if err != nil {
		return err
	}
	if current != nil {
		switch {
		case throughBlock < current.ThroughBlock:
			return fmt.Errorf("%w: through block %d is before stored %d",
				repository.ErrInputObservationConflict,
				throughBlock, current.ThroughBlock)
		case throughBlock == current.ThroughBlock &&
			nextInputIndex != current.NextInputIndex:
			return fmt.Errorf("%w: at stored block %d next index is %d, got %d",
				repository.ErrInputObservationConflict,
				throughBlock, current.NextInputIndex, nextInputIndex)
		case throughBlock > current.ThroughBlock &&
			nextInputIndex < current.NextInputIndex:
			return fmt.Errorf("%w: next index %d is before stored %d",
				repository.ErrInputObservationConflict,
				nextInputIndex, current.NextInputIndex)
		}
	}

	for _, input := range inputs {
		if input == nil {
			return errors.New("cannot observe a nil input")
		}
		if input.Index >= nextInputIndex {
			return fmt.Errorf("%w: input index %d is outside the observed prefix ending at %d",
				repository.ErrInputObservationConflict, input.Index, nextInputIndex)
		}
		if input.BlockNumber > throughBlock {
			return fmt.Errorf("%w: input %d is at block %d, after observed block %d",
				repository.ErrInputObservationConflict,
				input.Index, input.BlockNumber, throughBlock)
		}
	}

	if err := insertObservedInputs(ctx, tx, appID, inputs); err != nil {
		return err
	}
	if err := upsertInputObservation(ctx, tx, appID, throughBlock, nextInputIndex); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertObservedInputs(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputs []*model.Input,
) error {
	if len(inputs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, input := range inputs {
		stmt := table.ObservedInput.INSERT(
			table.ObservedInput.ApplicationID,
			table.ObservedInput.Index,
			table.ObservedInput.BlockNumber,
			table.ObservedInput.TransactionHash,
			table.ObservedInput.LogIndex,
			table.ObservedInput.RawData,
		)
		sqlStr, args := stmt.VALUES(
			postgres.Int64(appID),
			uint64Expr(input.Index),
			uint64Expr(input.BlockNumber),
			postgres.Bytea(input.TransactionHash.Bytes()),
			uint64Expr(input.LogIndex),
			postgres.Bytea(input.RawData),
		).ON_CONFLICT(table.ObservedInput.ApplicationID, table.ObservedInput.Index).
			DO_NOTHING().Sql()
		batch.Queue(sqlStr, args...)
	}

	br := tx.SendBatch(ctx, batch)
	var existingRows []*model.Input
	for _, input := range inputs {
		tag, err := br.Exec()
		if err != nil {
			br.Close()
			return wrapInputLogIdentityConflict(err)
		}
		if tag.RowsAffected() == 0 {
			existingRows = append(existingRows, input)
		}
	}
	if err := br.Close(); err != nil {
		return wrapInputLogIdentityConflict(err)
	}

	return verifyObservedInputIdentitiesTx(ctx, tx, appID, existingRows)
}

func verifyObservedInputIdentitiesTx(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputs []*model.Input,
) error {
	for start := 0; start < len(inputs); start += inputIdentityChunkSize {
		chunk := inputs[start:min(start+inputIdentityChunkSize, len(inputs))]
		indices := make([]postgres.Expression, len(chunk))
		for i, input := range chunk {
			indices[i] = uint64Expr(input.Index)
		}
		stmt := table.ObservedInput.SELECT(
			table.ObservedInput.Index,
			table.ObservedInput.TransactionHash,
			table.ObservedInput.LogIndex,
		).WHERE(
			table.ObservedInput.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.ObservedInput.Index.IN(indices...)),
		)
		sqlStr, args := stmt.Sql()
		rows, err := tx.Query(ctx, sqlStr, args...)
		if err != nil {
			return err
		}
		found := make(map[uint64]inputIdentity, len(chunk))
		for rows.Next() {
			var index uint64
			var identity inputIdentity
			var transactionHash []byte
			if err := rows.Scan(&index, &transactionHash,
				&identity.logIndex); err != nil {
				rows.Close()
				return err
			}
			copy(identity.transactionHash[:], transactionHash)
			found[index] = identity
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, input := range chunk {
			existing, ok := found[input.Index]
			if !ok ||
				existing.transactionHash != input.TransactionHash ||
				existing.logIndex != input.LogIndex {
				return fmt.Errorf("%w: observed input %d (tx %s, log %d)",
					repository.ErrInputLogIdentityConflict,
					input.Index, input.TransactionHash.Hex(), input.LogIndex)
			}
		}
	}
	return nil
}

func upsertInputObservation(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	throughBlock, nextInputIndex uint64,
) error {
	stmt := table.InputObservation.INSERT(
		table.InputObservation.ApplicationID,
		table.InputObservation.ThroughBlock,
		table.InputObservation.NextInputIndex,
	).VALUES(
		postgres.Int64(appID),
		uint64Expr(throughBlock),
		uint64Expr(nextInputIndex),
	).ON_CONFLICT(table.InputObservation.ApplicationID).
		DO_UPDATE(postgres.SET(
			table.InputObservation.ThroughBlock.SET(table.InputObservation.EXCLUDED.ThroughBlock),
			table.InputObservation.NextInputIndex.SET(table.InputObservation.EXCLUDED.NextInputIndex),
		))
	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

func getInputObservationTx(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
) (*model.InputObservation, error) {
	stmt := table.InputObservation.SELECT(
		table.InputObservation.ThroughBlock,
		table.InputObservation.NextInputIndex,
	).WHERE(
		table.InputObservation.ApplicationID.EQ(postgres.Int64(appID)),
	)
	sqlStr, args := stmt.Sql()
	var observation model.InputObservation
	err := tx.QueryRow(ctx, sqlStr, args...).Scan(
		&observation.ThroughBlock, &observation.NextInputIndex)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &observation, nil
}

func (r *PostgresRepository) GetInputObservation(
	ctx context.Context,
	nameOrAddress string,
) (*model.InputObservation, error) {
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)
	stmt := table.InputObservation.SELECT(
		table.InputObservation.ThroughBlock,
		table.InputObservation.NextInputIndex,
	).FROM(
		table.InputObservation.INNER_JOIN(table.Application,
			table.InputObservation.ApplicationID.EQ(table.Application.ID)),
	).WHERE(
		whereClause,
	)
	sqlStr, args := stmt.Sql()
	var observation model.InputObservation
	err := r.db.QueryRow(ctx, sqlStr, args...).Scan(
		&observation.ThroughBlock, &observation.NextInputIndex)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &observation, nil
}

func (r *PostgresRepository) ListObservedInputs(
	ctx context.Context,
	nameOrAddress string,
	lower, upper uint64,
) ([]*model.Input, error) {
	if upper <= lower {
		return nil, nil
	}
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)
	stmt := table.ObservedInput.SELECT(
		table.ObservedInput.Index,
		table.ObservedInput.BlockNumber,
		table.ObservedInput.TransactionHash,
		table.ObservedInput.LogIndex,
		table.ObservedInput.RawData,
	).FROM(
		table.ObservedInput.INNER_JOIN(table.Application,
			table.ObservedInput.ApplicationID.EQ(table.Application.ID)),
	).WHERE(
		whereClause.
			AND(table.ObservedInput.Index.GT_EQ(uint64Expr(lower))).
			AND(table.ObservedInput.Index.LT(uint64Expr(upper))),
	).ORDER_BY(
		table.ObservedInput.Index.ASC(),
	)

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	inputs := make([]*model.Input, 0)
	for rows.Next() {
		input := &model.Input{Status: model.InputCompletionStatus_None}
		var transactionHash []byte
		if err := rows.Scan(
			&input.Index,
			&input.BlockNumber,
			&transactionHash,
			&input.LogIndex,
			&input.RawData,
		); err != nil {
			return nil, err
		}
		copy(input.TransactionHash[:], transactionHash)
		inputs = append(inputs, input)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return inputs, nil
}

// CountObservedInputs is retained on the concrete repository for Postgres
// tests; ListObservedInputs covers production needs.
func (r *PostgresRepository) CountObservedInputs(
	ctx context.Context,
	nameOrAddress string,
) (uint64, error) {
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)
	stmt := table.ObservedInput.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(
			table.ObservedInput.INNER_JOIN(table.Application,
				table.ObservedInput.ApplicationID.EQ(table.Application.ID)),
		).WHERE(
		whereClause,
	)
	sqlStr, args := stmt.Sql()
	var count uint64
	err := r.db.QueryRow(ctx, sqlStr, args...).Scan(&count)
	return count, err
}
