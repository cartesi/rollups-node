// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package status

import (
	"context"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func statusRepository(t *testing.T) (repository.Repository, string) {
	t.Helper()
	endpoint, err := db.GetTestDatabaseEndpoint()
	if err != nil {
		t.Skip(err)
	}
	unlock, err := db.LockTestPostgres(t.Context(), endpoint)
	require.NoError(t, err)
	t.Cleanup(unlock)
	require.NoError(t, db.SetupTestPostgres(endpoint))
	r, err := postgres.NewPostgresRepository(t.Context(), endpoint, 1, 0)
	require.NoError(t, err)
	t.Cleanup(r.Close)
	previous := viper.Get(config.DATABASE_CONNECTION)
	viper.Set(config.DATABASE_CONNECTION, endpoint)
	t.Cleanup(func() { viper.Set(config.DATABASE_CONNECTION, previous) })
	originalContext, originalYes := Cmd.Context(), yesFlag
	yesFlag = false
	t.Cleanup(func() { Cmd.SetContext(originalContext); Cmd.SetArgs(nil); yesFlag = originalYes })
	return r, endpoint
}

func TestExplicitEnableResetsAlreadyEnabledHealthyApplication(t *testing.T) {
	r, _ := statusRepository(t)
	for _, spelling := range []string{"enabled", "enable"} {
		t.Run(spelling, func(t *testing.T) {
			app := repotest.NewApplicationBuilder().WithEnabled(true).Create(t.Context(), t, r)
			key := repository.DeploymentIdentity{ApplicationID: app.ID, ApplicationAddress: app.IApplicationAddress,
				ConsensusAddress: app.IConsensusAddress}
			_, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
				ConfirmedTier: new(model.DeploymentTierTable), Reason: "old confirmed table"})
			require.NoError(t, err)
			// Execute the actual Cobra dispatch. No replacement repository or command seam is used.
			Cmd.SetArgs([]string{app.Name, spelling})
			require.NoError(t, Cmd.ExecuteContext(t.Context()))
			state, err := r.GetDeploymentAdmissionState(t.Context(), key)
			require.NoError(t, err)
			require.EqualValues(t, 1, state.ResetGeneration)
			require.Nil(t, state.ConfirmedTier)
			got, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.True(t, got.Enabled)
			require.Equal(t, model.ApplicationStatus_OK, got.Status)
			Cmd.SetArgs([]string{app.Name, "enabled"})
			require.NoError(t, Cmd.ExecuteContext(t.Context()))
			state, err = r.GetDeploymentAdmissionState(t.Context(), key)
			require.NoError(t, err)
			require.EqualValues(t, 2, state.ResetGeneration)
			_, err = r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
				ResetGeneration: state.ResetGeneration, ConfirmedTier: new(model.DeploymentTierGeneration),
				Reason: "preserve across disable"})
			require.NoError(t, err)
			before, err := r.GetDeploymentAdmissionState(t.Context(), key)
			require.NoError(t, err)
			Cmd.SetArgs([]string{app.Name, "disabled"})
			require.NoError(t, Cmd.ExecuteContext(t.Context()))
			after, err := r.GetDeploymentAdmissionState(t.Context(), key)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestExplicitEnablePreservesFailureNotObservedByCLI(t *testing.T) {
	for _, skipPrompt := range []bool{false, true} {
		name := "without yes"
		if skipPrompt {
			name = "with yes"
		}
		t.Run(name, func(t *testing.T) {
			r, endpoint := statusRepository(t)
			app := repotest.NewApplicationBuilder().WithEnabled(false).Create(t.Context(), t, r)
			key := repository.DeploymentIdentity{ApplicationID: app.ID, ApplicationAddress: app.IApplicationAddress,
				ConsensusAddress: app.IConsensusAddress}
			_, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
				ConfirmedTier: new(model.DeploymentTierTable), Reason: "old table"})
			require.NoError(t, err)
			control, err := pgx.Connect(t.Context(), endpoint)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, control.Close(context.Background())) })
			failure, err := control.Begin(t.Context())
			require.NoError(t, err)
			t.Cleanup(func() { _ = failure.Rollback(context.Background()) })
			_, err = failure.Exec(t.Context(), "UPDATE application SET status='FAILED',reason=$2 WHERE id=$1", app.ID, "new failure")
			require.NoError(t, err)
			args := []string{app.Name, "enabled"}
			if skipPrompt {
				args = append(args, "--yes")
			}
			Cmd.SetArgs(args)
			done := make(chan error, 1)
			go func() { done <- Cmd.ExecuteContext(t.Context()) }()
			// The uncommitted failure is invisible to the CLI's read. Its enable then waits for this transaction.
			var waiting bool
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				_, err := failure.Exec(t.Context(), "SELECT pg_stat_clear_snapshot()")
				require.NoError(t, err)
				err = failure.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
					WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid))
					AND query LIKE '%UPDATE public.application%')`, control.PgConn().PID()).Scan(&waiting)
				require.NoError(t, err)
				if waiting {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			require.True(t, waiting, "CLI enable must wait after reading the old healthy status")
			require.NoError(t, failure.Commit(t.Context()))
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("CLI enable did not finish after the failure committed")
			}
			got, err := r.GetApplication(t.Context(), app.Name)
			require.NoError(t, err)
			require.True(t, got.Enabled)
			require.Equal(t, model.ApplicationStatus_Failed, got.Status)
			require.Equal(t, new("new failure"), got.Reason)
			state, err := r.GetDeploymentAdmissionState(t.Context(), key)
			require.NoError(t, err)
			require.EqualValues(t, 1, state.ResetGeneration)
			require.Nil(t, state.ConfirmedTier)
			require.Nil(t, state.DeploymentReason)
		})
	}
}

func TestExplicitEnableClearsObservedFailureWithYes(t *testing.T) {
	r, _ := statusRepository(t)
	for _, enabled := range []bool{false, true} {
		app := repotest.NewApplicationBuilder().WithEnabled(enabled).Create(t.Context(), t, r)
		require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Failed, new("repaired failure")))
		Cmd.SetArgs([]string{app.Name, "enable", "--yes"})
		require.NoError(t, Cmd.ExecuteContext(t.Context()))
		got, err := r.GetApplication(t.Context(), app.Name)
		require.NoError(t, err)
		require.True(t, got.Enabled)
		require.Equal(t, model.ApplicationStatus_OK, got.Status)
		require.Nil(t, got.Reason)
		state, err := r.GetDeploymentAdmissionState(t.Context(), repository.DeploymentIdentity{ApplicationID: app.ID,
			ApplicationAddress: app.IApplicationAddress, ConsensusAddress: app.IConsensusAddress})
		require.NoError(t, err)
		require.EqualValues(t, 1, state.ResetGeneration)
	}
}
