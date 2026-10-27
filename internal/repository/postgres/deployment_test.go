// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func deploymentKey(app *model.Application) repository.DeploymentIdentity {
	return repository.DeploymentIdentity{ApplicationID: app.ID,
		ApplicationAddress: app.IApplicationAddress, ConsensusAddress: app.IConsensusAddress}
}

func deploymentFixture(t *testing.T) (*PostgresRepository, *model.Application, repository.DeploymentIdentity) {
	t.Helper()
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().Create(t.Context(), t, r)
	return r, app, deploymentKey(app)
}

func deploymentRestrict(t *testing.T, r *PostgresRepository, key repository.DeploymentIdentity, generation int64,
	tier model.DeploymentTier, reason string,
) *model.DeploymentAdmissionState {
	t.Helper()
	got, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{
		Identity: key, ResetGeneration: generation, ConfirmedTier: new(tier), Reason: reason,
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	return got
}

func TestDeploymentStoreMissingIdentityAndNeutralState(t *testing.T) {
	r, app, key := deploymentFixture(t)
	neutral, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, &model.DeploymentAdmissionState{ApplicationID: key.ApplicationID}, neutral)
	for _, wrong := range []repository.DeploymentIdentity{
		{ApplicationID: key.ApplicationID + 1, ApplicationAddress: key.ApplicationAddress, ConsensusAddress: key.ConsensusAddress},
		{ApplicationID: key.ApplicationID, ApplicationAddress: repotest.UniqueAddress(), ConsensusAddress: key.ConsensusAddress},
		{ApplicationID: key.ApplicationID, ApplicationAddress: key.ApplicationAddress, ConsensusAddress: repotest.UniqueAddress()},
	} {
		got, err := r.GetDeploymentAdmissionState(t.Context(), wrong)
		require.NoError(t, err)
		require.Nil(t, got)
		got, err = r.BeginDeploymentStartup(t.Context(), wrong, 0)
		require.ErrorIs(t, err, repository.ErrNoUpdate)
		require.Nil(t, got)
		ack, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: wrong})
		require.ErrorIs(t, err, repository.ErrNoUpdate)
		require.Nil(t, ack)
	}
	_, err = r.db.Exec(t.Context(), `DELETE FROM deployment_admission_state WHERE application_id=$1`, app.ID)
	require.NoError(t, err)
	missing, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.Nil(t, missing)
	startup, err := r.BeginDeploymentStartup(t.Context(), key, 0)
	require.ErrorIs(t, err, repository.ErrNoUpdate)
	require.Nil(t, startup)
	previous, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key})
	require.ErrorIs(t, err, repository.ErrNoUpdate)
	require.Nil(t, previous)
	require.NoError(t, r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false))
	repaired := readDeployment(t, r, key)
	require.EqualValues(t, 1, repaired.ResetGeneration)
	require.Nil(t, repaired.ConfirmedTier)
}

func readDeployment(t *testing.T, r *PostgresRepository, key repository.DeploymentIdentity) *model.DeploymentAdmissionState {
	t.Helper()
	row, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.NotNil(t, row)
	return row
}

func TestDeploymentAssessmentStoresRestrictionAndRecovers(t *testing.T) {
	r, _, key := deploymentFixture(t)
	readDeployment(t, r, key)
	startup, err := r.BeginDeploymentStartup(t.Context(), key, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, startup.ResetGeneration)
	previous := deploymentRestrict(t, r, key, startup.ResetGeneration, model.DeploymentTierGeneration, "old generation")
	require.Nil(t, previous.ConfirmedTier)
	saved := readDeployment(t, r, key)
	require.Equal(t, model.DeploymentTierGeneration, *saved.ConfirmedTier)
	require.Equal(t, "old generation", *saved.DeploymentReason)
	for _, tier := range []model.DeploymentTier{model.DeploymentTierTable, model.DeploymentTierGeneration} {
		deploymentRestrict(t, r, key, startup.ResetGeneration, tier, "new confirmation")
		saved, err = r.GetDeploymentAdmissionState(t.Context(), key)
		require.NoError(t, err)
		require.Equal(t, tier, *saved.ConfirmedTier)
	}
	recovered, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
		ResetGeneration: startup.ResetGeneration})
	require.NoError(t, err)
	require.NotNil(t, recovered.ConfirmedTier)
	saved, err = r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.Nil(t, saved.ConfirmedTier)
	require.Nil(t, saved.DeploymentReason)
	require.Equal(t, startup.ResetGeneration, saved.ResetGeneration)
}

