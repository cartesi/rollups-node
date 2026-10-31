// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/repository"
)

type proofBatchTx struct {
	pgx.Tx
	batch  *pgx.Batch
	result *proofBatchResults
}

func (tx *proofBatchTx) SendBatch(_ context.Context, batch *pgx.Batch) pgx.BatchResults {
	tx.batch = batch
	return tx.result
}

type proofBatchResults struct {
	pgx.BatchResults
	affected          int
	execErr, closeErr error
	closed            bool
	calls             int
}

func (results *proofBatchResults) Exec() (pgconn.CommandTag, error) {
	results.calls++
	return pgconn.NewCommandTag("UPDATE " + strconv.Itoa(results.affected)), results.execErr
}

func (results *proofBatchResults) Close() error {
	results.closed = true
	return results.closeErr
}

func TestClaimProofWriterClosesResultsOnEveryOutcome(t *testing.T) {
	sentinel := errors.New("proof batch failed")
	for _, test := range []struct {
		name              string
		affected          int
		execErr, closeErr error
		wantErr           bool
	}{
		{name: "success", affected: 1},
		{name: "missing row", affected: 0, wantErr: true},
		{name: "multiple rows", affected: 2, wantErr: true},
		{name: "exec error", execErr: sentinel, wantErr: true},
		{name: "close error", affected: 1, closeErr: sentinel, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			results := &proofBatchResults{affected: test.affected, execErr: test.execErr, closeErr: test.closeErr}
			tx := &proofBatchTx{result: results}
			writer := &claimProofWriter{tx: tx, appID: 81}
			err := writer.StoreOutputProofs(t.Context(), []repository.OutputProof{{Index: 9}})
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if test.execErr != nil || test.closeErr != nil {
				require.ErrorIs(t, err, sentinel)
			}
			require.True(t, results.closed)
		})
	}
}
