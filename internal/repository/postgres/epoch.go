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
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func getEpochNextVirtualIndex(
	ctx context.Context,
	tx pgx.Tx,
	nameOrAddress string,
) (uint64, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return 0, err
	}

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
	err = tx.QueryRow(ctx, queryStr, args...).Scan(&currentIndex)
	if err != nil {
		err = fmt.Errorf("failed to get the next epoch virtual index: %w", err)
		return 0, errors.Join(err, tx.Rollback(ctx))
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

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return err
	}

	epochInsertStmt := table.Epoch.INSERT(
		table.Epoch.ApplicationID,
		table.Epoch.Index,
		table.Epoch.FirstBlock,
		table.Epoch.LastBlock,
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

	epochs := orderEpochs(epochInputsMap)
	for _, epoch := range epochs {
		inputs := epochInputsMap[epoch]

		nextVirtualIndex, err := getEpochNextVirtualIndex(ctx, tx, nameOrAddress)
		if err != nil {
			return err
		}

		epochSelectQuery := table.Application.SELECT(
			table.Application.ID,
			postgres.RawFloat(fmt.Sprintf("%d", epoch.Index)),
			postgres.RawFloat(fmt.Sprintf("%d", epoch.FirstBlock)),
			postgres.RawFloat(fmt.Sprintf("%d", epoch.LastBlock)),
			postgres.NewEnumValue(epoch.Status.String()),
			postgres.RawFloat(fmt.Sprintf("%d", nextVirtualIndex)),
		).WHERE(
			whereClause,
		)

		sqlStr, args := epochInsertStmt.QUERY(epochSelectQuery).
			ON_CONFLICT(table.Epoch.ApplicationID, table.Epoch.Index).
			DO_UPDATE(postgres.SET(
				table.Epoch.Status.SET(postgres.NewEnumValue(epoch.Status.String())),
			)).Sql() // FIXME on conflict
		_, err = tx.Exec(ctx, sqlStr, args...)

		if err != nil {
			return errors.Join(err, tx.Rollback(ctx))
		}

		for _, input := range inputs {
			inputSelectQuery := table.Application.SELECT(
				table.Application.ID,
				postgres.RawFloat(fmt.Sprintf("%d", epoch.Index)),
				postgres.RawFloat(fmt.Sprintf("%d", input.Index)),
				postgres.RawFloat(fmt.Sprintf("%d", input.BlockNumber)),
				postgres.Bytea(input.RawData),
				postgres.NewEnumValue(input.Status.String()),
				postgres.Bytea(input.TransactionReference.Bytes()),
			).WHERE(
				whereClause,
			)

			sqlStr, args := inputInsertStmt.QUERY(inputSelectQuery).Sql()
			_, err := tx.Exec(ctx, sqlStr, args...)
			if err != nil {
				return errors.Join(err, tx.Rollback(ctx))
			}
		}
	}

	// Update last processed block
	appUpdateStmt := table.Application.
		UPDATE(
			table.Application.LastInputCheckBlock,
		).
		SET(
			postgres.RawFloat(fmt.Sprintf("%d", blockNumber)),
		).
		WHERE(whereClause)

	sqlStr, args := appUpdateStmt.Sql()
	_, err = tx.Exec(ctx, sqlStr, args...)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}

	return nil
}

