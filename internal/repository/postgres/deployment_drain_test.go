// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const (
	deploymentDrainMissingCase = "missing"
	deploymentDrainTableCase   = "table"
)

func deploymentDrainFixture(t *testing.T, computed bool) (
	*PostgresRepository, *model.Application, *model.Epoch, repository.DeploymentDrainRequest,
) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, r)
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
	require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 20, repotest.UniqueHash(), 20))
	_, err := r.db.Exec(t.Context(), `UPDATE application SET last_epoch_check_block=20,last_input_check_block=20 WHERE id=$1`, app.ID)
	require.NoError(t, err)
	key := deploymentKey(app)
	readDeployment(t, r, key)
	deploymentRestrict(t, r, key, 0, model.DeploymentTierGeneration, "fresh confirmed generation")
	if computed {
		completeDeploymentDrainProofs(t, r, app, epoch)
	}
	epoch, err = r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	return r, app, epoch, repository.DeploymentDrainRequest{Identity: key, EpochIndex: epoch.Index}
}

func completeDeploymentDrainProofs(t *testing.T, r *PostgresRepository, app *model.Application, epoch *model.Epoch) {
	t.Helper()
	current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	current.Commitment = new(repotest.UniqueHash())
	current.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	output, err := r.GetOutput(t.Context(), app.Name, 0)
	require.NoError(t, err)
	output.Hash = new(repotest.UniqueHash())
	output.OutputHashesSiblings = []common.Hash{repotest.UniqueHash()}
	// Use a complete nonempty callback. Arbitrary nil callers do not prove proof completeness.
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), current, repotest.WriteProofs(output)))
}

func TestDeploymentDrainWaitsForProtectedIPAndPreservesCompletedProofs(t *testing.T) {
	r, app, epoch, request := deploymentDrainFixture(t, false)
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_InputsProcessed, before.Status)
	require.ErrorIs(t, r.ForecloseDeploymentEpoch(t.Context(), request), repository.ErrNoUpdate)
	pending, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, before, pending, "generation must retain the valid IP proof producer")
	completeDeploymentDrainProofs(t, r, app, epoch)
	computed, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	outputBefore, err := r.GetOutput(t.Context(), app.Name, 0)
	require.NoError(t, err)
	require.NotNil(t, outputBefore.Hash)
	require.NotEmpty(t, outputBefore.OutputHashesSiblings)
	appBefore, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.True(t, appBefore.ForeclosureScanCaughtUp())
	require.NoError(t, r.ForecloseDeploymentEpoch(t.Context(), request))
	after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	expected := *computed
	expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
	require.Equal(t, expected, *after, "the transition changes only status and its inherited timestamp")
	outputAfter, err := r.GetOutput(t.Context(), app.Name, 0)
	require.NoError(t, err)
	require.Equal(t, outputBefore, outputAfter)
	appAfter, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, appBefore, appAfter)
	control, err := r.GetDeploymentAdmissionState(t.Context(), request.Identity)
	require.NoError(t, err)
	require.EqualValues(t, 0, control.ResetGeneration)
	require.Equal(t, "fresh confirmed generation", *control.DeploymentReason)
}

func TestDeploymentDrainRequiresActualCurrentGenerationTierAndIdentity(t *testing.T) {
	for _, name := range []string{
		deploymentDrainMissingCase, "neutral", deploymentDrainTableCase,
		"stale generation", "application identity", "consensus identity",
	} {
		t.Run(name, func(t *testing.T) {
			r, app, epoch, request := deploymentDrainFixture(t, true)
			switch name {
			case deploymentDrainMissingCase:
				_, err := r.db.Exec(t.Context(), `DELETE FROM deployment_admission_state WHERE application_id=$1`, app.ID)
				require.NoError(t, err)
			case "neutral":
				_, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{
					Identity: request.Identity})
				require.NoError(t, err)
			case deploymentDrainTableCase:
				deploymentRestrict(t, r, request.Identity, 0, model.DeploymentTierTable, "new confirmed table")
			case "stale generation":
				request.ResetGeneration = 1
			case "application identity":
				request.Identity.ApplicationAddress = repotest.UniqueAddress()
			case "consensus identity":
				request.Identity.ConsensusAddress = repotest.UniqueAddress()
			}
			require.ErrorIs(t, r.ForecloseDeploymentEpoch(t.Context(), request), repository.ErrNoUpdate)
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, epoch, after)
			if name == deploymentDrainMissingCase {
				control, err := r.GetDeploymentAdmissionState(t.Context(), deploymentKey(app))
				require.NoError(t, err)
				require.Nil(t, control, "the epoch owner must never Ensure a private row")
			}
		})
	}
}

