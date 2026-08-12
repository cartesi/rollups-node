// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestPostgresInputExceptionDataContract(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)

	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })

	seed := repotest.Seed(ctx, t, repo)
	machineHash := repotest.UniqueHash()
	outputsHash := repotest.UniqueHash()

	_, err = conn.Exec(ctx, `
		UPDATE input
		SET status = 'EXCEPTION', machine_hash = $2, outputs_hash = $3
		WHERE epoch_application_id = $1 AND index = 0`,
		seed.App.ID, machineHash[:], outputsHash[:],
	)
	requirePostgresConstraint(t, err, "input_exception_data_check")

	_, err = conn.Exec(ctx, `
		UPDATE input
		SET status = 'ACCEPTED', exception_data = '\x01', machine_hash = $2, outputs_hash = $3
		WHERE epoch_application_id = $1 AND index = 0`,
		seed.App.ID, machineHash[:], outputsHash[:],
	)
	requirePostgresConstraint(t, err, "input_exception_data_check")

	require.NoError(t, repo.StoreAdvanceResult(ctx, seed.App.ID, &model.AdvanceResult{
		EpochIndex:    seed.Epoch.Index,
		InputIndex:    seed.Input.Index,
		Status:        model.InputCompletionStatus_Exception,
		ExceptionData: []byte{},
		OutputsProof: model.OutputsProof{
			MachineHash: machineHash,
			OutputsHash: outputsHash,
		},
	}))
	completed, err := repo.GetInput(ctx, seed.App.IApplicationAddress.String(), seed.Input.Index)
	require.NoError(t, err)
	require.NotNil(t, completed.ExceptionData)
	require.Empty(t, completed.ExceptionData)
}
