// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}

	repotest.RunAllSuites(t, func(ctx context.Context, t *testing.T) (repository.Repository, func()) {
		t.Helper()
		err := db.SetupTestPostgres(endpoint)
		require.NoError(t, err)

		repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
		require.NoError(t, err)

		return repo, func() { repo.Close() }
	})
}