func TestDeploymentDrainRetainsOrdinaryTypedSuffixAndAcceptedExclusion(t *testing.T) {
	t.Run("typed suffix", func(t *testing.T) {
		r, app, epoch, request := deploymentDrainFixture(t, false)
		require.NoError(t, r.StoreInvalidOutputsRoot(t.Context(), app.ID, epoch.Index, "cut makes this prefix invalid"))
		require.NoError(t, r.ForecloseDeploymentEpoch(t.Context(), request))
		current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.Equal(t, model.EpochStatus_ClaimForeclosed, current.Status)
	})
	t.Run("accepted is terminal", func(t *testing.T) {
		r, app, epoch, request := deploymentDrainFixture(t, true)
		require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, epoch.Index, new(repotest.UniqueHash())))
		before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.ErrorIs(t, r.ForecloseDeploymentEpoch(t.Context(), request), repository.ErrNoUpdate)
		after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})
	t.Run("known transaction remains reconcilable", func(t *testing.T) {
		r, app, epoch, request := deploymentDrainFixture(t, true)
		require.NoError(t, r.UpdateEpochWithSubmittedClaim(t.Context(), app.ID, epoch.Index, repotest.UniqueHash()))
		require.NoError(t, r.ForecloseDeploymentEpoch(t.Context(), request))
		current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
		require.NoError(t, err)
		require.Equal(t, model.EpochStatus_ClaimForeclosed, current.Status)
		require.NotNil(t, current.ClaimTransactionHash)
	})
}

func waitDeploymentEpochBlocker(t *testing.T, r *PostgresRepository, pid uint32) {
	t.Helper()
	require.Eventually(t, func() bool {
		var waiting bool
		err := r.db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&waiting)
		return err == nil && waiting
	}, time.Second, 10*time.Millisecond)
}

func TestDeploymentDrainOwnsNoControlDuringEpochWaitAndRejectsReset(t *testing.T) {
	r, app, epoch, request := deploymentDrainFixture(t, true)
	lock, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(lock)
	var marker int
	require.NoError(t, lock.QueryRow(t.Context(), `SELECT 1 FROM epoch WHERE application_id=$1 AND index=$2 FOR UPDATE`,
		app.ID, epoch.Index).Scan(&marker))
	done := make(chan error, 1)
	go func() { done <- r.ForecloseDeploymentEpoch(t.Context(), request) }()
	waitDeploymentEpochBlocker(t, r, lock.Conn().PgConn().PID())
	resetCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	err = r.EnableApplicationAndResetDeployment(resetCtx, app.ID, false)
	cancel()
	require.NoError(t, err, "waiting for epoch must not own control and block reset")
	require.NoError(t, lock.Rollback(t.Context()))
	require.ErrorIs(t, <-done, repository.ErrNoUpdate)
	current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, epoch, current)
}

func TestDeploymentDrainSharedControlFencesSameGenerationTierChange(t *testing.T) {
	r, app, epoch, request := deploymentDrainFixture(t, true)
	owned, barrier := deploymentBarrierRepository(t, r, func(query string) bool { return strings.Contains(query, "UPDATE public.epoch") })
	done := make(chan error, 1)
	go func() { done <- owned.ForecloseDeploymentEpoch(t.Context(), request) }()
	awaitDeploymentBarrier(t, barrier)
	type result struct {
		receipt *model.DeploymentAdmissionState
		err     error
	}
	restrict := make(chan result, 1)
	go func() {
		receipt, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: request.Identity,
			ConfirmedTier: new(model.DeploymentTierTable), Reason: "same generation new tier"})
		restrict <- result{receipt, err}
	}()
	waitDeploymentRowLock(t, r)
	// The statement snapshot permits a later health stop; control serialization is a separate boundary.
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Failed, new("after drain statement")))
	close(barrier.release)
	require.NoError(t, <-done)
	changed := <-restrict
	require.NoError(t, changed.err)
	require.EqualValues(t, 0, changed.receipt.ResetGeneration)
	require.Equal(t, model.DeploymentTierGeneration, *changed.receipt.ConfirmedTier)
	current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimForeclosed, current.Status)
	stopped, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Failed, stopped.Status)
}

