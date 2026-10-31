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
	"github.com/stretchr/testify/require"
)

func TestEffectiveDeploymentRequiresFreshCurrentEvidence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state        deploymentState
		tier         *model.DeploymentTier
		acknowledged bool
		generation   int64
		want         deploymentState
	}{
		{"unacknowledged", deploymentCompatible, nil, false, 1, deploymentNone},
		{"reset generation", deploymentCompatible, nil, true, 2, deploymentNone},
		{"restart saved generation", deploymentNone, new(model.DeploymentTierGeneration), true, 1, deploymentNone},
		{"pending saved generation", deploymentPending, new(model.DeploymentTierGeneration), true, 1, deploymentPending},
		{"restart saved table", deploymentNone, new(model.DeploymentTierTable), true, 1, deploymentNone},
		{"current compatible", deploymentCompatible, nil, true, 1, deploymentCompatible},
		{"durable generation before publication", deploymentCompatible, new(model.DeploymentTierGeneration), true, 1, deploymentGeneration},
		{"durable table before publication", deploymentCompatible, new(model.DeploymentTierTable), true, 1, deploymentTable},
		{"fresh confirmed generation", deploymentGeneration, new(model.DeploymentTierGeneration), true, 1, deploymentGeneration},
		{"fresh confirmed table", deploymentTable, new(model.DeploymentTierTable), true, 1, deploymentTable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := deploymentSnapshot{Acknowledged: tc.acknowledged, ResetGeneration: 1,
				Assessment: deploymentAssessment{State: tc.state}}
			require.Equal(t, tc.want, effectiveDeployment(snapshot, tc.generation, tc.tier))
		})
	}
}

func TestDeploymentFinalGateRetriesOnePublicationAndRetainsItsFence(t *testing.T) {
	const (
		repeatedState       = "ABA"
		retryRestriction    = "restriction on retry"
		repeatedPublication = "repeated publication"
	)
	for _, change := range []string{
		string(deploymentCompatible), repeatedState, string(deploymentPending),
		"restriction", "reset", repeatedPublication, retryRestriction,
	} {
		t.Run(change, func(t *testing.T) {
			f, call, _ := preparePRTAction(t, stageTournament)
			reads := 0
			f.repo.deploymentCompletion = func(ctx context.Context, key repository.DeploymentIdentity,
				index uint64,
			) (*repository.DeploymentCompletionState, error) {
				completion, err := f.repo.GetEpochCompletionState(ctx, key.ApplicationAddress.Hex(), index)
				require.NoError(t, err)
				row, err := f.repo.GetDeploymentAdmissionState(ctx, key)
				require.NoError(t, err)
				reads++
				if reads == 2 || reads == 3 && (change == repeatedPublication || change == retryRestriction) {
					switch change {
					case string(deploymentPending):
						setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentPending, nil)
					case "restriction":
						setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1,
							deploymentGeneration, new(model.DeploymentTierGeneration))
					case "reset":
						setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 2, deploymentNone, nil)
					case repeatedState:
						setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentPending, nil)
						setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentCompatible, nil)
					case retryRestriction:
						if reads == 3 {
							setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1,
								deploymentGeneration, new(model.DeploymentTierGeneration))
						} else {
							setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentCompatible, nil)
						}
					default:
						setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentCompatible, nil)
					}
				}
				return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, nil
			}
			eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
			if change == string(deploymentCompatible) || change == repeatedState {
				require.NoError(t, err)
				require.Equal(t, txsender.Useful, eligibility)
			} else {
				require.NotEqual(t, txsender.Useful, eligibility)
			}
			require.Equal(t, 3, reads, "only the raced final gate gets one fresh read")
		})
	}
}

func TestDeploymentCompletionDoesNotRetryRepositoryErrors(t *testing.T) {
	f := newDeliveryFixture(t)
	cause := errors.New("completion read failed")
	reads := 0
	f.repo.deploymentCompletion = func(context.Context, repository.DeploymentIdentity, uint64) (
		*repository.DeploymentCompletionState, error,
	) {
		reads++
		return nil, cause
	}
	_, _, err := f.d.deploymentCompletion(t.Context(), f.app, f.epoch.Index)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, reads)
}
