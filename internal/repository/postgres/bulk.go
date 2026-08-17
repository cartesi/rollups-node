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
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func encodeSiblings[T ~[32]byte](siblings []T) [][]byte {
	arr := make([][]byte, len(siblings))
	for i, h := range siblings {
		arr[i] = make([]byte, len(h))
		copy(arr[i], h[:])
	}
	return arr
}

func getOutputNextIndex(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
) (uint64, error) {

	query := table.Output.SELECT(
		postgres.COALESCE(
			postgres.Float(1).ADD(postgres.MAXf(table.Output.Index)),
			postgres.Float(0),
		),
	).FROM(
		table.Output.INNER_JOIN(table.Input, table.Input.EpochApplicationID.EQ(table.Output.InputEpochApplicationID).
			AND(table.Input.Index.EQ(table.Output.InputIndex))),
	).WHERE(
		table.Output.InputEpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get the next output index: %w", err)
	}
	return currentIndex, nil
}

func getReportNextIndex(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
) (uint64, error) {

	query := table.Report.SELECT(
		postgres.COALESCE(
			postgres.Float(1).ADD(postgres.MAXf(table.Report.Index)),
			postgres.Float(0),
		),
	).FROM(
		table.Report.INNER_JOIN(table.Input, table.Input.EpochApplicationID.EQ(table.Report.InputEpochApplicationID).
			AND(table.Input.Index.EQ(table.Report.InputIndex))),
	).WHERE(
		table.Report.InputEpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get the next report index: %w", err)
	}
	return currentIndex, nil
}

func getStateHashNextIndex(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
) (uint64, error) {

	query := table.StateHashes.SELECT(table.StateHashes.Index).
		WHERE(
			table.StateHashes.InputEpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.StateHashes.EpochIndex.EQ(uint64Expr(epochIndex))),
		).
		ORDER_BY(table.StateHashes.Index.DESC()).
		LIMIT(1)
	queryStr, args := query.Sql()
	var maxIndex uint64
	if err := tx.QueryRow(ctx, queryStr, args...).Scan(&maxIndex); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("failed to get the last state hash index: %w", err)
	}
	if maxIndex == math.MaxUint64 {
		return 0, errors.New("state hash index space is exhausted")
	}
	return maxIndex + 1, nil
}

