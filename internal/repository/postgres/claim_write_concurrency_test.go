// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
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

func TestStoreClaimAndProofsDoesNotLockApplicationAcrossProofBatch(t *testing.T) {
	for _, stop := range []model.ApplicationStatus{
		model.ApplicationStatus_Failed, model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted,
	} {
		t.Run(stop.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			repo := newEpochPublicationRepository(t)
			app, epoch := seedEpochPublication(t, repo)
			require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, epoch.Index, repotest.DummyStateProof()))
			epoch, err := repo.GetEpoch(ctx, app.Name, epoch.Index)
			require.NoError(t, err)
			epoch.Commitment = new(repotest.UniqueHash())
			epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			output, err := repo.GetOutput(ctx, app.Name, 0)
			require.NoError(t, err)
			output.Hash = new(repotest.UniqueHash())
			output.OutputHashesSiblings = []common.Hash{repotest.UniqueHash()}

			endpoint, err := db.GetTestDatabaseEndpoint()
			require.NoError(t, err)
			control, err := pgx.Connect(ctx, endpoint)
			require.NoError(t, err)
			result := make(chan error, 1)
			started := false
			finished := false
			t.Cleanup(func() {
				cancel()
				cleanup, stopCleanup := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCleanup()
				_, unlockErr := control.Exec(cleanup, `SELECT pg_advisory_unlock_all()`)
				assert.NoError(t, unlockErr)
				if started && !finished {
					select {
					case claimErr := <-result:
						if claimErr != nil {
							t.Logf("claim writer stopped during cleanup: %v", claimErr)
						}
					case <-cleanup.Done():
						t.Error("claim writer did not stop after cancellation")
					}
				}
				_, dropErr := control.Exec(cleanup, `DROP TRIGGER IF EXISTS pause_claim_proofs_for_test ON output;
					DROP FUNCTION IF EXISTS pause_claim_proofs_for_test()`)
				assert.NoError(t, dropErr)
				assert.NoError(t, control.Close(cleanup))
			})
			// Pause the real proof write after its epoch CAS and snapshot guard.
			_, err = control.Exec(ctx, `
				CREATE FUNCTION pause_claim_proofs_for_test() RETURNS TRIGGER AS $$
				BEGIN
					IF NEW.hash IS DISTINCT FROM OLD.hash AND NEW.hash IS NOT NULL THEN
						PERFORM pg_advisory_xact_lock(3890004);
					END IF;
					RETURN NEW;
				END;
				$$ LANGUAGE plpgsql;
				CREATE TRIGGER pause_claim_proofs_for_test AFTER UPDATE ON output
				FOR EACH ROW EXECUTE FUNCTION pause_claim_proofs_for_test();
				SELECT pg_advisory_lock(3890004)`)
			require.NoError(t, err)
			started = true
			go func() { result <- repo.StoreClaimAndProofs(ctx, epoch, []*model.Output{output}) }()
			require.Eventually(t, func() bool {
				var waiting bool
				err := control.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
					WHERE locktype='advisory' AND objid=3890004 AND NOT granted)`).Scan(&waiting)
				return err == nil && waiting
			}, 5*time.Second, 10*time.Millisecond, "claim writer must reach the proof batch")

			// FOR UPDATE also conflicts with KEY SHARE, unlike the status
			// update below. Roll the probe back before testing that update.
			probe, err := control.Begin(ctx)
			require.NoError(t, err)
			var marker int
			lockErr := probe.QueryRow(ctx, `SELECT 1 FROM application WHERE id=$1 FOR UPDATE NOWAIT`, app.ID).Scan(&marker)
			rollbackErr := probe.Rollback(ctx)
			require.NoError(t, lockErr, "claim proof batch must not hold any application row lock")
			require.NoError(t, rollbackErr)
			require.Equal(t, 1, marker)

			stopCtx, stopCancel := context.WithTimeout(ctx, 3*time.Second)
			err = repo.UpdateApplicationStatus(stopCtx, app.ID, stop, new("concurrent stop"))
			stopCancel()
			require.NoError(t, err, "application updates must not wait for the proof batch")
			_, err = control.Exec(ctx, `SELECT pg_advisory_unlock_all()`)
			require.NoError(t, err)
			select {
			case err := <-result:
				finished = true
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("claim writer did not finish: ", ctx.Err())
			}

			state, err := repo.GetEpochCompletionState(ctx, app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, stop, state.Application.Status)
			require.Equal(t, model.EpochStatus_ClaimComputed, state.Epoch.Status,
				"valid local work can commit after a concurrent stop")
			require.True(t, claimValidated(t, repo.(*PostgresRepository), app.ID, epoch.Index),
				"a completed validation remains qualified after the concurrent application stop")
			require.Equal(t, epoch.Commitment, state.Epoch.Commitment)
			storedOutput, err := repo.GetOutput(ctx, app.Name, output.Index)
			require.NoError(t, err)
			require.Equal(t, output.Hash, storedOutput.Hash)
			require.Equal(t, output.OutputHashesSiblings, storedOutput.OutputHashesSiblings)
			allowed, err := state.AllowsEpochCompletion()
			require.NoError(t, err)
			require.False(t, allowed, "persisted local work does not authorize publication under the stop")
		})
	}
}
