// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func encodeSiblings(outputHashesSiblings []common.Hash) ([]byte, error) {
	// 1) Make a slice of []byte
	arr := make([][]byte, 0, len(outputHashesSiblings))
	for _, h := range outputHashesSiblings {
		// h is [32]byte
		// we must copy it into a slice of bytes
		copyH := make([]byte, len(h))
		copy(copyH, h[:])
		arr = append(arr, copyH)
	}

	// 2) Use pgtype.ByteaArray and call Set with [][]byte
	var siblings pgtype.ByteaArray
	if err := siblings.Set(arr); err != nil {
		return nil, fmt.Errorf("failed to set ByteaArray: %w", err)
	}

	// 3) Encode it as text (the Postgres array string, e.g. '{\\x...,\\x..., ...}')
	encoded, err := siblings.EncodeText(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ByteaArray: %w", err)
	}

	return encoded, nil
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
		err = fmt.Errorf("failed to get the next output index: %w", err)
		return 0, errors.Join(err, tx.Rollback(ctx))
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
		table.Report.INNER_JOIN(table.Input, table.Input.EpochApplicationID.EQ(table.Report.InputEpochApplicationID)),
	).WHERE(
		table.Report.InputEpochApplicationID.EQ(postgres.Int64(appID)).
			AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		err = fmt.Errorf("failed to get the next report index: %w", err)
		return 0, errors.Join(err, tx.Rollback(ctx))
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
			AND(table.StateHashes.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", epochIndex)))),
	)

	queryStr, args := query.Sql()
	var currentIndex uint64
	err := tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		err = fmt.Errorf("failed to get the next state hash index: %w", err)
		return 0, errors.Join(err, tx.Rollback(ctx))
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
		return errors.Join(err, tx.Rollback(ctx))
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
		return errors.Join(err, tx.Rollback(ctx))
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
		return errors.Join(err, tx.Rollback(ctx))
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
			machineHash,
			outputsHash,
		).
		WHERE(
			table.Input.EpochApplicationID.EQ(postgres.Int64(appID)).
				AND(table.Input.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", inputIndex)))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return sql.ErrNoRows
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
			postgres.RawFloat(fmt.Sprintf("%d", inputIndex+1)),
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
		return sql.ErrNoRows
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

	err = updateApp(ctx, tx, appID, res.InputIndex)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	return nil
}

func updateEpochClaim(
	ctx context.Context,
	tx pgx.Tx,
	e *model.Epoch,
) error {

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.MachineHash,
			table.Epoch.ClaimHash,
			table.Epoch.Commitment,
			table.Epoch.Status,
		).
		SET(
			e.MachineHash,
			e.ClaimHash,
			e.Commitment,
			postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()),
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(e.ApplicationID)).
				AND(table.Epoch.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", e.Index)))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(
			fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err),
			tx.Rollback(ctx),
		)
	}
	if cmd.RowsAffected() != 1 {
		return errors.Join(
			fmt.Errorf("failed to update application %d epoch %d: no rows affected", e.ApplicationID, e.Index),
			tx.Rollback(ctx),
		)
	}
	return nil
}

func updateOutputs(
	ctx context.Context,
	tx pgx.Tx,
	outputs []*model.Output,
) error {
	for _, output := range outputs {
		siblings, err := encodeSiblings(output.OutputHashesSiblings)
		if err != nil {
			return errors.Join(
				fmt.Errorf("failed to serialize outputHashesSiblings for output '%d'. %w", output.Index, err),
				tx.Rollback(ctx),
			)
		}

		updStmt := table.Output.
			UPDATE(
				table.Output.Hash,
				table.Output.OutputHashesSiblings,
			).
			SET(
				output.Hash,
				siblings,
			).
			WHERE(
				table.Output.InputEpochApplicationID.EQ(postgres.Int64(output.InputEpochApplicationID)).
					AND(table.Output.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", output.Index)))),
			)

		sqlStr, args := updStmt.Sql()
		cmd, err := tx.Exec(ctx, sqlStr, args...)
		if err != nil {
			return errors.Join(
				fmt.Errorf("failed to insert proof for output '%d'. %w", output.Index, err),
				tx.Rollback(ctx),
			)
		}
		if cmd.RowsAffected() == 0 {
			return errors.Join(
				fmt.Errorf(
					"failed to insert proof for output '%d'. No rows affected",
					output.Index,
				),
				tx.Rollback(ctx),
			)
		}
	}
	return nil
}

func (r *PostgresRepository) StoreClaimAndProofs(ctx context.Context, epoch *model.Epoch, outputs []*model.Output) error {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err)
	}

	err = updateEpochClaim(ctx, tx, epoch)
	if err != nil {
		return err
	}

	err = updateOutputs(ctx, tx, outputs)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(
			fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err),
			tx.Rollback(ctx),
		)
	}
	return nil
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
			c.Commitment,
			c.FinalStateHash,
			c.SubmitterAddress,
			c.BlockNumber,
			c.TxHash,
		)
	}

	sqlStr, args := stmt.Sql()
	_, err := tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return nil
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
			m.TournamentAddress,
			m.IDHash,
			m.CommitmentOne,
			m.CommitmentTwo,
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
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return nil
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
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return nil
}

func updateMatches(ctx context.Context, tx pgx.Tx, appID int64, matches []*model.Match) error {
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
				AND(table.Matches.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", m.EpochIndex)))).
				AND(table.Matches.TournamentAddress.EQ(postgres.Bytea(m.TournamentAddress.Bytes()))).
				AND(table.Matches.IDHash.EQ(postgres.Bytea(m.IDHash.Bytes()))),
		)

		sqlStr, args := updStmt.Sql()
		cmd, err := tx.Exec(ctx, sqlStr, args...)
		if err != nil {
			return errors.Join(err, tx.Rollback(ctx))
		}
		if cmd.RowsAffected() == 0 {
			return errors.Join(
				fmt.Errorf("no match found for update: app %d, epoch %d, tournament %s, idHash %s", m.ApplicationID, m.EpochIndex, m.TournamentAddress.Hex(), m.IDHash.Hex()),
				tx.Rollback(ctx),
			)
		}
	}
	return nil
}

func updateLastProcessedBlock(ctx context.Context, tx pgx.Tx, appID int64, lastProcessedBlock uint64) error {
	lastBlock := postgres.RawFloat(fmt.Sprintf("%d", lastProcessedBlock))
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
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	return nil
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

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	return nil
}
