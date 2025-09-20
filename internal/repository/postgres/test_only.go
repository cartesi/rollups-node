// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func (r *PostgresRepository) CreateEpoch(
	ctx context.Context,
	e *model.Epoch,
) error {
	insertStmt := table.Epoch.INSERT(
		table.Epoch.ApplicationID,
		table.Epoch.Index,
		table.Epoch.FirstBlock,
		table.Epoch.LastBlock,
		table.Epoch.InputIndexLowerBound,
		table.Epoch.InputIndexUpperBound,
		table.Epoch.MachineHash,
		table.Epoch.ClaimHash,
		table.Epoch.ClaimTransactionHash,
		table.Epoch.TournamentAddress,
		table.Epoch.Commitment,
		table.Epoch.Status,
		table.Epoch.VirtualIndex,
	).VALUES(
		e.ApplicationID,
		e.Index,
		e.FirstBlock,
		e.LastBlock,
		e.InputIndexLowerBound,
		e.InputIndexUpperBound,
		e.MachineHash,
		e.ClaimHash,
		e.ClaimTransactionHash,
		e.TournamentAddress,
		e.Commitment,
		e.Status,
		e.VirtualIndex,
	)

	sqlStr, args := insertStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) CreateInput(
	ctx context.Context,
	inp *model.Input,
) error {
	insertStmt := table.Input.INSERT(
		table.Input.EpochApplicationID,
		table.Input.EpochIndex,
		table.Input.Index,
		table.Input.BlockNumber,
		table.Input.RawData,
		table.Input.Status,
		table.Input.MachineHash,
		table.Input.OutputsHash,
		table.Input.TransactionReference,
		table.Input.SnapshotURI,
	).VALUES(
		inp.EpochApplicationID,
		inp.EpochIndex,
		inp.Index,
		inp.BlockNumber,
		inp.RawData,
		inp.Status,
		inp.MachineHash,
		inp.OutputsHash,
		inp.TransactionReference,
		inp.SnapshotURI,
	)

	sqlStr, args := insertStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) CreateOutput(
	ctx context.Context,
	out *model.Output,
) error {
	insertStmt := table.Output.INSERT(
		table.Output.InputEpochApplicationID,
		table.Output.InputIndex,
		table.Output.Index,
		table.Output.RawData,
		table.Output.Hash,
		table.Output.OutputHashesSiblings,
		table.Output.ExecutionTransactionHash,
	).VALUES(
		out.InputEpochApplicationID,
		out.InputIndex,
		out.Index,
		out.RawData,
		out.Hash,
		out.OutputHashesSiblings,
		out.ExecutionTransactionHash,
	)

	sqlStr, args := insertStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) CreateReport(
	ctx context.Context,
	out *model.Report,
) error {
	insertStmt := table.Report.INSERT(
		table.Report.InputEpochApplicationID,
		table.Report.InputIndex,
		table.Report.Index,
		table.Report.RawData,
	).VALUES(
		out.InputEpochApplicationID,
		out.InputIndex,
		out.Index,
		out.RawData,
	)

	sqlStr, args := insertStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}
