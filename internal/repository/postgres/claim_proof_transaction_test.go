// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
)

func TestClaimProofWriterIsNotInvokedForComputedEpoch(t *testing.T) {
	repo := newEpochPublicationRepository(t)
	app, epoch := seedEpochPublication(t, repo)
	require.NoError(t, repo.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
	epoch.Commitment = new(repotest.UniqueHash())
	epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	require.NoError(t, repo.StoreClaimAndProofs(t.Context(), epoch, nil))
	called := false
	err := repo.StoreClaimAndProofs(t.Context(), epoch, func(context.Context, repository.ClaimProofWriter) error {
		called = true
		return nil
	})
	require.ErrorIs(t, err, repository.ErrNoUpdate)
	require.False(t, called)
}

func TestClaimProofWriterBetweenBatchesDoesNotLockApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	repo := newEpochPublicationRepository(t)
	app, epoch := seedEpochPublication(t, repo)
	require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, epoch.Index, repotest.DummyStateProof()))
	epoch.Commitment = new(repotest.UniqueHash())
	epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	output, err := repo.GetOutput(ctx, app.Name, 0)
	require.NoError(t, err)
	proof := repository.OutputProof{Index: output.Index, Hash: repotest.UniqueHash()}
	paused := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	completed := false
	t.Cleanup(func() {
		cancel()
		if !completed {
			select {
			case err := <-result:
				if err != nil {
					t.Logf("claim writer stopped during cleanup: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("claim writer did not stop after cancellation")
			}
		}
	})
	go func() {
		result <- repo.StoreClaimAndProofs(ctx, epoch, func(ctx context.Context, writer repository.ClaimProofWriter) error {
			if err := writer.StoreOutputProofs(ctx, []repository.OutputProof{proof}); err != nil {
				return err
			}
			close(paused)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return writer.StoreOutputProofs(ctx, []repository.OutputProof{proof})
		})
	}()
	select {
	case <-paused:
	case err := <-result:
		completed = true
		t.Fatalf("writer stopped before pause: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	control, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	defer control.Close(ctx)
	probe, err := control.Begin(ctx)
	require.NoError(t, err)
	var marker int
	lockErr := probe.QueryRow(ctx, "SELECT 1 FROM application WHERE id=$1 FOR UPDATE NOWAIT", app.ID).Scan(&marker)
	rollbackErr := probe.Rollback(ctx)
	require.NoError(t, lockErr)
	require.NoError(t, rollbackErr)
	require.Equal(t, 1, marker)
	_, err = control.Exec(ctx, "UPDATE application SET status='FAILED', reason='concurrent stop' WHERE id=$1", app.ID)
	require.NoError(t, err)
	close(release)
	select {
	case err := <-result:
		completed = true
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	state, err := repo.GetEpochCompletionState(ctx, app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Failed, state.Application.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, state.Epoch.Status)
	allowed, err := state.AllowsEpochCompletion()
	require.NoError(t, err)
	require.False(t, allowed)
	stored, err := repo.GetOutput(ctx, app.Name, output.Index)
	require.NoError(t, err)
	require.Equal(t, &proof.Hash, stored.Hash)
}

func TestClaimProofWriterUpdatesOnlyItsApplication(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, r)
	other, _ := seedEpochPublication(t, r, []byte("other application's output"))
	require.Equal(t, app.ID+1, other.ID)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
	epoch.Commitment = new(repotest.UniqueHash())
	epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	proof := repository.OutputProof{Index: 0, Hash: repotest.UniqueHash(), Siblings: make([]common.Hash, 63)}
	for i := range proof.Siblings {
		proof.Siblings[i] = repotest.UniqueHash()
	}
	err := r.StoreClaimAndProofs(t.Context(), epoch, func(ctx context.Context, writer repository.ClaimProofWriter) error {
		seen := false
		err := writer.ScanOutputs(ctx, 0, 0, func(index uint64, raw []byte) error {
			require.Zero(t, index)
			require.Equal(t, []byte("output"), raw)
			seen = true
			return nil
		})
		require.NoError(t, err)
		require.True(t, seen)
		return writer.StoreOutputProofs(ctx, []repository.OutputProof{proof})
	})
	require.NoError(t, err)
	for _, id := range []int64{app.ID, other.ID} {
		var hash []byte
		var siblings [][]byte
		require.NoError(t, r.db.QueryRow(t.Context(), `SELECT hash, output_hashes_siblings FROM output
   WHERE input_epoch_application_id=$1 AND index=0`, id).Scan(&hash, &siblings))
		if id == app.ID {
			require.Equal(t, proof.Hash.Bytes(), hash)
			require.Len(t, siblings, len(proof.Siblings))
			for i := range proof.Siblings {
				require.Equal(t, proof.Siblings[i].Bytes(), siblings[i])
			}
		} else {
			require.Nil(t, hash)
			require.Empty(t, siblings)
		}
	}
}
