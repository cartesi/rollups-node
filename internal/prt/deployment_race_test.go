// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeploymentPublicationChangeDuringCompletionBlocksUsefulAction(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	f.repo.deploymentCompletion = func(ctx context.Context, identity repository.DeploymentIdentity, index uint64) (
		*repository.DeploymentCompletionState, error,
	) {
		completion, err := f.repo.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), index)
		require.NoError(t, err)
		row, err := f.repo.GetDeploymentAdmissionState(ctx, identity)
		require.NoError(t, err)
		setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentPending, nil)
		return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, nil
	}
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Equal(t, txsender.ProtocolWait, eligibility)
}
func TestDeploymentResetDuringProtocolReadsBlocksUsefulAction(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	for _, expected := range f.consensus.ExpectedCalls {
		if expected.Method == "GetCurrentSealedEpoch" {
			expected.Run(func(mock.Arguments) {
				row := f.repo.deploymentRows[f.app.ID]
				row.ResetGeneration = 2
				row.ConfirmedTier = nil
				row.DeploymentReason = nil
				f.repo.deploymentRows[f.app.ID] = row
			})
		}
	}
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Equal(t, txsender.ProtocolWait, eligibility)
}