func TestDeploymentDrainControlNOWAITAvoidsApplicationDeleteCycle(t *testing.T) {
	r, app, _, request := deploymentDrainFixture(t, true)
	owned, barrier := deploymentBarrierRepository(t, r, func(query string) bool {
		return strings.Contains(query, "FOR NO KEY UPDATE OF epoch")
	})
	var ownedPID uint32
	require.NoError(t, owned.db.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&ownedPID))
	done := make(chan error, 1)
	go func() { done <- owned.ForecloseDeploymentEpoch(t.Context(), request) }()
	awaitDeploymentBarrier(t, barrier)
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
	deleted := make(chan error, 1)
	go func() { deleted <- r.DeleteApplication(t.Context(), app.ID) }()
	waitDeploymentEpochBlocker(t, r, ownedPID)
	close(barrier.release)
	err := <-done
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, "55P03", pgError.Code)
	require.NoError(t, <-deleted, "drain must release the epoch so the cascade can finish")
	current, err := r.GetDeploymentAdmissionState(t.Context(), request.Identity)
	require.NoError(t, err)
	require.Nil(t, current)
}

func TestDeploymentDrainSharedControlKeepsResetAfterCFCommit(t *testing.T) {
	r, app, epoch, request := deploymentDrainFixture(t, true)
	owned, barrier := deploymentBarrierRepository(t, r, func(query string) bool { return strings.Contains(query, "UPDATE public.epoch") })
	done := make(chan error, 1)
	go func() { done <- owned.ForecloseDeploymentEpoch(t.Context(), request) }()
	awaitDeploymentBarrier(t, barrier)
	reset := make(chan error, 1)
	go func() { reset <- r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false) }()
	waitDeploymentRowLock(t, r)
	close(barrier.release)
	require.NoError(t, <-done)
	require.NoError(t, <-reset)
	current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimForeclosed, current.Status)
	control, err := r.GetDeploymentAdmissionState(t.Context(), request.Identity)
	require.NoError(t, err)
	require.EqualValues(t, 1, control.ResetGeneration)
	require.Nil(t, control.ConfirmedTier, "a later reset does not retract the already committed CF")
}

func TestDeploymentDrainEpochWaitIsBoundedAndRetainsPGCause(t *testing.T) {
	r, app, epoch, request := deploymentDrainFixture(t, true)
	lock, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(lock)
	_, err = lock.Exec(t.Context(), `SELECT 1 FROM epoch WHERE application_id=$1 AND index=$2 FOR UPDATE`, app.ID, epoch.Index)
	require.NoError(t, err)
	started := time.Now()
	err = r.ForecloseDeploymentEpoch(t.Context(), request)
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, "55P03", pgError.Code)
	require.Less(t, time.Since(started), 5*time.Second)
	require.NoError(t, lock.Rollback(t.Context()))
	require.NoError(t, r.ForecloseDeploymentEpoch(t.Context(), request), "a bounded failed acquisition must release its transaction")
}

func TestDeploymentDrainDeferredCommitFailurePreservesEpochAndReleasesControl(t *testing.T) {
	r, app, epoch, request := deploymentDrainFixture(t, true)
	_, err := r.db.Exec(t.Context(), `CREATE FUNCTION reject_deployment_drain_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.status='CLAIM_FORECLOSED' THEN RAISE EXCEPTION 'test drain commit rejection'; END IF;
			RETURN NEW;
		END $$;
		CREATE CONSTRAINT TRIGGER reject_deployment_drain_commit AFTER UPDATE ON epoch
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_deployment_drain_commit()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := r.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_deployment_drain_commit ON epoch;
			DROP FUNCTION IF EXISTS reject_deployment_drain_commit()`)
		require.NoError(t, err)
	})
	err = r.ForecloseDeploymentEpoch(t.Context(), request)
	require.ErrorContains(t, err, "test drain commit rejection")
	require.NotErrorIs(t, err, repository.ErrNoUpdate)
	current, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, epoch, current)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, r.EnableApplicationAndResetDeployment(ctx, app.ID, false))
}