func insertOutputs(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	dataArray [][]byte,
) error {
	if len(dataArray) < 1 {
		return nil
	}

	nextIndex, err := getOutputNextIndex(ctx, tx, appID)
	if err != nil {
		return err
	}

	stmt := table.Output.INSERT(
		table.Output.InputEpochApplicationID,
		table.Output.InputIndex,
		table.Output.Index,
		table.Output.RawData,
	)
	for i, data := range dataArray {
		stmt = stmt.VALUES(
			appID,
			inputIndex,
			nextIndex+uint64(i),
			data,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	return nil
}

func insertReports(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	dataArray [][]byte,
) error {
	if len(dataArray) < 1 {
		return nil
	}

	nextIndex, err := getReportNextIndex(ctx, tx, appID)
	if err != nil {
		return err
	}

	stmt := table.Report.INSERT(
		table.Report.InputEpochApplicationID,
		table.Report.InputIndex,
		table.Report.Index,
		table.Report.RawData,
	)
	for i, data := range dataArray {
		stmt = stmt.VALUES(
			appID,
			inputIndex,
			nextIndex+uint64(i),
			data,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	return nil
}

func insertStateHashes(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
	inputIndex uint64,
	hashes [][32]byte,
	machineHash common.Hash,
	paddingRepetitions uint64,
) error {
	rowCount, err := stateHashInsertShape(uint64(len(hashes)), paddingRepetitions)
	if err != nil {
		return err
	}

	nextIndex, err := getStateHashNextIndex(ctx, tx, appID, epochIndex)
	if err != nil {
		return err
	}
	if rowCount-1 > math.MaxUint64-nextIndex {
		return fmt.Errorf(
			"state hash index range overflows uint64: start=%d rows=%d",
			nextIndex,
			rowCount,
		)
	}

	source := &stateHashCopySource{
		appID:              appID,
		epochIndex:         epochIndex,
		inputIndex:         inputIndex,
		nextIndex:          nextIndex,
		hashes:             hashes,
		machineHash:        machineHash,
		paddingRepetitions: paddingRepetitions,
		rowCount:           rowCount,
	}
	copied, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"public", "state_hashes"},
		[]string{
			"input_epoch_application_id",
			"epoch_index",
			"input_index",
			"index",
			"machine_hash",
			"repetitions",
		},
		source,
	)
	if err != nil {
		return err
	}
	expectedCopied := int64(rowCount) //nolint:gosec // Span validation bounds rowCount far below MaxInt64.
	if copied != expectedCopied {
		return fmt.Errorf("copied %d state hashes, expected %d", copied, expectedCopied)
	}
	return nil
}

// stateHashCopySource streams one input hash collection directly into PostgreSQL. It
// holds only the caller's hash slice and one row of CopyFrom values, so even a
// full input-span collection does not create a second multi-million-row object.
type stateHashCopySource struct {
	appID              int64
	epochIndex         uint64
	inputIndex         uint64
	nextIndex          uint64
	hashes             [][32]byte
	machineHash        common.Hash
	paddingRepetitions uint64
	rowCount           uint64
	nextRow            uint64
	currentRow         uint64
	values             [6]any
}

func (source *stateHashCopySource) Next() bool {
	if source.nextRow >= source.rowCount {
		return false
	}
	source.currentRow = source.nextRow
	source.nextRow++
	return true
}

func (source *stateHashCopySource) Values() ([]any, error) {
	source.values[0] = source.appID
	source.values[1] = source.epochIndex
	source.values[2] = source.inputIndex
	source.values[3] = source.nextIndex + source.currentRow
	if source.currentRow < uint64(len(source.hashes)) {
		source.values[4] = source.hashes[source.currentRow][:]
		source.values[5] = int64(1)
		return source.values[:], nil
	}
	source.values[4] = source.machineHash[:]
	source.values[5] = source.paddingRepetitions
	return source.values[:], nil
}

func (*stateHashCopySource) Err() error { return nil }

func stateHashInsertShape(
	hashCount uint64,
	paddingRepetitions uint64,
) (rowCount uint64, err error) {
	if err := model.ValidateInputHashCollectionSpan(hashCount, paddingRepetitions); err != nil {
		return 0, fmt.Errorf("invalid input hash collection span: %w", err)
	}
	if paddingRepetitions == 0 {
		return 0, errors.New("canonical input hash collection requires a positive final repetition tail")
	}
	// Span validation bounds hashCount by the collection capacity, so appending the
	// required canonical tail row cannot overflow uint64.
	return hashCount + 1, nil
}

func updateInput(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
	inputIndex uint64,
	status model.InputCompletionStatus,
	exceptionData []byte,
	txBufferDataBlock common.Hash,
	machineHash common.Hash,
) error {

	updStmt := table.Input.
		UPDATE(
			table.Input.Status,
			table.Input.ExceptionData,
			table.Input.MachineHash,
			table.Input.TxBufferDataBlock,
		).
		SET(
			status,
			exceptionData,
			machineHash[:],
			txBufferDataBlock[:],
		).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Input.EpochIndex.EQ(uint64Expr(epochIndex))).
				AND(table.Input.Index.EQ(uint64Expr(inputIndex))).
				AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_None.String()))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func updateEpochState(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
	proof *model.StateProof,
) error {

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.TxBufferDataBlock,
			table.Epoch.TxBufferProof,
			table.Epoch.MachineHash,
			table.Epoch.IflagsYDataBlock,
			table.Epoch.IflagsYProof,
			table.Epoch.HtifTohostDataBlock,
			table.Epoch.HtifTohostProof,
		).
		SET(
			proof.TxBufferDataBlock[:],
			encodeSiblings(proof.TxBufferProof),
			proof.MachineHash[:],
			proof.IflagsYDataBlock[:],
			encodeSiblings(proof.IflagsYProof),
			proof.HtifTohostDataBlock[:],
			encodeSiblings(proof.HtifTohostProof),
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Epoch.Index.EQ(uint64Expr(epochIndex))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// lockAdvanceRows acquires and validates StoreAdvanceResult's lock set in the
// shared Application -> child-row order. The application row is the aggregate
// lock that serializes multi-row work for one application; the input row lock
// then keeps the exact input's epoch and completion status stable until the
// result transaction finishes.
//
// These are PostgreSQL transaction-scoped row locks, not Go mutexes or session
// advisory locks. Commit or rollback releases them. If the node connection is
// lost, PostgreSQL aborts its transaction when it detects the disconnect; if
// PostgreSQL itself restarts, crash recovery discards the uncommitted
// transaction. A network failure can delay release until the server detects
// the dead connection (or a configured timeout terminates it), but the lock has
// no lifetime independent of that transaction and server backend.
func lockAdvanceRows(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
	inputIndex uint64,
) error {
	// SELECT FOR NO KEY UPDATE reads the cursor and status while taking a row
	// lock that conflicts with concurrent updates of this application. It does
	// not modify the row and does not block ordinary SELECTs. Because each
	// participating multi-row writer takes this row before an input or epoch,
	// two writers cannot form an Application <-> child-row deadlock cycle.
	appStmt := table.Application.
		SELECT(table.Application.ProcessedInputs, table.Application.Status).
		WHERE(table.Application.ID.EQ(postgres.Int64(appID))).
		FOR(postgres.NO_KEY_UPDATE())
	appSQL, appArgs := appStmt.Sql()
	var processedInputs uint64
	var applicationStatus model.ApplicationStatus
	if err := tx.QueryRow(ctx, appSQL, appArgs...).Scan(
		&processedInputs, &applicationStatus,
	); errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return err
	}
	if applicationStatus != model.ApplicationStatus_OK {
		classification := repository.ErrApplicationNotRunnable
		if applicationStatus.IsTerminal() {
			classification = errors.Join(classification, repository.ErrAdvanceAfterTerminal)
		}
		return fmt.Errorf(
			"%w: application %d has status %q",
			classification,
			appID,
			applicationStatus,
		)
	}
	if processedInputs != inputIndex {
		return fmt.Errorf(
			"%w: application %d expects input %d, got %d",
			repository.ErrAdvanceCursorMismatch,
			appID,
			processedInputs,
			inputIndex,
		)
	}

	// SELECT FOR UPDATE is stronger because this exact input will be updated.
	// Taking it after the aggregate lock makes the preflight check durable: no
	// concurrent writer can change its epoch or completion status between this
	// validation and the result's output/proof/application writes.
	inputStmt := table.Input.
		SELECT(table.Input.EpochIndex, table.Input.Status).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Input.Index.EQ(uint64Expr(inputIndex))),
		).
		FOR(postgres.UPDATE())
	inputSQL, inputArgs := inputStmt.Sql()
	var storedEpochIndex uint64
	var storedStatus model.InputCompletionStatus
	if err := tx.QueryRow(ctx, inputSQL, inputArgs...).Scan(&storedEpochIndex, &storedStatus); errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return err
	}
	if storedEpochIndex != epochIndex || storedStatus != model.InputCompletionStatus_None {
		return fmt.Errorf(
			"%w: input %d has epoch %d and status %q, result has epoch %d",
			repository.ErrAdvanceCursorMismatch,
			inputIndex,
			storedEpochIndex,
			storedStatus,
			epochIndex,
		)
	}

	return nil
}

