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
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

var (
	ErrNoUpdate = fmt.Errorf("update did not take effect")
)

// Retrieve the computed claim of each application with the smallest index.
// The query may return either 0 or 1 entries per application.
func (r *postgresRepository) SelectOldestComputedClaimPerApp(ctx context.Context) (
	map[common.Address]*model.ClaimRow,
	error,
) {
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
		table.Application.IapplicationAddress,
		table.Application.IconsensusAddress,
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
			table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimComputed.String())).
				AND(table.Application.State.EQ(postgres.NewEnumValue(model.ApplicationState_Enabled.String()))),
		).
		ORDER_BY(
			table.Epoch.ApplicationID,
			table.Epoch.Index.ASC(),
		)

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := map[common.Address]*model.ClaimRow{}
	for rows.Next() {
		var cr model.ClaimRow
		err := rows.Scan(
			&cr.ApplicationID,
			&cr.Index,
			&cr.FirstBlock,
			&cr.LastBlock,
			&cr.ClaimHash,
			&cr.ClaimTransactionHash,
			&cr.Status,
			&cr.VirtualIndex,
			&cr.CreatedAt,
			&cr.UpdatedAt,
			&cr.IApplicationAddress,
			&cr.IConsensusAddress,
		)
		if err != nil {
			return nil, err
		}
		results[cr.IApplicationAddress] = &cr
	}
	return results, nil
}

// Retrieve the newest accepted claim of each application
func (r *postgresRepository) SelectNewestSubmittedOrAcceptedClaimPerApp(ctx context.Context) (
	map[common.Address]*model.ClaimRow,
	error,
) {
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
		table.Application.IapplicationAddress,
		table.Application.IconsensusAddress,
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
			table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimSubmitted.String())).
				OR(table.Epoch.Status.EQ(postgres.NewEnumValue(model.EpochStatus_ClaimAccepted.String()))).
				AND(table.Application.State.EQ(postgres.NewEnumValue(model.ApplicationState_Enabled.String()))),
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

	results := map[common.Address]*model.ClaimRow{}
	for rows.Next() {
		var cr model.ClaimRow
		err := rows.Scan(
			&cr.ApplicationID,
			&cr.Index,
			&cr.FirstBlock,
			&cr.LastBlock,
			&cr.ClaimHash,
			&cr.ClaimTransactionHash,
			&cr.Status,
			&cr.VirtualIndex,
			&cr.CreatedAt,
			&cr.UpdatedAt,
			&cr.IApplicationAddress,
			&cr.IConsensusAddress,
		)
		if err != nil {
			return nil, err
		}
		results[cr.IApplicationAddress] = &cr
	}
	return results, nil
}

func (r *postgresRepository) SelectClaimPairsPerApp(ctx context.Context) (
	map[common.Address]*model.ClaimRow,
	map[common.Address]*model.ClaimRow,
	error,
) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Commit(ctx)

	computed, err := r.SelectOldestComputedClaimPerApp(ctx)
	if err != nil {
		return nil, nil, err
	}

	accepted, err := r.SelectNewestSubmittedOrAcceptedClaimPerApp(ctx)
	if err != nil {
		return nil, nil, err
	}

	return computed, accepted, err
}

func (r *postgresRepository) UpdateEpochWithSubmittedClaim(
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
