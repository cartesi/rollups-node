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
			table.Input.TransactionReference,
		)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

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
					postgres.Bytea(input.TransactionReference.Bytes()),
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
					return err
				}
			}
			if err := br.Close(); err != nil {
				return err
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
			table.Epoch.OutputsMerkleRoot,
			table.Epoch.OutputsMerkleProof,
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
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
		&ep.OutputsMerkleRoot,
		&ep.OutputsMerkleProof,
		&ep.Commitment,
		&ep.CommitmentProof,
		&ep.ClaimTransactionHash,
		&ep.TournamentAddress,
		&ep.Status,
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
			table.Epoch.OutputsMerkleRoot,
			table.Epoch.OutputsMerkleProof,
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
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
		&ep.OutputsMerkleRoot,
		&ep.OutputsMerkleProof,
		&ep.Commitment,
		&ep.CommitmentProof,
		&ep.ClaimTransactionHash,
		&ep.TournamentAddress,
		&ep.Status,
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
			table.Epoch.OutputsMerkleRoot,
			table.Epoch.OutputsMerkleProof,
			table.Epoch.Commitment,
			table.Epoch.CommitmentProof,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
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
		&ep.OutputsMerkleRoot,
		&ep.OutputsMerkleProof,
		&ep.Commitment,
		&ep.CommitmentProof,
		&ep.ClaimTransactionHash,
		&ep.TournamentAddress,
		&ep.Status,
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

func (r *PostgresRepository) UpdateEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64, proof *model.OutputsProof) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	err = updateEpochOutputsMerkleProof(ctx, tx, appID, epochIndex,
		proof.OutputsHash, byteSliceToHashSlice(proof.OutputsHashProof), proof.MachineHash)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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
) error {

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

	// Update statement to set epoch status to InputsProcessed
	updateStmt := table.Epoch.UPDATE(table.Epoch.Status).
		SET(enum.EpochStatus.InputsProcessed).
		FROM(table.Application).
		WHERE(postgres.AND(
			table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_Closed.String())),
			table.Epoch.ApplicationID.EQ(table.Application.ID),
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
			return nil
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
			table.Epoch.OutputsMerkleRoot,
			table.Epoch.Commitment,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.TournamentAddress,
			table.Epoch.Status,
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
			&ep.OutputsMerkleRoot,
			&ep.Commitment,
			&ep.ClaimTransactionHash,
			&ep.TournamentAddress,
			&ep.Status,
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

func (r *PostgresRepository) RepeatPreviousEpochOutputsProof(
	ctx context.Context,
	appID int64,
	epochIndex uint64,
) error {
	if epochIndex == 0 {
		return fmt.Errorf("cannot repeat outputs proof for epoch 0")
	}

	e1 := table.Epoch.AS("e1")
	e2 := table.Epoch.AS("e2")
	updStmt := e1.
		UPDATE(
			e1.OutputsMerkleRoot,
			e1.OutputsMerkleProof,
			e1.MachineHash,
		).
		SET(
			e2.OutputsMerkleRoot,
			e2.OutputsMerkleProof,
			e2.MachineHash,
		).
		FROM(e2).
		WHERE(postgres.AND(
			e1.ApplicationID.EQ(postgres.Int64(appID)),
			e1.Index.EQ(uint64Expr(epochIndex)),
			e2.ApplicationID.EQ(postgres.Int64(appID)),
			e2.Index.EQ(uint64Expr(epochIndex-1)),
		))

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