func updateApp(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	completionStatus model.InputCompletionStatus,
) error {
	where := table.Application.ID.EQ(postgres.Int64(appID)).
		AND(table.Application.ProcessedInputs.EQ(uint64Expr(inputIndex))).
		AND(table.Application.Status.EQ(
			postgres.NewEnumValue(model.ApplicationStatus_OK.String()),
		))

	if applicationStatus, terminal := completionStatus.TerminalApplicationStatus(); terminal {
		reason := fmt.Sprintf("input %d completed with %s", inputIndex, completionStatus)
		updStmt := table.Application.
			UPDATE(
				table.Application.ProcessedInputs,
				table.Application.Status,
				table.Application.Reason,
			).
			SET(
				uint64Expr(inputIndex+1),
				postgres.NewEnumValue(applicationStatus.String()),
				reason,
			).
			WHERE(where)
		return executeApplicationAdvanceUpdate(ctx, tx, updStmt)
	}

	updStmt := table.Application.
		UPDATE(
			table.Application.ProcessedInputs,
		).
		SET(
			uint64Expr(inputIndex + 1),
		).
		WHERE(where)
	return executeApplicationAdvanceUpdate(ctx, tx, updStmt)
}

func executeApplicationAdvanceUpdate(
	ctx context.Context,
	tx pgx.Tx,
	updStmt postgres.UpdateStatement,
) error {
	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrApplicationNotRunnable
	}
	return nil
}

