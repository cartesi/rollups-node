// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

var replayCompletedStatuses = []postgres.Expression{
	postgres.NewEnumValue(model.InputCompletionStatus_Accepted.String()),
	postgres.NewEnumValue(model.InputCompletionStatus_Rejected.String()),
	postgres.NewEnumValue(model.InputCompletionStatus_Exception.String()),
	postgres.NewEnumValue(model.InputCompletionStatus_MachineHalted.String()),
}

func replayCompletedStatus(status postgres.StringExpression) postgres.BoolExpression {
	return status.IN(replayCompletedStatuses...)
}

// replayChildSpec captures the schema shared by replay outputs and reports.
// State hashes deliberately use a separate path because their ordering and
// repetition invariants are different.
type replayChildSpec struct {
	table          postgres.Table
	applicationID  postgres.IntegerExpression
	inputIndex     postgres.FloatExpression
	index          postgres.FloatExpression
	rawData        postgres.ByteaExpression
	childKind      repository.ReplayEvidenceKind
	appendToRecord func(*model.ReplayRecord, []byte)
}

var (
	replayOutputSpec = replayChildSpec{
		table:         table.Output,
		applicationID: table.Output.InputEpochApplicationID,
		inputIndex:    table.Output.InputIndex,
		index:         table.Output.Index,
		rawData:       table.Output.RawData,
		childKind:     repository.ReplayEvidenceOutput,
		appendToRecord: func(record *model.ReplayRecord, data []byte) {
			record.Outputs = append(record.Outputs, data)
		},
	}
	replayReportSpec = replayChildSpec{
		table:         table.Report,
		applicationID: table.Report.InputEpochApplicationID,
		inputIndex:    table.Report.InputIndex,
		index:         table.Report.Index,
		rawData:       table.Report.RawData,
		childKind:     repository.ReplayEvidenceReport,
		appendToRecord: func(record *model.ReplayRecord, data []byte) {
			record.Reports = append(record.Reports, data)
		},
	}
)

