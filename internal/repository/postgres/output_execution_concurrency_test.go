// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreClaimAndProofsConcurrentOutputExecution(t *testing.T) {
	for _, test := range []struct {
		name           string
		requestedBlock uint64
		expectedBlock  uint64
	}{
		{name: "advancing cursor", requestedBlock: 200, expectedBlock: 200},
		{name: "older cursor", requestedBlock: 50, expectedBlock: 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			repo := newEpochPublicationRepository(t)
			app, epoch := seedEpochPublication(t, repo)
			require.NoError(t, repo.UpdateEventLastCheckBlock(ctx, []int64{app.ID}, model.MonitoredEvent_OutputExecuted, 100))
			require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, epoch.Index, repotest.DummyStateProof()))
			epoch, err := repo.GetEpoch(ctx, app.Name, epoch.Index)
			require.NoError(t, err)
			epoch.Commitment = new(repotest.UniqueHash())
			epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			output, err := repo.GetOutput(ctx, app.Name, 0)
			require.NoError(t, err)
			require.NotNil(t, output)
			proofOutput := *output
			proofOutput.Hash = new(repotest.UniqueHash())
			proofOutput.OutputHashesSiblings = []common.Hash{repotest.UniqueHash()}
			executedOutput := *output
			executedOutput.ExecutionTransactionHash = new(repotest.UniqueHash())

			endpoint, err := db.GetTestDatabaseEndpoint()
			require.NoError(t, err)
			control, err := pgx.Connect(ctx, endpoint)
			require.NoError(t, err)
			unlock := func(ctx context.Context) error {
				_, err := control.Exec(ctx, `SELECT pg_advisory_unlock_all()`)
				return err
			}
			var writers sync.WaitGroup
			t.Cleanup(func() {
				// Cancel and release the gate before repository cleanup, including after a failed assertion.
				cancel()
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cleanupCancel()
				assert.NoError(t, unlock(cleanupCtx))
				done := make(chan struct{})
				go func() {
					writers.Wait()
					close(done)
				}()
				select {
				case <-done:
				case <-cleanupCtx.Done():
					t.Error("concurrent repository writers did not stop after cancellation")
				}
				_, err := control.Exec(cleanupCtx, `
					DROP TRIGGER IF EXISTS pause_output_execution_for_test ON output;
					DROP FUNCTION IF EXISTS pause_output_execution_for_test()`)
				assert.NoError(t, err)
				assert.NoError(t, control.Close(cleanupCtx))
			})
			// An AFTER trigger pauses the execution writer with its output row locked.
			// Both repository operations use their real transactions and SQL unchanged.
			_, err = control.Exec(ctx, `
				CREATE FUNCTION pause_output_execution_for_test() RETURNS TRIGGER AS $$
				BEGIN
					IF NEW.execution_transaction_hash IS DISTINCT FROM OLD.execution_transaction_hash
						AND NEW.execution_transaction_hash IS NOT NULL THEN
						PERFORM pg_advisory_xact_lock(3890003);
					END IF;
					RETURN NEW;
				END;
				$$ LANGUAGE plpgsql;
				CREATE TRIGGER pause_output_execution_for_test AFTER UPDATE ON output
				FOR EACH ROW EXECUTE FUNCTION pause_output_execution_for_test()`)
			require.NoError(t, err)
			_, err = control.Exec(ctx, `SELECT pg_advisory_lock(3890003)`)
			require.NoError(t, err)

			type result struct {
				writer string
				err    error
			}
			results := make(chan result, 2)
			writers.Add(1)
			go func() {
				defer writers.Done()
				err := repo.UpdateOutputsExecution(ctx, app.Name, []*model.Output{&executedOutput}, test.requestedBlock)
				results <- result{writer: "UpdateOutputsExecution", err: err}
			}()
			var executionPID int32
			require.Eventually(t, func() bool {
				err := control.QueryRow(ctx, `SELECT pid FROM pg_locks
					WHERE locktype = 'advisory' AND objid = 3890003 AND NOT granted`).Scan(&executionPID)
				return err == nil
			}, 5*time.Second, 10*time.Millisecond, "execution writer must reach the gate with its output locked")
			writers.Add(1)
			go func() {
				defer writers.Done()
				err := repo.StoreClaimAndProofs(ctx, epoch, repotest.WriteProofs(&proofOutput))
				results <- result{writer: "StoreClaimAndProofs", err: err}
			}()
			require.Eventually(t, func() bool {
				var blocked bool
				err := control.QueryRow(ctx, `SELECT EXISTS (
					SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
					AND pid <> $1 AND $1 = ANY(pg_blocking_pids(pid)))`, executionPID).Scan(&blocked)
				return err == nil && blocked
			}, 5*time.Second, 10*time.Millisecond, "claim publisher must contend with the execution writer before release")
			require.NoError(t, unlock(ctx))
			for range 2 {
				select {
				case result := <-results:
					require.NoError(t, result.err, result.writer)
				case <-ctx.Done():
					t.Fatal("concurrent publication and execution did not complete: ", ctx.Err())
				}
			}

			stored, err := repo.GetOutput(ctx, app.Name, output.Index)
			require.NoError(t, err)
			require.NotNil(t, stored)
			require.Equal(t, proofOutput.Hash, stored.Hash)
			require.Equal(t, proofOutput.OutputHashesSiblings, stored.OutputHashesSiblings)
			require.Equal(t, executedOutput.ExecutionTransactionHash, stored.ExecutionTransactionHash)
			storedEpoch, err := repo.GetEpoch(ctx, app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_ClaimComputed, storedEpoch.Status)
			require.Equal(t, epoch.Commitment, storedEpoch.Commitment)
			require.Equal(t, epoch.CommitmentProof, storedEpoch.CommitmentProof)
			cursor, err := repo.GetEventLastCheckBlock(ctx, app.ID, model.MonitoredEvent_OutputExecuted)
			require.NoError(t, err)
			require.Equal(t, test.expectedBlock, cursor, "concurrent writers must not move the cursor backwards")
		})
	}
}