func TestDeploymentEnableResetAndStaleAssessment(t *testing.T) {
	r, app, key := deploymentFixture(t)
	require.NoError(t, r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false))
	state, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.NotNil(t, state, "explicit reset must retain a control row")
	require.EqualValues(t, 1, state.ResetGeneration, "the first explicit reset creates initial generation plus one")
	deploymentRestrict(t, r, key, state.ResetGeneration, model.DeploymentTierTable, "old table")
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
	disabled, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, model.DeploymentTierTable, *disabled.ConfirmedTier)
	require.Equal(t, state.ResetGeneration, disabled.ResetGeneration)
	// Explicit recovery authorization permits clearing the stored FAILED status.
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Failed, new("new failure")))
	require.NoError(t, r.EnableApplicationAndResetDeployment(t.Context(), app.ID, true))
	enabled, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.True(t, enabled.Enabled)
	require.Equal(t, model.ApplicationStatus_OK, enabled.Status)
	require.Nil(t, enabled.Reason)
	state, err = r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.EqualValues(t, 2, state.ResetGeneration)
	require.Nil(t, state.ConfirmedTier)
	for _, tier := range []*model.DeploymentTier{new(model.DeploymentTierGeneration), nil} {
		receipt, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
			ResetGeneration: disabled.ResetGeneration, ConfirmedTier: tier})
		require.ErrorIs(t, err, repository.ErrNoUpdate)
		require.Nil(t, receipt)
	}
	stale, err := r.BeginDeploymentStartup(t.Context(), key, disabled.ResetGeneration)
	require.ErrorIs(t, err, repository.ErrNoUpdate)
	require.Nil(t, stale)
	require.NoError(t, r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false))
	state, err = r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.EqualValues(t, 3, state.ResetGeneration, "healthy already-enabled reset still advances the fence")
}

func TestDeploymentEnableOverflowRollsBackApplicationAndControl(t *testing.T) {
	r, app, key := deploymentFixture(t)
	readDeployment(t, r, key)
	_, err := r.db.Exec(t.Context(), `UPDATE deployment_admission_state SET reset_generation=$2,
		confirmed_tier='generation', deployment_reason='retain on overflow' WHERE application_id=$1`, app.ID, int64(math.MaxInt64))
	require.NoError(t, err)
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Failed, new("retain failure")))
	require.NoError(t, r.UpdateApplicationEnabled(t.Context(), app.ID, false))
	before, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	control, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	err = r.EnableApplicationAndResetDeployment(t.Context(), app.ID, true)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "22003", pgErr.Code)
	after, err := r.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, before, after, "application enable and FAILED clear must roll back together")
	got, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, control, got)
	episode, err := r.BeginDeploymentStartup(t.Context(), key, math.MaxInt64)
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "22003", pgErr.Code)
	require.Nil(t, episode)
}

type deploymentBarrierKey struct{}

type deploymentBarrier struct {
	match   func(string) bool
	once    sync.Once
	reached chan struct{}
	release chan struct{}
}

func (b *deploymentBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, deploymentBarrierKey{}, b.match(data.SQL))
}

func (b *deploymentBarrier) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if match, _ := ctx.Value(deploymentBarrierKey{}).(bool); match {
		b.once.Do(func() {
			close(b.reached)
			select {
			case <-b.release:
			case <-ctx.Done():
			}
		})
	}
}

func deploymentBarrierRepository(t *testing.T, r *PostgresRepository, match func(string) bool) (*PostgresRepository, *deploymentBarrier) {
	t.Helper()
	barrier := &deploymentBarrier{match: match, reached: make(chan struct{}), release: make(chan struct{})}
	cfg := r.db.Config()
	cfg.MaxConns, cfg.ConnConfig.Tracer = 1, barrier
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	t.Cleanup(func() {
		select {
		case <-barrier.release:
		default:
			close(barrier.release)
		}
	})
	return &PostgresRepository{db: pool}, barrier
}

func awaitDeploymentBarrier(t *testing.T, barrier *deploymentBarrier) {
	t.Helper()
	select {
	case <-barrier.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("deployment query did not reach its actual SQL boundary")
	}
}