func (r *PostgresRepository) GetEpoch(
	ctx context.Context,
	nameOrAddress string,
	index uint64,
) (*model.Epoch, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	stmt := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.ClaimHash,
			table.Epoch.ClaimTransactionHash,
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
				AND(table.Epoch.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", index)))),
		)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var ep model.Epoch
	err = row.Scan(
		&ep.ApplicationID,
		&ep.Index,
		&ep.FirstBlock,
		&ep.LastBlock,
		&ep.ClaimHash,
		&ep.ClaimTransactionHash,
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

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return 0, err
	}

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
				AND(table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimAccepted.String()))),
		).
		ORDER_BY(table.Epoch.Index.DESC()).
		LIMIT(1)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var index uint64
	err = row.Scan(
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

func (r *PostgresRepository) GetEpochByVirtualIndex(
	ctx context.Context,
	nameOrAddress string,
	index uint64,
) (*model.Epoch, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	stmt := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.ClaimHash,
			table.Epoch.ClaimTransactionHash,
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
				AND(table.Epoch.VirtualIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", index)))),
		)

	sqlStr, args := stmt.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var ep model.Epoch
	err = row.Scan(
		&ep.ApplicationID,
		&ep.Index,
		&ep.FirstBlock,
		&ep.LastBlock,
		&ep.ClaimHash,
		&ep.ClaimTransactionHash,
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

func (r *PostgresRepository) UpdateEpoch(
	ctx context.Context,
	nameOrAddress string,
	e *model.Epoch,
) error {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return err
	}

	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.ClaimHash,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.Status,
		).
		SET(
			e.ClaimHash,
			e.ClaimTransactionHash,
			e.Status,
		).
		FROM(
			table.Application,
		).
		WHERE(
			whereClause.
				AND(table.Epoch.ApplicationID.EQ(table.Application.ID)).
				AND(table.Epoch.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", e.Index)))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PostgresRepository) UpdateEpochsInputsProcessed(
	ctx context.Context,
	nameOrAddress string,
) (int64, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return 0, err
	}

	subSelect := table.Input.SELECT(postgres.Raw("1")).
		WHERE(
			table.Input.EpochApplicationID.EQ(table.Epoch.ApplicationID).
				AND(table.Input.EpochIndex.EQ(table.Epoch.Index)).
				AND(table.Input.Status.EQ(postgres.NewEnumValue(model.InputCompletionStatus_None.String()))),
		)

	notExistsClause := postgres.NOT(
		postgres.EXISTS(subSelect),
	)

	updateStmt := table.Epoch.UPDATE(table.Epoch.Status).
		SET(postgres.NewEnumValue(model.EpochStatus_InputsProcessed.String())).
		FROM(table.Application).
		WHERE(
			table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_Closed.String())).
				AND(table.Epoch.ApplicationID.EQ(table.Application.ID)).
				AND(whereClause).
				AND(notExistsClause),
		)

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func (r *PostgresRepository) ListEpochs(
	ctx context.Context,
	nameOrAddress string,
	f repository.EpochFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Epoch, uint64, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, 0, err
	}

	sel := table.Epoch.
		SELECT(
			table.Epoch.ApplicationID,
			table.Epoch.Index,
			table.Epoch.FirstBlock,
			table.Epoch.LastBlock,
			table.Epoch.ClaimHash,
			table.Epoch.ClaimTransactionHash,
			table.Epoch.Status,
			table.Epoch.VirtualIndex,
			table.Epoch.CreatedAt,
			table.Epoch.UpdatedAt,
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.Epoch.
				INNER_JOIN(table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.Status != nil {
		conditions = append(conditions, table.Epoch.Status.EQ(postgres.NewEnumValue(f.Status.String())))
	}

	if f.BeforeBlock != nil {
		conditions = append(conditions, table.Epoch.LastBlock.LT(postgres.RawFloat(fmt.Sprintf("%d", *f.BeforeBlock))))
	}

	sel = sel.WHERE(postgres.AND(conditions...))

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
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var epochs []*model.Epoch
	var total uint64
	for rows.Next() {
		var ep model.Epoch
		err := rows.Scan(
			&ep.ApplicationID,
			&ep.Index,
			&ep.FirstBlock,
			&ep.LastBlock,
			&ep.ClaimHash,
			&ep.ClaimTransactionHash,
			&ep.Status,
			&ep.VirtualIndex,
			&ep.CreatedAt,
			&ep.UpdatedAt,
			&total,
		)
		if err != nil {
			return nil, 0, err
		}
		epochs = append(epochs, &ep)
	}
	return epochs, total, nil
}
