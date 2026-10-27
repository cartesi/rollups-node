// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeploymentActionPolicyUsesFreshAndDurableState(t *testing.T) {
	for _, state := range []deploymentState{
		deploymentNone, deploymentPending, deploymentGeneration, deploymentTable, deploymentCompatible,
	} {
		for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
			t.Run(string(state)+"/"+actionName(kind), func(t *testing.T) {
				f, call, _ := preparePRTAction(t, kind)
				var tier *model.DeploymentTier
				if state == deploymentGeneration {
					tier = new(model.DeploymentTierGeneration)
				}
				if state == deploymentTable {
					tier = new(model.DeploymentTierTable)
				}
				setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, state, tier)
				eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
				switch {
				case state == deploymentNone || state == deploymentPending:
					require.ErrorIs(t, err, txsender.ErrInconclusive)
					require.Equal(t, txsender.ProtocolWait, eligibility)
				case state == deploymentGeneration || state == deploymentTable && kind == joinTournament:
					require.NoError(t, err)
					require.Equal(t, txsender.Obsolete, eligibility)
				default:
					require.NoError(t, err)
					require.Equal(t, txsender.Useful, eligibility)
				}
			})
		}
	}
}

func TestDeploymentRecheckRejectsPublicationABAAndUsesCurrentEligibility(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "current disabled state", true: "equal publication ABA"}[changed], func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			f.repo.deploymentCompletion = func(ctx context.Context, identity repository.DeploymentIdentity, index uint64) (
				*repository.DeploymentCompletionState, error,
			) {
				state, err := f.repo.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), index)
				require.NoError(t, err)
				owned := *state.Application
				if !changed {
					owned.Enabled = false
				}
				completion := *state
				completion.Application = &owned
				row, err := f.repo.GetDeploymentAdmissionState(ctx, identity)
				require.NoError(t, err)
				if changed {
					key := deploymentIdentity(f.app)
					captured := f.d.deployments.capture(key)
					f.d.deployments.publish(key, captured.snapshot)
				}
				return &repository.DeploymentCompletionState{Completion: &completion, Deployment: row}, nil
			}
			eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
			if changed {
				require.ErrorIs(t, err, txsender.ErrInconclusive)
				require.Equal(t, txsender.ProtocolWait, eligibility)
			} else {
				require.NoError(t, err)
				require.Equal(t, txsender.Obsolete, eligibility)
				require.True(t, f.app.Enabled, "earlier planning metadata remains enabled")
			}
			require.Empty(t, f.backend.published)
		})
	}
}

func TestDeploymentPreparedBytesSurviveResetAfterSnapshotAndAmbiguousPOST(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, deploymentCompatible, nil)
	preflights := 0
	f.backend.onPreflight = func(context.Context, ethereum.CallMsg, txsender.BlockRef) error {
		preflights++
		if preflights == 2 {
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 3, deploymentCompatible, nil)
		}
		return nil
	}
	cause := errors.New("lost POST acknowledgment")
	f.backend.publishError = cause
	report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, call})
	require.ErrorIs(t, err, cause)
	require.Equal(t, txsender.Uncertain, report.Class)
	require.Equal(t, int64(3), f.repo.deploymentRows[f.app.ID].ResetGeneration)
	require.Equal(t, call.Data, f.backend.published[0].Data())
	require.Equal(t, call.To, *f.backend.published[0].To())
	require.Equal(t, call.Value, f.backend.published[0].Value())
}