func waitDeploymentRowLock(t *testing.T, r *PostgresRepository) {
	t.Helper()
	require.Eventually(t, func() bool {
		var waiting bool
		err := r.db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%deployment_admission_state%')`).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 10*time.Millisecond)
}

func TestDeploymentStartupRetainsLateCommittedRestriction(t *testing.T) {
	r, _, key := deploymentFixture(t)
	readDeployment(t, r, key)
	old, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(old)
	_, err = old.Exec(t.Context(), `UPDATE deployment_admission_state SET confirmed_tier='table',
		deployment_reason='old backend committed late' WHERE application_id=$1 AND reset_generation=0`, key.ApplicationID)
	require.NoError(t, err)
	type result struct {
		state *model.DeploymentAdmissionState
		err   error
	}
	done := make(chan result, 1)
	go func() {
		state, err := r.BeginDeploymentStartup(t.Context(), key, 0)
		done <- result{state, err}
	}()
	waitDeploymentRowLock(t, r)
	require.NoError(t, old.Commit(t.Context()))
	got := <-done
	require.NoError(t, got.err)
	require.EqualValues(t, 1, got.state.ResetGeneration)
	require.Equal(t, model.DeploymentTierTable, *got.state.ConfirmedTier)
	require.Equal(t, "old backend committed late", *got.state.DeploymentReason)
}

func TestDeploymentQueuedOldAssessmentLosesToStartup(t *testing.T) {
	r, _, key := deploymentFixture(t)
	readDeployment(t, r, key)
	fenced, barrier := deploymentBarrierRepository(t, r, func(query string) bool {
		return strings.HasPrefix(strings.TrimSpace(query), "UPDATE public.deployment_admission_state")
	})
	type result struct {
		state *model.DeploymentAdmissionState
		err   error
	}
	fenceDone := make(chan result, 1)
	go func() {
		state, err := fenced.BeginDeploymentStartup(t.Context(), key, 0)
		fenceDone <- result{state, err}
	}()
	awaitDeploymentBarrier(t, barrier)
	ackDone := make(chan error, 1)
	go func() {
		receipt, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
			ConfirmedTier: new(model.DeploymentTierGeneration), Reason: "stale"})
		if receipt != nil {
			ackDone <- fmt.Errorf("stale assessment returned an owned receipt")
			return
		}
		ackDone <- err
	}()
	waitDeploymentRowLock(t, r)
	close(barrier.release)
	got := <-fenceDone
	require.NoError(t, got.err)
	require.EqualValues(t, 1, got.state.ResetGeneration)
	require.ErrorIs(t, <-ackDone, repository.ErrNoUpdate)
}

func TestDeploymentAssessmentOwnsOnlyControlAndResetFollowsCommit(t *testing.T) {
	r, app, key := deploymentFixture(t)
	readDeployment(t, r, key)
	deploymentRestrict(t, r, key, 0, model.DeploymentTierTable, "before reset")
	owned, barrier := deploymentBarrierRepository(t, r, func(query string) bool {
		return strings.Contains(query, "FOR NO KEY UPDATE OF deployment_admission_state")
	})
	type result struct {
		receipt *model.DeploymentAdmissionState
		err     error
	}
	done := make(chan result, 1)
	go func() {
		receipt, err := owned.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{
			Identity: key})
		done <- result{receipt, err}
	}()
	awaitDeploymentBarrier(t, barrier)
	probe, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(probe)
	var marker int
	require.NoError(t, probe.QueryRow(t.Context(), `SELECT 1 FROM application WHERE id=$1 FOR UPDATE NOWAIT`, app.ID).Scan(&marker),
		"assessment must not own application")
	require.NoError(t, probe.Rollback(t.Context()))
	reset := make(chan error, 1)
	go func() { reset <- r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false) }()
	waitDeploymentRowLock(t, r)
	close(barrier.release)
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, "before reset", *got.receipt.DeploymentReason)
	require.NoError(t, <-reset)
	state, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.EqualValues(t, 1, state.ResetGeneration)
	require.Nil(t, state.ConfirmedTier)
}