// ReplaySummary reads the application identity, consensus, and processed-input
// count from one database snapshot. It checks that the count matches the number
// of completed inputs and that completed input indexes are contiguous from zero.
// In Full mode, it also checks PRT state-hash ordering. For non-PRT applications,
// it checks that no state hashes exist. All queries run in the same short
// repeatable-read transaction.
func (r *PostgresRepository) ReplaySummary(
	ctx context.Context,
	applicationAddress common.Address,
	verification repository.ReplayVerificationLevel,
) (model.ReplaySummary, error) {
	if !verification.IsValid() {
		return model.ReplaySummary{}, fmt.Errorf("unsupported replay verification level %d", verification)
	}
	whereApp := table.Application.IapplicationAddress.EQ(postgres.Bytea(applicationAddress.Bytes()))
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return model.ReplaySummary{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	appStmt := table.Application.SELECT(
		table.Application.ID,
		table.Application.ConsensusType,
		table.Application.ProcessedInputs,
	).WHERE(whereApp)
	appSQL, appArgs := appStmt.Sql()
	var summary model.ReplaySummary
	if err := tx.QueryRow(ctx, appSQL, appArgs...).Scan(
		&summary.ApplicationID,
		&summary.Consensus,
		&summary.ProcessedInputs,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ReplaySummary{}, repository.ErrNotFound
		}
		return model.ReplaySummary{}, err
	}

	whereInputApp := table.Input.EpochApplicationID.EQ(postgres.Int64(summary.ApplicationID))
	countStmt := table.Input.SELECT(postgres.COUNT(postgres.STAR)).
		WHERE(whereInputApp.AND(replayCompletedStatus(table.Input.Status)))
	completedInputCount, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return model.ReplaySummary{}, err
	}
	if completedInputCount != summary.ProcessedInputs {
		return model.ReplaySummary{}, &repository.ReplayStructureViolationError{
			Kind:                       repository.ReplayStructureProcessedInputCount,
			InputIndex:                 completedInputCount,
			ApplicationProcessedInputs: summary.ProcessedInputs,
			CompletedInputCount:        completedInputCount,
		}
	}
	if err := validateReplayCompletedPrefix(ctx, tx, whereInputApp, completedInputCount); err != nil {
		return model.ReplaySummary{}, err
	}
	if verification == repository.ReplayVerificationFull {
		whereStateHashApp := table.StateHashes.InputEpochApplicationID.EQ(
			postgres.Int64(summary.ApplicationID),
		)
		var err error
		switch summary.Consensus {
		case model.Consensus_PRT:
			err = validateReplayStateHashOrdering(ctx, tx, whereStateHashApp)
		case model.Consensus_Authority, model.Consensus_Quorum:
			err = validateReplayStateHashesAbsent(ctx, tx, whereStateHashApp)
		default:
			err = fmt.Errorf("unsupported replay consensus %q", summary.Consensus)
		}
		if err != nil {
			return model.ReplaySummary{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ReplaySummary{}, err
	}
	return summary, nil
}

func scanOptionalRow(
	ctx context.Context,
	tx pgx.Tx,
	stmt postgres.SelectStatement,
	dest ...any,
) (bool, error) {
	sqlStr, args := stmt.Sql()
	err := tx.QueryRow(ctx, sqlStr, args...).Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func replayStructureViolation(
	kind repository.ReplayStructureViolationKind,
	epochIndex uint64,
	inputIndex uint64,
	evidenceIndex uint64,
) *repository.ReplayStructureViolationError {
	return &repository.ReplayStructureViolationError{
		Kind:          kind,
		EpochIndex:    &epochIndex,
		InputIndex:    inputIndex,
		EvidenceIndex: evidenceIndex,
	}
}

func validateReplayCompletedPrefix(
	ctx context.Context,
	tx pgx.Tx,
	whereInputApp postgres.BoolExpression,
	total uint64,
) error {
	if total == 0 {
		return nil
	}
	stmt := table.Input.SELECT(table.Input.Index).
		WHERE(whereInputApp.AND(replayCompletedStatus(table.Input.Status))).
		ORDER_BY(table.Input.Index.DESC()).
		LIMIT(1)
	sqlStr, args := stmt.Sql()
	var lastInputIndex uint64
	if err := tx.QueryRow(ctx, sqlStr, args...).Scan(&lastInputIndex); err != nil {
		return err
	}
	expectedLastInputIndex := total - 1
	// Input indexes are non-negative and unique by primary key. Therefore,
	// total distinct rows whose maximum index is total-1 must be exactly the
	// contiguous sequence 0..total-1; a gap would force a larger maximum.
	if lastInputIndex != expectedLastInputIndex {
		return &repository.ReplayStructureViolationError{
			Kind:          repository.ReplayStructureCompletedInputSequence,
			InputIndex:    lastInputIndex,
			ExpectedIndex: expectedLastInputIndex,
		}
	}
	return nil
}

func validateReplayStateHashOrdering(
	ctx context.Context,
	tx pgx.Tx,
	whereStateHashApp postgres.BoolExpression,
) error {
	orderedStateHashes := table.StateHashes.SELECT(
		table.StateHashes.EpochIndex.AS("epoch_index"),
		table.StateHashes.Index.AS("state_hash_index"),
		table.StateHashes.InputIndex.AS("input_index"),
		postgres.ROW_NUMBER().OVER(
			postgres.PARTITION_BY(table.StateHashes.EpochIndex).
				ORDER_BY(table.StateHashes.Index),
		).AS("ordinal"),
		postgres.LAG(table.StateHashes.InputIndex).OVER(
			postgres.PARTITION_BY(table.StateHashes.EpochIndex).
				ORDER_BY(table.StateHashes.Index),
		).AS("previous_input_index"),
	).WHERE(whereStateHashApp).AsTable("ordered_state_hash")

	epochIndexColumn := postgres.FloatColumn("epoch_index").From(orderedStateHashes)
	stateHashIndexColumn := postgres.FloatColumn("state_hash_index").From(orderedStateHashes)
	inputIndexColumn := postgres.FloatColumn("input_index").From(orderedStateHashes)
	ordinalColumn := postgres.FloatColumn("ordinal").From(orderedStateHashes)
	previousInputIndexColumn := postgres.FloatColumn("previous_input_index").From(orderedStateHashes)
	stmt := orderedStateHashes.SELECT(
		epochIndexColumn,
		stateHashIndexColumn,
		inputIndexColumn,
		ordinalColumn,
		postgres.COALESCE(previousInputIndexColumn, inputIndexColumn),
		previousInputIndexColumn.IS_NOT_NULL(),
	).WHERE(
		stateHashIndexColumn.NOT_EQ(ordinalColumn.SUB(postgres.Float(1))).OR(
			previousInputIndexColumn.IS_NOT_NULL().AND(
				inputIndexColumn.LT(previousInputIndexColumn),
			),
		),
	).ORDER_BY(epochIndexColumn.ASC(), stateHashIndexColumn.ASC()).LIMIT(1)

	var epochIndex, stateHashIndex, inputIndex, ordinal, previousInputIndex uint64
	var hasPreviousInput bool
	found, err := scanOptionalRow(ctx, tx, stmt,
		&epochIndex,
		&stateHashIndex,
		&inputIndex,
		&ordinal,
		&previousInputIndex,
		&hasPreviousInput,
	)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if stateHashIndex != ordinal-1 {
		violation := replayStructureViolation(
			repository.ReplayStructureStateHashIndexSequence,
			epochIndex,
			inputIndex,
			stateHashIndex,
		)
		violation.ExpectedIndex = ordinal - 1
		return violation
	}
	violation := replayStructureViolation(
		repository.ReplayStructureStateHashInputOrder,
		epochIndex,
		inputIndex,
		stateHashIndex,
	)
	if hasPreviousInput {
		violation.PreviousInputIndex = previousInputIndex
	}
	return violation
}

func validateReplayStateHashesAbsent(
	ctx context.Context,
	tx pgx.Tx,
	whereStateHashApp postgres.BoolExpression,
) error {
	stmt := table.StateHashes.SELECT(
		table.StateHashes.EpochIndex,
		table.StateHashes.InputIndex,
		table.StateHashes.Index,
	).WHERE(whereStateHashApp).
		ORDER_BY(table.StateHashes.EpochIndex.ASC(), table.StateHashes.Index.ASC()).
		LIMIT(1)

	var epochIndex, inputIndex, stateHashIndex uint64
	found, err := scanOptionalRow(
		ctx, tx, stmt,
		&epochIndex,
		&inputIndex,
		&stateHashIndex,
	)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return replayStructureViolation(
		repository.ReplayStructureUnexpectedStateHash,
		epochIndex,
		inputIndex,
		stateHashIndex,
	)
}

// ReplayPage returns one keyset page from the summary's fixed application and
// high-water mark. Canonical pages query inputs only. Full pages load child
// evidence in the same short repeatable-read transaction.
func (r *PostgresRepository) ReplayPage(
	ctx context.Context,
	request repository.ReplayPageRequest,
) ([]*model.ReplayRecord, error) {
	if !request.Verification.IsValid() {
		return nil, fmt.Errorf("unsupported replay verification level %d", request.Verification)
	}
	if request.ApplicationID <= 0 {
		return nil, fmt.Errorf("replay application ID must be greater than zero")
	}
	if request.Limit == 0 {
		return nil, fmt.Errorf("replay page limit must be greater than zero")
	}
	if request.Limit > math.MaxInt64 {
		return nil, fmt.Errorf("replay page limit %d exceeds maximum supported value %d", request.Limit, int64(math.MaxInt64))
	}
	if request.FromInput > request.ToInputExclusive {
		return nil, fmt.Errorf(
			"replay input range is invalid: lower bound %d exceeds upper bound %d",
			request.FromInput, request.ToInputExclusive,
		)
	}
	if request.FromInput == request.ToInputExclusive {
		return []*model.ReplayRecord{}, nil
	}

	whereInputApp := table.Input.EpochApplicationID.EQ(postgres.Int64(request.ApplicationID))
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	inputStmt := table.Input.SELECT(
		table.Input.EpochApplicationID,
		table.Input.EpochIndex,
		table.Input.Index,
		table.Input.RawData,
		table.Input.Status,
		table.Input.ExceptionData,
		table.Input.MachineHash,
		table.Input.OutputsHash,
	).
		WHERE(
			whereInputApp.
				AND(replayCompletedStatus(table.Input.Status)).
				AND(table.Input.Index.GT_EQ(uint64Expr(request.FromInput))).
				AND(table.Input.Index.LT(uint64Expr(request.ToInputExclusive))),
		).
		ORDER_BY(table.Input.Index.ASC()).
		LIMIT(int64(request.Limit))

	records, err := selectReplayInputs(ctx, tx, inputStmt)
	if err != nil {
		return nil, err
	}

	byInput := make(map[uint64]*model.ReplayRecord, len(records))
	for _, record := range records {
		byInput[record.Input.InputIndex] = record
	}
	if request.Verification == repository.ReplayVerificationCanonical {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return records, nil
	}

	childToInputIndexExclusive := request.ToInputExclusive
	if uint64(len(records)) == request.Limit {
		// A full page covers only through its last returned input. Evidence for
		// later completed inputs belongs to a subsequent page and must not be
		// mistaken for an unmatched child in this page.
		childToInputIndexExclusive = records[len(records)-1].Input.InputIndex + 1
	}

	if err := selectReplayChildren(
		ctx, tx, request.ApplicationID, request.FromInput, childToInputIndexExclusive, byInput, replayOutputSpec,
	); err != nil {
		return nil, err
	}
	if err := selectReplayChildren(
		ctx, tx, request.ApplicationID, request.FromInput, childToInputIndexExclusive, byInput, replayReportSpec,
	); err != nil {
		return nil, err
	}
	if err := selectReplayStateHashes(
		ctx, tx, request.ApplicationID, request.FromInput, childToInputIndexExclusive, byInput,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return records, nil
}

func selectReplayInputs(
	ctx context.Context,
	tx pgx.Tx,
	stmt postgres.SelectStatement,
) ([]*model.ReplayRecord, error) {
	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*model.ReplayRecord
	for rows.Next() {
		record := new(model.ReplayRecord)
		in := &record.Input
		if err := rows.Scan(
			&in.ApplicationID,
			&in.EpochIndex,
			&in.InputIndex,
			&in.RawData,
			&in.Status,
			&in.ExceptionData,
			&in.MachineHash,
			&in.OutputsHash,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func replayRecordForChild(
	byInput map[uint64]*model.ReplayRecord,
	childKind repository.ReplayEvidenceKind,
	inputIndex uint64,
) (*model.ReplayRecord, error) {
	record := byInput[inputIndex]
	if record == nil {
		return nil, &repository.ReplayInconsistentEvidenceError{
			Kind:       childKind,
			InputIndex: inputIndex,
		}
	}
	return record, nil
}

func selectReplayChildren(
	ctx context.Context,
	tx pgx.Tx,
	applicationID int64,
	fromInputIndex, toInputIndexExclusive uint64,
	byInput map[uint64]*model.ReplayRecord,
	spec replayChildSpec,
) error {
	stmt := spec.table.SELECT(
		spec.inputIndex,
		spec.rawData,
	).WHERE(
		spec.applicationID.EQ(postgres.Int64(applicationID)).
			AND(spec.inputIndex.GT_EQ(uint64Expr(fromInputIndex))).
			AND(spec.inputIndex.LT(uint64Expr(toInputIndexExclusive))),
	).ORDER_BY(spec.inputIndex.ASC(), spec.index.ASC())

	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var inputIndex uint64
		var data []byte
		if err := rows.Scan(&inputIndex, &data); err != nil {
			return err
		}
		record, err := replayRecordForChild(byInput, spec.childKind, inputIndex)
		if err != nil {
			return err
		}
		spec.appendToRecord(record, data)
	}
	return rows.Err()
}

func selectReplayStateHashes(
	ctx context.Context,
	tx pgx.Tx,
	applicationID int64,
	fromInputIndex, toInputIndexExclusive uint64,
	byInput map[uint64]*model.ReplayRecord,
) error {
	stmt := table.StateHashes.SELECT(
		table.StateHashes.InputIndex,
		table.StateHashes.Index,
		table.StateHashes.MachineHash,
		table.StateHashes.Repetitions,
	).WHERE(
		table.StateHashes.InputEpochApplicationID.EQ(postgres.Int64(applicationID)).
			AND(table.StateHashes.InputIndex.GT_EQ(uint64Expr(fromInputIndex))).
			AND(table.StateHashes.InputIndex.LT(uint64Expr(toInputIndexExclusive))),
	).ORDER_BY(table.StateHashes.InputIndex.ASC(), table.StateHashes.Index.ASC())

	sqlStr, args := stmt.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var inputIndex uint64
		var row model.ReplayStateHash
		if err := rows.Scan(&inputIndex, &row.Index, &row.MachineHash, &row.Repetitions); err != nil {
			return err
		}
		record, err := replayRecordForChild(byInput, repository.ReplayEvidenceStateHash, inputIndex)
		if err != nil {
			return err
		}
		record.StateHashes = append(record.StateHashes, row)
	}
	return rows.Err()
}
