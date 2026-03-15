// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

// lockApplication acquires a FOR NO KEY UPDATE row lock on the application row,
// preventing a concurrent hard-delete or lifecycle update from modifying the row
// while this transaction is in progress.
//
// FOR NO KEY UPDATE (not FOR SHARE) is required because callers such as
// StoreAdvanceResult and StoreTournamentEvents later UPDATE the same application
// row within the same transaction. FOR SHARE would deadlock when attempting to
// upgrade the lock if a concurrent UPDATE (e.g., SetApplicationEnabled) is
// queued on the same row.
//
// Returns repository.ErrApplicationDeleted if the row no longer exists.
func lockApplication(ctx context.Context, tx pgx.Tx, appID int64) error {
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM application WHERE id = $1 FOR NO KEY UPDATE)`,
		appID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("lock application: %w", err)
	}
	if !exists {
		return fmt.Errorf("application %d: %w", appID, repository.ErrApplicationDeleted)
	}
	return nil
}

// lockApplicationByName is like lockApplication but resolves
// the application by name or hex address.
func lockApplicationByName(ctx context.Context, tx pgx.Tx, nameOrAddress string) error {
	var exists bool
	var err error
	if isHexAddress(nameOrAddress) {
		addr := common.HexToAddress(nameOrAddress)
		err = tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM application WHERE iapplication_address = $1 FOR NO KEY UPDATE)`,
			addr.Bytes()).Scan(&exists)
	} else {
		err = tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM application WHERE name = $1 FOR NO KEY UPDATE)`,
			nameOrAddress).Scan(&exists)
	}
	if err != nil {
		return fmt.Errorf("lock application: %w", err)
	}
	if !exists {
		return fmt.Errorf("application %q: %w", nameOrAddress, repository.ErrApplicationDeleted)
	}
	return nil
}

// byteSliceToHashSlice converts [][32]byte to []common.Hash without copying.
// This is safe because common.Hash is defined as [32]byte, so the memory layout is identical.
func byteSliceToHashSlice(b [][32]byte) []common.Hash {
	return *(*[]common.Hash)(unsafe.Pointer(&b))
}

func encodeSiblings(siblings []common.Hash) [][]byte {
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

	query := table.StateHashes.SELECT(
		postgres.COALESCE(
			postgres.Float(1).ADD(postgres.MAXf(table.StateHashes.Index)),
			postgres.Float(0),
		),
	).WHERE(
		table.StateHashes.InputEpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.StateHashes.EpochIndex.EQ(uint64Expr(epochIndex))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get the next state hash index: %w", err)
	}
	return currentIndex, nil
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
	remainingMetaCycles uint64,
) error {

	nextIndex, err := getStateHashNextIndex(ctx, tx, appID, epochIndex)
	if err != nil {
		return err
	}

	stmt := table.StateHashes.INSERT(
		table.StateHashes.InputEpochApplicationID,
		table.StateHashes.EpochIndex,
		table.StateHashes.InputIndex,
		table.StateHashes.Index,
		table.StateHashes.MachineHash,
		table.StateHashes.Repetitions,
	)

	for i, h := range hashes {
		stmt = stmt.VALUES(
			appID,
			epochIndex,
			inputIndex,
			nextIndex+uint64(i),
			h[:],
			1,
		)
	}

	stmt = stmt.VALUES(
		appID,
		epochIndex,
		inputIndex,
		nextIndex+uint64(len(hashes)),
		machineHash[:],
		remainingMetaCycles,
	)

	sqlStr, args := stmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	return nil
}

func updateInput(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
	status model.InputCompletionStatus,
	outputsHash common.Hash,
	machineHash common.Hash,
) error {

	updStmt := table.Input.
		UPDATE(
			table.Input.Status,
			table.Input.MachineHash,
			table.Input.OutputsHash,
		).
		SET(
			status,
			machineHash[:],
			outputsHash[:],
		).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Input.Index.EQ(uint64Expr(inputIndex))),
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

func updateEpochOutputsMerkleProof(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	epochIndex uint64,
	outputsHash common.Hash,
	outputsHashProof []common.Hash,
	machineHash common.Hash,
) error {

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.OutputsMerkleRoot,
			table.Epoch.OutputsMerkleProof,
			table.Epoch.MachineHash,
		).
		SET(
			outputsHash[:],
			encodeSiblings(outputsHashProof),
			machineHash[:],
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

func updateApp(
	ctx context.Context,
	tx pgx.Tx,
	appID int64,
	inputIndex uint64,
) error {

	updStmt := table.Application.
		UPDATE(
			table.Application.ProcessedInputs,
		).
		SET(
			uint64Expr(inputIndex + 1),
		).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)),
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

func (r *PostgresRepository) StoreAdvanceResult(
	ctx context.Context,
	appID int64,
	res *model.AdvanceResult,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := lockApplication(ctx, tx, appID); err != nil {
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
		err = insertStateHashes(ctx, tx, appID, res.EpochIndex, res.InputIndex, res.Hashes, res.MachineHash, res.RemainingMetaCycles)
		if err != nil {
			return err
		}
	}

	err = updateInput(ctx, tx, appID, res.InputIndex, res.Status, res.OutputsHash, res.MachineHash)
	if err != nil {
		return err
	}

	err = updateEpochOutputsMerkleProof(ctx, tx, appID, res.EpochIndex, res.OutputsHash,
		byteSliceToHashSlice(res.OutputsHashProof), res.MachineHash)
	if err != nil {
		return err
	}

	err = updateApp(ctx, tx, appID, res.InputIndex)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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

	if err := lockApplication(ctx, tx, epoch.ApplicationID); err != nil {
		return err
	}

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

	if err := lockApplication(ctx, tx, appID); err != nil {
		return err
	}

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
