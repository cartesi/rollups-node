// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		os.Exit(m.Run())
	}
	release, err := db.LockTestPostgres(context.Background(), endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	release()
	os.Exit(code)
}

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

func TestPostgresSchemaExecutionOutcomeContract(t *testing.T) {
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	require.NoError(t, db.SetupTestPostgres(endpoint))

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })

	rows, err := conn.Query(ctx, `
		SELECT enumlabel
		FROM pg_enum
		JOIN pg_type ON pg_type.oid = pg_enum.enumtypid
		WHERE pg_type.typname = 'InputCompletionStatus'
		ORDER BY enumsortorder`)
	require.NoError(t, err)
	labels, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.Equal(t, []string{"NONE", "ACCEPTED", "REJECTED", "EXCEPTION", "MACHINE_HALTED"}, labels)

	rows, err = conn.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'execution_parameters'`)
	require.NoError(t, err)
	columns, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.Contains(t, columns, "advance_max_cycles")
	require.Contains(t, columns, "inspect_max_cycles")
	require.Contains(t, columns, "advance_inc_cycles")
	require.Contains(t, columns, "inspect_inc_cycles")

	for _, column := range []string{"advance_max_cycles", "inspect_max_cycles"} {
		var defaultValue string
		err := conn.QueryRow(ctx, `
			SELECT column_default
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'execution_parameters'
			  AND column_name = $1`, column).Scan(&defaultValue)
		require.NoError(t, err)
		require.Equal(t, "0", defaultValue)
	}

	repo, err := factory.NewRepositoryFromConnectionString(ctx, endpoint)
	require.NoError(t, err)
	t.Cleanup(repo.Close)
	app := repotest.NewApplicationBuilder().Create(ctx, t, repo)
	for _, column := range []string{"advance_max_cycles", "inspect_max_cycles"} {
		for _, value := range []int64{0, int64(model.MaxExecutionCycleSpan)} {
			_, err := conn.Exec(ctx, fmt.Sprintf(
				`UPDATE execution_parameters SET %s = $1 WHERE application_id = $2`,
				column,
			), value, app.ID)
			require.NoError(t, err, "%s must accept %d", column, value)
		}
	}
	for _, test := range []struct {
		column string
		value  int64
	}{
		{"advance_inc_cycles", 0},
		{"inspect_inc_cycles", 0},
		{"advance_max_cycles", -1},
		{"inspect_max_cycles", -1},
		{"advance_max_cycles", int64(model.MaxExecutionCycles)},
		{"inspect_max_cycles", int64(model.MaxExecutionCycles)},
	} {
		_, err := conn.Exec(ctx, fmt.Sprintf(
			`UPDATE execution_parameters SET %s = $1 WHERE application_id = $2`,
			test.column,
		), test.value, app.ID)
		requirePostgresConstraint(t, err, "execution_parameters_"+test.column+"_check")
	}
}

func requirePostgresConstraint(t *testing.T, err error, constraint string) {
	t.Helper()
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, constraint, pgErr.ConstraintName)
}