func TestDeploymentConfirmedCancellationDoesNotNeedProjectionAndRetainsSignedBytes(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, deploymentGeneration, new(model.DeploymentTierGeneration))
	f.repo.publishedRoots = nil
	neutralCalls := 0
	f.backend.onCall = func(_ context.Context, msg ethereum.CallMsg, _ txsender.BlockRef) error {
		if len(msg.Data) == 0 {
			neutralCalls++
			if neutralCalls == 2 {
				setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 3, deploymentPending, new(model.DeploymentTierGeneration))
			}
		}
		return nil
	}
	cause := errors.New("ambiguous neutral POST")
	f.backend.publishError = cause
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.ErrorIs(t, err, cause)
	require.True(t, report.Current.Neutral)
	require.Equal(t, int64(3), f.repo.deploymentRows[f.app.ID].ResetGeneration)
	require.Empty(t, f.backend.published[1].Data())
	setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 4, deploymentGeneration, new(model.DeploymentTierGeneration))
	f.backend.publishError = nil
	again, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, again.Current.Neutral)
	require.Equal(t, f.backend.raw[1], f.backend.raw[2])
}

func TestDeploymentUnknownRetainsSignedRequestWithoutCancellation(t *testing.T) {
	for _, state := range []deploymentState{deploymentNone, deploymentPending} {
		t.Run(string(state), func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			before := submitPRTAction(t, f, call)
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, state, new(model.DeploymentTierGeneration))
			report, err := f.d.sender.PublishNext(t.Context(), f.d)
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.False(t, report.Attempted)
			require.False(t, report.Current.Neutral)
			require.Equal(t, before.Current, report.Current)
			require.Len(t, f.backend.published, 1)
		})
	}
}

func TestDeploymentTablePublishesTrueAlternateFinalResult(t *testing.T) {
	for _, kind := range []actionKind{stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, _, view := preparePRTAction(t, kind)
			view.stage.WinnerCommitment = common.HexToHash("0xdead")
			updatePRTView(f, 110, view)
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentTable, new(model.DeploymentTierTable))
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
			require.NoError(t, err)
			require.NotNil(t, call)
			report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, *call})
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.Equal(t, kind, report.Current.Action.Kind)
			require.Equal(t, call.Data, f.backend.published[0].Data())
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		})
	}
}

func TestDeploymentVerdictGatePreservesTrueResultPolicyAndConsensusBounds(t *testing.T) {
	for _, state := range []deploymentState{deploymentNone, deploymentPending, deploymentGeneration, deploymentTable} {
		t.Run(string(state), func(t *testing.T) {
			f, _, view := preparePRTAction(t, stageTournament)
			view.stage.WinnerPostEpochMachineStateHash = common.HexToHash("0xbad")
			var saved *model.DeploymentTier
			if state == deploymentGeneration {
				saved = new(model.DeploymentTierGeneration)
			}
			if state == deploymentTable {
				saved = new(model.DeploymentTierTable)
				f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, model.ApplicationStatus_Diverged, mock.Anything).
					Return(nil).Once()
				expectStoredProtection(t, f.repo.prtRepositoryMock, f.app)
			}
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, state, saved)
			err := f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view)
			if state == deploymentTable {
				require.Error(t, err)
				f.repo.AssertExpectations(t)
			} else {
				require.NoError(t, err)
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestDeploymentFinalCheckDoesNotAuthorizeCancellationFromOldApplicationMetadata(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	f.app.Enabled = false
	f.repo.deploymentCompletion = func(ctx context.Context, identity repository.DeploymentIdentity, index uint64) (
		*repository.DeploymentCompletionState, error,
	) {
		completion, err := f.repo.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), index)
		require.NoError(t, err)
		current := *completion.Application
		current.Enabled = true
		owned := *completion
		owned.Application = &current
		row, err := f.repo.GetDeploymentAdmissionState(ctx, identity)
		require.NoError(t, err)
		return &repository.DeploymentCompletionState{Completion: &owned, Deployment: row}, nil
	}
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.NoError(t, err)
	require.Equal(t, txsender.Useful, eligibility)
	require.False(t, f.app.Enabled)
}

func TestDeploymentConfirmedRetainedRefusalPrecedesIncompatibleProtocolReads(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	f.consensus.ExpectedCalls = nil
	f.consensus.Calls = nil
	f.consensus.On("GetCurrentSealedEpoch", mock.Anything).
		Return(CurrentSealedEpoch{}, errors.New("incompatible static consensus view")).Maybe()
	setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, deploymentGeneration, new(model.DeploymentTierGeneration))
	f.repo.publishedRoots = nil
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.True(t, report.Current.Neutral)
	f.consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.Anything)
}

