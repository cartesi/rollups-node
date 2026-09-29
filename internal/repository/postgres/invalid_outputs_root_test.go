// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestPostgresInvalidOutputsRootRollback(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))
	ctx := t.Context()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })

	seed := repotest.Seed(ctx, t, repo)
	result := &model.AdvanceResult{
		EpochIndex:          seed.Epoch.Index,
		InputIndex:          seed.Input.Index,
		Status:              model.InputCompletionStatus_InvalidOutputsRoot,
		StateProof:          *repotest.DummyStateProof(),
		IsDaveConsensus:     true,
		PeriodicStateHashes: [][32]byte{repotest.UniqueHash()},
		PaddingRepetitions:  model.InputHashCollectionCapacity - 1,
	}
	// Reject the final application update after StoreAdvanceResult has written
	// the hash collection, input result, and epoch proof. All must roll back.
	_, err = conn.Exec(ctx, `ALTER TABLE application ADD CONSTRAINT invalid_outputs_root_test_rejection
		CHECK (status <> 'INVALID_OUTPUTS_ROOT') NOT VALID`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := conn.Exec(context.Background(), `ALTER TABLE application DROP CONSTRAINT invalid_outputs_root_test_rejection`)
		require.NoError(t, err)
	})
	err = repo.StoreAdvanceResult(ctx, seed.App.ID, result)
	requirePostgresConstraint(t, err, "invalid_outputs_root_test_rejection")
	input, err := repo.GetInput(ctx, seed.App.Name, seed.Input.Index)
	require.NoError(t, err)
	require.Equal(t, model.InputCompletionStatus_None, input.Status)
	require.Nil(t, input.MachineHash)
	require.Nil(t, input.TxBufferDataBlock)
	epoch, err := repo.GetEpoch(ctx, seed.App.Name, seed.Epoch.Index)
	require.NoError(t, err)
	require.False(t, epoch.HasCompleteStateProof())
	require.Nil(t, epoch.MachineHash)
	require.Nil(t, epoch.TxBufferDataBlock)
	app, err := repo.GetApplication(ctx, seed.App.Name)
	require.NoError(t, err)
	require.Zero(t, app.ProcessedInputs)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	require.Nil(t, app.Reason)
	hashes, total, err := repo.ListStateHashes(ctx, seed.App.Name,
		repository.StateHashFilter{}, repository.Pagination{}, false)
	require.NoError(t, err)
	require.Empty(t, hashes)
	require.Zero(t, total)
}

func TestPostgresInvalidOutputsRootInputContract(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))
	ctx := t.Context()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })
	seed := repotest.Seed(ctx, t, repo)

	_, err = conn.Exec(ctx, `UPDATE input SET status = 'INVALID_OUTPUTS_ROOT'
		WHERE epoch_application_id = $1 AND index = $2`, seed.App.ID, seed.Input.Index)
	requirePostgresConstraint(t, err, "input_completed_hashes_check")
	proof := repotest.DummyStateProof()
	require.NoError(t, repo.StoreAdvanceResult(ctx, seed.App.ID, &model.AdvanceResult{
		EpochIndex: seed.Epoch.Index,
		InputIndex: seed.Input.Index,
		Status:     model.InputCompletionStatus_InvalidOutputsRoot,
		StateProof: *proof,
	}))
	_, err = conn.Exec(ctx, `UPDATE input SET status = 'ACCEPTED'
		WHERE epoch_application_id = $1 AND index = $2`, seed.App.ID, seed.Input.Index)
	require.ErrorContains(t, err, "completed input result is immutable")
	_, err = conn.Exec(ctx, `UPDATE input SET machine_hash = $3
		WHERE epoch_application_id = $1 AND index = $2`, seed.App.ID, seed.Input.Index, repotest.UniqueHash().Bytes())
	require.ErrorContains(t, err, "completed input result is immutable")
	stored, err := repo.GetInput(ctx, seed.App.Name, seed.Input.Index)
	require.NoError(t, err)
	require.Equal(t, model.InputCompletionStatus_InvalidOutputsRoot, stored.Status)
	require.Equal(t, &proof.MachineHash, stored.MachineHash)
}