func TestDeploymentCompletionSnapshotSurvivesLaterResetAndStop(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, r)
	key := deploymentKey(app)
	readDeployment(t, r, key)
	deploymentRestrict(t, r, key, 0, model.DeploymentTierTable, "at snapshot")
	coherent, barrier := deploymentBarrierRepository(t, r, func(query string) bool {
		return strings.Contains(query, "execution_parameters.application_id") && strings.Contains(query, "application.name")
	})
	type result struct {
		state *repository.DeploymentCompletionState
		err   error
	}
	done := make(chan result, 1)
	go func() {
		state, err := coherent.GetDeploymentCompletionState(t.Context(), key, epoch.Index)
		done <- result{state, err}
	}()
	awaitDeploymentBarrier(t, barrier)
	require.NoError(t, r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false))
	require.NoError(t, r.UpdateApplicationStatus(t.Context(), app.ID, model.ApplicationStatus_Failed, new("after S")))
	close(barrier.release)
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, model.ApplicationStatus_OK, got.state.Completion.Application.Status)
	require.Equal(t, epoch.Index, got.state.Completion.Epoch.Index)
	require.EqualValues(t, 0, got.state.Deployment.ResetGeneration)
	require.Equal(t, "at snapshot", *got.state.Deployment.DeploymentReason)
	require.Zero(t, coherent.db.Stat().AcquiredConns(), "the RR connection must close before return")
	fresh, err := coherent.GetDeploymentCompletionState(t.Context(), key, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Failed, fresh.Completion.Application.Status)
	require.EqualValues(t, 1, fresh.Deployment.ResetGeneration)
	require.Nil(t, fresh.Deployment.ConfirmedTier)
	wrong := key
	wrong.ConsensusAddress = repotest.UniqueAddress()
	bad, err := coherent.GetDeploymentCompletionState(t.Context(), wrong, epoch.Index)
	require.ErrorIs(t, err, repository.ErrNoUpdate)
	require.Nil(t, bad)
	require.Zero(t, coherent.db.Stat().AcquiredConns())
}

func TestDeploymentEnablePreservesHealthBoundariesForeclosureAndClaimEvidence(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK, model.ApplicationStatus_Failed,
		model.ApplicationStatus_Corrupted, model.ApplicationStatus_Diverged,
		model.ApplicationStatus_GuestException, model.ApplicationStatus_MachineHalted,
		model.ApplicationStatus_McycleOverflow, model.ApplicationStatus_UnexpectedYield, model.ApplicationStatus_InvalidOutputsRoot} {
		for _, clearFailed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/recovery=%t", status, clearFailed), func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				app, epoch, proof := machinePublicationEpoch(t, r, false)
				key := deploymentKey(app)
				require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
				require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
				readDeployment(t, r, key)
				deploymentRestrict(t, r, key, 0, model.DeploymentTierGeneration, "incompatible generation")
				require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 100, repotest.UniqueHash(), 100))
				kind := model.ApplicationStatus_MachineHalted
				var input any = uint64(2)
				if status == model.ApplicationStatus_InvalidOutputsRoot {
					kind, input = status, nil
				}
				_, err := r.db.Exec(t.Context(), `UPDATE application SET enabled=false,status=$2,reason='retained health',
				problem_epoch_index=3,problem_kind=$3,problem_input_index=$4,
				last_input_check_block=120,last_output_check_block=130,last_claim_check_block=50,processed_inputs=2 WHERE id=$1`,
					app.ID, status, kind, input)
				require.NoError(t, err)
				before, problem, err := getApplication(t.Context(), r.db, app.Name)
				require.NoError(t, err)
				require.True(t, claimValidated(t, r, app.ID, epoch.Index))
				require.NoError(t, r.EnableApplicationAndResetDeployment(t.Context(), app.ID, clearFailed))
				after, afterProblem, err := getApplication(t.Context(), r.db, app.Name)
				require.NoError(t, err)
				expected := *before
				expected.Enabled = true
				if status == model.ApplicationStatus_Failed && clearFailed {
					expected.Status, expected.Reason = model.ApplicationStatus_OK, nil
				}
				after.UpdatedAt = expected.UpdatedAt
				require.Equal(t, expected, *after, "only enabled and authorized current FAILED status/reason may change")
				require.Equal(t, problem, afterProblem)
				require.True(t, claimValidated(t, r, app.ID, epoch.Index))
				claimBlock, err := r.GetEventLastCheckBlock(t.Context(), app.ID, model.MonitoredEvent_ClaimAccepted)
				require.NoError(t, err)
				require.EqualValues(t, 50, claimBlock)
				state, err := r.GetDeploymentAdmissionState(t.Context(), key)
				require.NoError(t, err)
				require.EqualValues(t, 1, state.ResetGeneration)
				require.Nil(t, state.ConfirmedTier)
				require.Nil(t, state.DeploymentReason)
			})
		}
	}
}

