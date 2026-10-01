// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
)

func TestInsertAdvanceDataQualifiesPublicSchema(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))
	ctx := t.Context()
	repo, err := NewPostgresRepository(ctx, endpoint, 1, 0)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	seed := repotest.Seed(ctx, t, repo)
	tx, err := repo.(*PostgresRepository).db.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE output (LIKE public.output INCLUDING ALL) ON COMMIT DROP;
		CREATE TEMP TABLE report (LIKE public.report INCLUDING ALL) ON COMMIT DROP;
		SET LOCAL search_path TO pg_temp, public`)
	require.NoError(t, err)
	payloads := [][]byte{[]byte("public evidence")}
	require.NoError(t, insertOutputs(ctx, tx, seed.App.ID, 0, payloads))
	require.NoError(t, insertReports(ctx, tx, seed.App.ID, 0, payloads))
	for _, tableName := range []string{table.Output.TableName(), table.Report.TableName()} {
		t.Run(tableName, func(t *testing.T) {
			var publicCount, shadowCount uint64
			require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM public."+tableName).Scan(&publicCount))
			require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM pg_temp."+tableName).Scan(&shadowCount))
			require.Equal(t, uint64(1), publicCount)
			require.Zero(t, shadowCount)
		})
	}
	require.NoError(t, tx.Commit(ctx))
}
