// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

type claimProofWriter struct {
	tx    pgx.Tx
	appID int64
}

func (writer *claimProofWriter) ScanOutputs(
	ctx context.Context, first, last uint64, fn repository.OutputScanFunc,
) error {
	return scanOutputs(ctx, writer.tx, writer.appID, first, last, fn)
}

func (writer *claimProofWriter) StoreOutputProofs(ctx context.Context, proofs []repository.OutputProof) error {
	if uint64(len(proofs)) > repository.MaxOutputRangeRows {
		return fmt.Errorf("output proof batch exceeds %d rows", repository.MaxOutputRangeRows)
	}
	if len(proofs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, proof := range proofs {
		statement := table.Output.UPDATE(table.Output.Hash, table.Output.OutputHashesSiblings).
			SET(proof.Hash.Bytes(), encodeSiblings(proof.Siblings)).
			WHERE(table.Output.InputEpochApplicationID.EQ(postgres.Int64(writer.appID)).
				AND(table.Output.Index.EQ(uint64Expr(proof.Index))))
		query, args := statement.Sql()
		batch.Queue(query, args...)
	}
	results := writer.tx.SendBatch(ctx, batch)
	for _, proof := range proofs {
		command, err := results.Exec()
		if err != nil {
			_ = results.Close()
			return fmt.Errorf("failed to insert proof for output '%d': %w", proof.Index, err)
		}
		if command.RowsAffected() != 1 {
			_ = results.Close()
			return fmt.Errorf("failed to insert proof for output '%d': expected one affected row", proof.Index)
		}
	}
	return results.Close()
}