func TestDeploymentActualCommitFailureReturnsNoReceiptOrEpisode(t *testing.T) {
	r, _, key := deploymentFixture(t)
	readDeployment(t, r, key)
	_, err := r.db.Exec(t.Context(), `CREATE FUNCTION reject_deployment_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.deployment_reason='reject at commit' OR NEW.reset_generation=1 THEN
				RAISE EXCEPTION 'test deferred commit rejection';
			END IF;
			RETURN NEW;
		END $$;
		CREATE CONSTRAINT TRIGGER reject_deployment_commit AFTER UPDATE ON deployment_admission_state
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_deployment_commit()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := r.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_deployment_commit ON deployment_admission_state;
			DROP FUNCTION IF EXISTS reject_deployment_commit()`)
		require.NoError(t, err)
	})
	receipt, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{Identity: key,
		ConfirmedTier: new(model.DeploymentTierTable), Reason: "reject at commit"})
	require.ErrorContains(t, err, "test deferred commit rejection")
	require.Nil(t, receipt)
	episode, err := r.BeginDeploymentStartup(t.Context(), key, 0)
	require.ErrorContains(t, err, "test deferred commit rejection")
	require.Nil(t, episode)
	state, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, &model.DeploymentAdmissionState{ApplicationID: key.ApplicationID}, state)
}

func TestDeploymentControlWaitRespectsCallerCancellation(t *testing.T) {
	r, _, key := deploymentFixture(t)
	readDeployment(t, r, key)
	lock, err := r.db.Begin(t.Context())
	require.NoError(t, err)
	defer rollbackDeliveryTx(lock)
	_, err = lock.Exec(t.Context(), `SELECT 1 FROM deployment_admission_state WHERE application_id=$1 FOR UPDATE`, key.ApplicationID)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	episode, err := r.BeginDeploymentStartup(ctx, key, 0)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, episode)
	ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
	receipt, err := r.AcknowledgeDeploymentAssessment(ctx, repository.DeploymentAssessment{
		Identity: key})
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, receipt)
	require.NoError(t, lock.Rollback(t.Context()))
	episode, err = r.BeginDeploymentStartup(t.Context(), key, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, episode.ResetGeneration)
}

func TestDeploymentResetMayFollowAcknowledgedStartup(t *testing.T) {
	r, app, key := deploymentFixture(t)
	readDeployment(t, r, key)
	deploymentRestrict(t, r, key, 0, model.DeploymentTierGeneration, "startup retains restriction")
	fenced, barrier := deploymentBarrierRepository(t, r, func(query string) bool {
		return strings.HasPrefix(strings.TrimSpace(query), "UPDATE public.deployment_admission_state")
	})
	type result struct {
		state *model.DeploymentAdmissionState
		err   error
	}
	done := make(chan result, 1)
	go func() { state, err := fenced.BeginDeploymentStartup(t.Context(), key, 0); done <- result{state, err} }()
	awaitDeploymentBarrier(t, barrier)
	reset := make(chan error, 1)
	go func() { reset <- r.EnableApplicationAndResetDeployment(t.Context(), app.ID, false) }()
	waitDeploymentRowLock(t, r)
	close(barrier.release)
	got := <-done
	require.NoError(t, got.err)
	require.EqualValues(t, 1, got.state.ResetGeneration)
	require.Equal(t, "startup retains restriction", *got.state.DeploymentReason)
	require.NoError(t, <-reset)
	current, err := r.GetDeploymentAdmissionState(t.Context(), key)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.ResetGeneration)
	require.Nil(t, current.ConfirmedTier)
	receipt, err := r.AcknowledgeDeploymentAssessment(t.Context(), repository.DeploymentAssessment{
		Identity: key, ResetGeneration: got.state.ResetGeneration})
	require.ErrorIs(t, err, repository.ErrNoUpdate, "a committed historical startup receipt cannot authorize after reset")
	require.Nil(t, receipt)
}