func TestDeploymentRecoveryRemainsIndependentOfAdmission(t *testing.T) {
	for _, state := range []deploymentState{deploymentNone, deploymentPending, deploymentGeneration, deploymentTable} {
		t.Run(string(state), func(t *testing.T) {
			f := newRecoveryPortFixture(t)
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, state, new(model.DeploymentTierGeneration))
			call, err := f.plan(t.Context())
			require.NoError(t, err)
			require.NotNil(t, call)
			report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, *call})
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.Equal(t, recoverBond, report.Current.Action.Kind)
			require.Equal(t, call.Data, f.backend.published[0].Data())
		})
	}
}

func TestDeploymentDurableRestrictionPrecedesCachePublication(t *testing.T) {
	for _, tier := range []model.DeploymentTier{model.DeploymentTierGeneration, model.DeploymentTierTable} {
		for _, kind := range []actionKind{joinTournament, stageTournament} {
			t.Run(string(tier)+"/"+actionName(kind), func(t *testing.T) {
				f, call, _ := preparePRTAction(t, kind)
				row := f.repo.deploymentRows[f.app.ID]
				row.ConfirmedTier, row.DeploymentReason = new(tier), new("confirmed before cache publication")
				f.repo.deploymentRows[f.app.ID] = row
				require.Equal(t, deploymentCompatible, f.d.deployments.capture(deploymentIdentity(f.app)).snapshot.Assessment.State)
				eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
				require.NoError(t, err)
				if tier == model.DeploymentTierGeneration || kind == joinTournament {
					require.Equal(t, txsender.Obsolete, eligibility)
				} else {
					require.Equal(t, txsender.Useful, eligibility)
				}
			})
		}
	}
}

func TestDeploymentCompletionSnapshotSurvivesResetBeforeReadReturns(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, deploymentCompatible, nil)
	reads := 0
	f.repo.deploymentCompletion = func(ctx context.Context, identity repository.DeploymentIdentity, index uint64) (
		*repository.DeploymentCompletionState, error,
	) {
		completion, err := f.repo.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), index)
		require.NoError(t, err)
		row, err := f.repo.GetDeploymentAdmissionState(ctx, identity)
		require.NoError(t, err)
		reads++
		if reads == 2 {
			// These returned values belong to S. Reset commits after S while
			// this unchanged immutable cache publication remains current.
			reset := *row
			reset.ResetGeneration = 3
			f.repo.deploymentRows[f.app.ID] = reset
		}
		return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, nil
	}
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.NoError(t, err)
	require.Equal(t, txsender.Useful, eligibility)
	require.Equal(t, int64(3), f.repo.deploymentRows[f.app.ID].ResetGeneration)
	eligibility, err = f.d.Recheck(t.Context(), f.block, call.Key)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Equal(t, txsender.ProtocolWait, eligibility, "the next attempt must open a fresh snapshot")
}

func TestDeploymentFinalDatabaseReadHoldsNoPublicationLock(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, deploymentCompatible, nil)
	reads := 0
	f.repo.deploymentCompletion = func(ctx context.Context, identity repository.DeploymentIdentity, index uint64) (
		*repository.DeploymentCompletionState, error,
	) {
		if !f.d.deployments.mu.TryLock() {
			return nil, errors.New("publication mutex held during SQL")
		}
		f.d.deployments.mu.Unlock()
		reads++
		completion, err := f.repo.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), index)
		require.NoError(t, err)
		row, err := f.repo.GetDeploymentAdmissionState(ctx, identity)
		require.NoError(t, err)
		return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, nil
	}
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.NoError(t, err)
	require.Equal(t, txsender.Useful, eligibility)
	require.Equal(t, 2, reads)
}
