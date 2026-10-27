// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeploymentAmbiguousRestrictionDoesNotRecoverWithinSameEpisode(t *testing.T) {
	for _, test := range []struct {
		name       string
		compatible bool
		committed  bool
	}{
		{name: "after compatibility", compatible: true, committed: true},
		{name: "first restriction reply lost", committed: true},
		{name: "first restriction rolled back"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newDeploymentObserverService()
			repo := s.repository.(*prtRepositoryMock)
			app := prtRevertTestApp()
			reader := newCompatibleDaveConsensusAdapterMock()
			if !test.committed {
				repo.deploymentRows = map[int64]model.DeploymentAdmissionState{app.ID: {
					ApplicationID: app.ID, ConfirmedTier: new(model.DeploymentTierGeneration),
					DeploymentReason: new("saved restriction"),
				}}
			}
			if test.compatible {
				_, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(99))
				require.NoError(t, err)
			}
			header := &types.Header{Number: big.NewInt(100)}
			first := txsender.BlockRef{Number: 100, Hash: header.Hash()}
			client := &ethClientMock{}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(header, nil).Once()
			s.client = client
			reader.deploymentRead = func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
				return tableDeploymentEvidence(h), nil
			}
			_, err := s.observeDeployment(t.Context(), app, reader, first)
			require.NoError(t, err)

			writeError := errors.New("restriction persistence failed")
			repo.deploymentAck = func(_ context.Context, request repository.DeploymentAssessment) (*model.DeploymentAdmissionState, error) {
				require.NotNil(t, request.ConfirmedTier)
				if test.committed {
					row := repo.deploymentRows[app.ID]
					row.ConfirmedTier, row.DeploymentReason = new(*request.ConfirmedTier), new(request.Reason)
					repo.deploymentRows[app.ID] = row
				}
				return nil, writeError
			}
			_, err = s.observeDeployment(t.Context(), app, reader, deploymentTestHead(101))
			require.ErrorIs(t, err, writeError)
			cached := s.deployments.capture(deploymentIdentity(app)).snapshot
			require.Equal(t, deploymentPending, cached.Assessment.State)

			repo.deploymentAck = nil
			reader = newCompatibleDaveConsensusAdapterMock()
			_, err = s.observeDeployment(t.Context(), app, reader, deploymentTestHead(102))
			require.NoError(t, err)
			require.NotNil(t, repo.deploymentRows[app.ID].ConfirmedTier,
				"a same-episode compatible tick must retain the restriction")
			row := repo.deploymentRows[app.ID]
			row.ResetGeneration++
			repo.deploymentRows[app.ID] = row
			_, err = s.observeDeployment(t.Context(), app, reader, deploymentTestHead(103))
			require.NoError(t, err)
			require.Nil(t, repo.deploymentRows[app.ID].ConfirmedTier, "an explicit reset permits fresh compatible recovery")
			client.AssertExpectations(t)
		})
	}
}