func (r *PostgresRepository) StoreAdvanceResult(
	ctx context.Context,
	appID int64,
	res *model.AdvanceResult,
) error {
	if res == nil {
		return errors.New("advance result must not be nil")
	}
	if !res.Status.IsCompleted() {
		return fmt.Errorf("cannot store advance result with noncompleted status %q", res.Status)
	}
	if err := validateAdvanceExceptionData(res.Status, res.ExceptionData); err != nil {
		return err
	}
	if res.InputIndex == math.MaxUint64 {
		return errors.New("cannot store an advance result at the maximum input index")
	}
	if res.Status != model.InputCompletionStatus_Accepted &&
		(len(res.Outputs) != 0 || len(res.Reports) != 0) {
		return fmt.Errorf("advance result with status %q must not contain outputs or reports", res.Status)
	}
	if !res.IsComplete() {
		return repository.ErrInvalidStateProof
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Acquire and validate the transaction-scoped Application -> Input lock set
	// before inserting any effects. Holding both rows through Commit keeps the
	// application cursor/status and the pending input used by this result
	// unchanged while outputs, reports, proofs, and terminal status are written.
	// Cancellation makes the waiting query return; the deferred rollback then
	// releases any lock that this transaction had already acquired.
	if err := lockAdvanceRows(ctx, tx, appID, res.EpochIndex, res.InputIndex); err != nil {
		return err
	}

	if res.Status == model.InputCompletionStatus_Accepted {
		err = insertOutputs(ctx, tx, appID, res.InputIndex, res.Outputs)
		if err != nil {
			return err
		}

		err = insertReports(ctx, tx, appID, res.InputIndex, res.Reports)
		if err != nil {
			return err
		}
	}

	if res.IsDaveConsensus {
		err = insertStateHashes(
			ctx, tx, appID, res.EpochIndex, res.InputIndex,
			res.PeriodicStateHashes, res.MachineHash, res.PaddingRepetitions,
		)
		if err != nil {
			return err
		}
	}

	err = updateInput(
		ctx, tx, appID, res.EpochIndex, res.InputIndex, res.Status, res.ExceptionData,
		res.TxBufferDataBlock, res.MachineHash,
	)
	if err != nil {
		return err
	}

	switch res.Status {
	case model.InputCompletionStatus_Accepted:
		err = updateEpochState(ctx, tx, appID, res.EpochIndex, &res.StateProof)
	case model.InputCompletionStatus_Rejected:
		// A rejection consumes the input but leaves the pre-input epoch state
		// and proof tuple unchanged.
	case model.InputCompletionStatus_Exception,
		model.InputCompletionStatus_MachineHalted,
		model.InputCompletionStatus_Overflow,
		model.InputCompletionStatus_UnexpectedYield:
		err = updateEpochState(ctx, tx, appID, res.EpochIndex, &res.StateProof)
	case model.InputCompletionStatus_None:
		return fmt.Errorf("unsupported noncompleted advance status %q", res.Status)
	default:
		return fmt.Errorf("unsupported completed advance status %q", res.Status)
	}
	if err != nil {
		return err
	}

	err = updateApp(ctx, tx, appID, res.InputIndex, res.Status)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func validateAdvanceExceptionData(status model.InputCompletionStatus, data []byte) error {
	switch {
	case status == model.InputCompletionStatus_Exception && data == nil:
		return errors.New("exception advance result must include exception data")
	case status != model.InputCompletionStatus_Exception && data != nil:
		return fmt.Errorf("advance result with status %q must not include exception data", status)
	default:
		return nil
	}
}

func updateEpochClaim(
	ctx context.Context,
	tx pgx.Tx,
	e *model.Epoch,
) error {

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.Status,
		).
		SET(
			hashToBytes(e.Commitment),
			encodeSiblings(e.CommitmentProof),
			postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()),
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(e.ApplicationID)).
				AND(table.Epoch.Index.EQ(uint64Expr(e.Index))).
				AND(table.Epoch.Status.EQ(
					postgres.NewEnumValue(model.EpochStatus_InputsProcessed.String()),
				)),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err)
	}
	if cmd.RowsAffected() != 1 {
		return fmt.Errorf("failed to update application %d epoch %d: no rows affected", e.ApplicationID, e.Index)
	}
	return nil
}

