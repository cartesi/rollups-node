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
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

var (
	ErrNoUpdate = fmt.Errorf("update did not take effect")
)

// Retrieve the claim of each application with the smallest index.
// The query may return either 0 or 1 entries per application.
func (r *PostgresRepository) selectOldestClaimPerApp(
	ctx context.Context,
	epochStatus model.EpochStatus,
) (
	map[int64]*model.Epoch,
	map[int64]*model.Application,
	error,
) {
	if (epochStatus != model.EpochStatus_ClaimSubmitted) && (epochStatus != model.EpochStatus_ClaimComputed) {
		return nil, nil, fmt.Errorf("Invalid epoch status: %v", epochStatus)
	}

	// NOTE(mpolitzer): DISTINCT ON is a postgres extension. To implement
	// this in SQLite there is an alternative using GROUP BY and HAVING
	// clauses instead.
	stmt := table.Epoch.SELECT(
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

		table.Application.ID,
		table.Application.Name,
		table.Application.IapplicationAddress,
		table.Application.IconsensusAddress,
		table.Application.IinputboxAddress,
		table.Application.TemplateHash,
		table.Application.TemplateURI,
		table.Application.EpochLength,
		table.Application.DataAvailability,
		table.Application.State,
		table.Application.Reason,
		table.Application.IinputboxBlock,
		table.Application.LastInputCheckBlock,
		table.Application.LastOutputCheckBlock,
		table.Application.ProcessedInputs,
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
				AND(table.Application.State.EQ(enum.ApplicationState.Enabled)).
				AND(table.Application.ConsensusType.NOT_EQ(enum.Consensus.Prt)),
		).
		ORDER_BY(
			table.Epoch.ApplicationID,
			table.Epoch.Index.ASC(),
		)

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, nil, err
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
			&epoch.ClaimHash,
			&epoch.ClaimTransactionHash,
			&epoch.Status,
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
			&application.DataAvailability,
			&application.State,
			&application.Reason,
			&application.IInputBoxBlock,
			&application.LastInputCheckBlock,
			&application.LastOutputCheckBlock,
			&application.ProcessedInputs,
			&application.CreatedAt,
			&application.UpdatedAt,
		)
		if err != nil {
			return nil, nil, err
		}
		epochs[application.ID] = &epoch
		applications[application.ID] = &application
	}
	return epochs, applications, nil
}

// Retrieve the newest accepted claim of each application
func (r *PostgresRepository) selectNewestAcceptedClaimPerApp(
	ctx context.Context,
	includeSubmitted bool,
) (
	map[int64]*model.Epoch,
	error,
) {
	expr := table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimAccepted.String()))
	if includeSubmitted {
		expr = expr.OR(table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String())))
	}

	// NOTE(mpolitzer): DISTINCT ON is a postgres extension. To implement
	// this in SQLite there is an alternative using GROUP BY and HAVING
	// clauses instead.
	stmt := table.Epoch.SELECT(
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
		DISTINCT(table.Epoch.ApplicationID).
		FROM(
			table.Epoch.
				INNER_JOIN(
					table.Application,
					table.Epoch.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			expr.AND(table.Application.State.EQ(enum.ApplicationState.Enabled)).
				AND(table.Application.ConsensusType.NOT_EQ(enum.Consensus.Prt)),
		).
		ORDER_BY(
			table.Epoch.ApplicationID,
			table.Epoch.Index.DESC(),
		)

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	epochs := map[int64]*model.Epoch{}
	for rows.Next() {
		var application model.Application
		var epoch model.Epoch
		err := rows.Scan(
			&epoch.ApplicationID,
			&epoch.Index,
			&epoch.FirstBlock,
			&epoch.LastBlock,
			&epoch.ClaimHash,
			&epoch.ClaimTransactionHash,
			&epoch.Status,
			&epoch.VirtualIndex,
			&epoch.CreatedAt,
			&epoch.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		epochs[application.ID] = &epoch
	}
	return epochs, nil
}

func (r *PostgresRepository) SelectSubmittedClaimPairsPerApp(ctx context.Context) (
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
		return nil, nil, nil, err
	}
	defer tx.Commit(ctx)

	computed, applications, err := r.selectOldestClaimPerApp(ctx, model.EpochStatus_ClaimComputed)
	if err != nil {
		return nil, nil, nil, err
	}

	acceptedOrSubmitted, err := r.selectNewestAcceptedClaimPerApp(ctx, true)
	if err != nil {
		return nil, nil, nil, err
	}

	return acceptedOrSubmitted, computed, applications, err
}

func (r *PostgresRepository) SelectAcceptedClaimPairsPerApp(ctx context.Context) (
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
		return nil, nil, nil, err
	}
	defer tx.Commit(ctx)

	submitted, applications, err := r.selectOldestClaimPerApp(ctx, model.EpochStatus_ClaimSubmitted)
	if err != nil {
		return nil, nil, nil, err
	}

	accepted, err := r.selectNewestAcceptedClaimPerApp(ctx, false)
	if err != nil {
		return nil, nil, nil, err
	}

	return accepted, submitted, applications, err
}

func (r *PostgresRepository) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	application_id int64,
	index uint64,
	transaction_hash common.Hash,
) error {
	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.ClaimTransactionHash,
			table.Epoch.Status,
		).
		SET(
			transaction_hash,
			postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(application_id)).
				AND(table.Epoch.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", index)))).
				AND(table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String()))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) UpdateEpochWithAcceptedClaim(
	ctx context.Context,
	application_id int64,
	index uint64,
) error {
	updStmt := table.Epoch.
		UPDATE(
			table.Epoch.Status,
		).
		SET(
			postgres.NewEnumValue(model.EpochStatus_ClaimAccepted.String()),
		).
		FROM(
			table.Application,
		).
		WHERE(
			table.Epoch.ApplicationID.EQ(postgres.Int64(application_id)).
				AND(table.Epoch.Index.EQ(postgres.RawFloat(fmt.Sprintf("%d", index)))).
				AND(table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String()))),
		)

	sqlStr, args := updStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNoUpdate
	}
	return nil
}
