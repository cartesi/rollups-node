// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"bytes"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func removalRunRepository(t *testing.T) repository.Repository {
	t.Helper()
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skip(err)
	}
	unlock, err := db.LockTestPostgres(t.Context(), endpoint)
	require.NoError(t, err)
	t.Cleanup(unlock)
	require.NoError(t, db.SetupTestPostgres(endpoint))
	repo, err := postgres.NewPostgresRepository(t.Context(), endpoint, 1, 0)
	require.NoError(t, err)
	t.Cleanup(repo.Close)

	// Removal must not need an RPC endpoint or submission registration.
	for key, value := range map[string]string{
		config.DATABASE_CONNECTION:           endpoint,
		config.BLOCKCHAIN_HTTP_ENDPOINT:      "",
		config.BLOCKCHAIN_HTTP_ENDPOINT_FILE: "",
	} {
		previous := viper.Get(key)
		t.Cleanup(func() { viper.Set(key, previous) })
		viper.Set(key, value)
	}
	for name, value := range map[string]string{"yes": "true", "allow-abandonment": "false", "timeout": "5s"} {
		flag := Cmd.Flags().Lookup(name)
		previous, changed := flag.Value.String(), flag.Changed
		t.Cleanup(func() {
			require.NoError(t, flag.Value.Set(previous))
			flag.Changed = changed
		})
		require.NoError(t, Cmd.Flags().Set(name, value))
	}
	return repo
}

func TestRunRemovalWithYesRequiresRiskAcknowledgment(t *testing.T) {
	// These tests mutate the command flags and Viper configuration, so they must
	// stay sequential even though each repository connection is independent.
	repo := removalRunRepository(t)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).
		WithEnabled(false).Create(t.Context(), t, repo)
	before, err := repo.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.NotNil(t, before)
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetContext(t.Context())
	command.SetOut(&output)

	err = run(command, []string{app.Name})
	require.ErrorContains(t, err, "--allow-abandonment")
	require.Empty(t, output.String())
	after, err := repo.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, before, after, "--yes must leave the risky application intact")

	require.NoError(t, Cmd.Flags().Set("allow-abandonment", "true"))
	output.Reset()
	require.NoError(t, run(command, []string{app.Name}))
	require.Contains(t, output.String(), "Transactions already sent or in progress may still execute")
	require.Contains(t, output.String(), "successfully removed")
	after, err = repo.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Nil(t, after, "explicit abandonment must reach the actual database deletion")
}

func TestRunRemovalWithAbandonmentStillRejectsEnabledApplication(t *testing.T) {
	repo := removalRunRepository(t)
	app := repotest.NewApplicationBuilder().WithEnabled(true).Create(t.Context(), t, repo)
	before, err := repo.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.NotNil(t, before)
	require.NoError(t, Cmd.Flags().Set("allow-abandonment", "true"))
	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetContext(t.Context())
	command.SetOut(&output)

	require.ErrorContains(t, run(command, []string{app.Name}), "must be disabled before removal")
	require.Empty(t, output.String())
	after, err := repo.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, before, after, "neither confirmation flag can remove an enabled application")
}