func updateOutputs(
	ctx context.Context,
	tx pgx.Tx,
	outputs []*model.Output,
) error {
	if len(outputs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, output := range outputs {
		updStmt := table.Output.
			UPDATE(
				table.Output.Hash,
				table.Output.OutputHashesSiblings,
			).
			SET(
				hashToBytes(output.Hash),
				encodeSiblings(output.OutputHashesSiblings),
			).
			WHERE(
				table.Output.InputEpochApplicationID.EQ(postgres.Int64(output.InputEpochApplicationID)).
					AND(table.Output.Index.EQ(uint64Expr(output.Index))),
			)
		sqlStr, args := updStmt.Sql()
		batch.Queue(sqlStr, args...)
	}

	br := tx.SendBatch(ctx, batch)

	for _, output := range outputs {
		cmd, err := br.Exec()
		if err != nil {
			br.Close()
			return fmt.Errorf("failed to insert proof for output '%d': %w", output.Index, err)
		}
		if cmd.RowsAffected() == 0 {
			br.Close()
			return fmt.Errorf("failed to insert proof for output '%d'. No rows affected", output.Index)
		}
	}
	return br.Close()
}

func (r *PostgresRepository) StoreClaimAndProofs(ctx context.Context, epoch *model.Epoch, outputs []*model.Output) error {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = updateEpochClaim(ctx, tx, epoch)
	if err != nil {
		return err
	}

	err = updateOutputs(ctx, tx, outputs)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func insertCommitments(ctx context.Context, tx pgx.Tx, appID int64, commitments []*model.Commitment) error {
	if len(commitments) < 1 {
		return nil
	}

	stmt := table.Commitments.INSERT(
		table.Commitments.ApplicationID,
		table.Commitments.EpochIndex,
		table.Commitments.TournamentAddress,
		table.Commitments.Commitment,
		table.Commitments.FinalStateHash,
		table.Commitments.SubmitterAddress,
		table.Commitments.BlockNumber,
		table.Commitments.TxHash,
	)
	for _, c := range commitments {
		stmt = stmt.VALUES(
			appID,
			c.EpochIndex,
			c.TournamentAddress,
			c.Commitment[:],
			c.FinalStateHash[:],
			c.SubmitterAddress[:],
			c.BlockNumber,
			c.TxHash[:],
		)
	}

	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

func insertMatches(ctx context.Context, tx pgx.Tx, appID int64, matches []*model.Match) error {
	if len(matches) < 1 {
		return nil
	}

	stmt := table.Matches.INSERT(
		table.Matches.ApplicationID,
		table.Matches.EpochIndex,
		table.Matches.TournamentAddress,
		table.Matches.IDHash,
		table.Matches.CommitmentOne,
		table.Matches.CommitmentTwo,
		table.Matches.LeftOfTwo,
		table.Matches.BlockNumber,
		table.Matches.TxHash,
		table.Matches.Winner,
		table.Matches.DeletionReason,
		table.Matches.DeletionBlockNumber,
		table.Matches.DeletionTxHash,
	)
	for _, m := range matches {
		stmt = stmt.VALUES(
			appID,
			m.EpochIndex,
			m.TournamentAddress[:],
			m.IDHash[:],
			m.CommitmentOne[:],
			m.CommitmentTwo[:],
			m.LeftOfTwo,
			m.BlockNumber,
			m.TxHash,
			m.Winner,
			m.DeletionReason,
			m.DeletionBlockNumber,
			m.DeletionTxHash,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

func insertMatchAdvanced(ctx context.Context, tx pgx.Tx, appID int64, matchAdvanced []*model.MatchAdvanced) error {
	if len(matchAdvanced) < 1 {
		return nil
	}

	stmt := table.MatchAdvances.INSERT(
		table.MatchAdvances.ApplicationID,
		table.MatchAdvances.EpochIndex,
		table.MatchAdvances.TournamentAddress,
		table.MatchAdvances.IDHash,
		table.MatchAdvances.OtherParent,
		table.MatchAdvances.LeftNode,
		table.MatchAdvances.BlockNumber,
		table.MatchAdvances.TxHash,
	)
	for _, ma := range matchAdvanced {
		stmt = stmt.VALUES(
			appID,
			ma.EpochIndex,
			ma.TournamentAddress,
			ma.IDHash,
			ma.OtherParent,
			ma.LeftNode,
			ma.BlockNumber,
			ma.TxHash,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

func updateMatches(ctx context.Context, tx pgx.Tx, appID int64, matches []*model.Match) error {
	if len(matches) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, m := range matches {
		updStmt := table.Matches.UPDATE(
			table.Matches.Winner,
			table.Matches.DeletionReason,
			table.Matches.DeletionBlockNumber,
			table.Matches.DeletionTxHash,
		).SET(
			m.Winner,
			m.DeletionReason,
			m.DeletionBlockNumber,
			m.DeletionTxHash,
		).WHERE(
			table.Matches.ApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Matches.EpochIndex.EQ(uint64Expr(m.EpochIndex))).
				AND(table.Matches.TournamentAddress.EQ(postgres.Bytea(m.TournamentAddress.Bytes()))).
				AND(table.Matches.IDHash.EQ(postgres.Bytea(m.IDHash.Bytes()))),
		)
		sqlStr, args := updStmt.Sql()
		batch.Queue(sqlStr, args...)
	}

	br := tx.SendBatch(ctx, batch)

	for _, m := range matches {
		cmd, err := br.Exec()
		if err != nil {
			br.Close()
			return err
		}
		if cmd.RowsAffected() == 0 {
			br.Close()
			return fmt.Errorf("no match found for update: app %d, epoch %d, tournament %s, idHash %s",
				m.ApplicationID, m.EpochIndex, m.TournamentAddress.Hex(), m.IDHash.Hex())
		}
	}
	return br.Close()
}

func updateLastProcessedBlock(ctx context.Context, tx pgx.Tx, appID int64, lastProcessedBlock uint64) error {
	lastBlock := uint64Expr(lastProcessedBlock)
	appUpdateStmt := table.Application.
		UPDATE(
			table.Application.LastTournamentCheckBlock,
		).
		SET(
			lastBlock,
		).
		WHERE(postgres.AND(
			table.Application.ID.EQ(postgres.Int64(appID)),
			table.Application.LastTournamentCheckBlock.LT(lastBlock),
		))

	sqlStr, args := appUpdateStmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) StoreTournamentEvents(
	ctx context.Context,
	appID int64,
	commitments []*model.Commitment,
	matches []*model.Match,
	matchAdvanced []*model.MatchAdvanced,
	matchDeleted []*model.Match,
	lastProcessedBlock uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = insertCommitments(ctx, tx, appID, commitments)
	if err != nil {
		return err
	}

	err = insertMatches(ctx, tx, appID, matches)
	if err != nil {
		return err
	}

	err = insertMatchAdvanced(ctx, tx, appID, matchAdvanced)
	if err != nil {
		return err
	}

	err = updateMatches(ctx, tx, appID, matchDeleted)
	if err != nil {
		return err
	}

	err = updateLastProcessedBlock(ctx, tx, appID, lastProcessedBlock)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
