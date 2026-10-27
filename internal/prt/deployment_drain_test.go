// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeploymentDrainWaitsForFreshConfirmationAndNormalProofProducer(t *testing.T) {
	s := newDeploymentObserverService()
	repo := s.repository.(*prtRepositoryMock)
	app := prtRevertTestApp()
	app.ForecloseBlock, app.LastEpochCheckBlock, app.LastInputCheckBlock = 100, 100, 100
	repo.deploymentRows = map[int64]model.DeploymentAdmissionState{app.ID: {ApplicationID: app.ID,
		ResetGeneration: 7, ConfirmedTier: new(model.DeploymentTierGeneration), DeploymentReason: new("saved")}}
	client := &ethClientMock{}
	expectNumericWindowHeaders(client)
	s.client = client
	reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
		return DeploymentEvidence{Head: head, Consensus: app.IConsensusAddress, Candidate: deploymentNone}, nil
	}}
	var writes []uint64
	repo.deploymentDrain = func(_ context.Context, request repository.DeploymentDrainRequest) error {
		require.Equal(t, int64(8), request.ResetGeneration)
		writes = append(writes, request.EpochIndex)
		return nil
	}
	_, err := s.observeDeployment(t.Context(), app, reader, observerFacts(100))
	require.NoError(t, err)
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, reader, 102))
	require.Empty(t, writes, "a saved generation restriction cannot authorize drain from fresh none")
	reader.deploymentRead = func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
		return DeploymentEvidence{Head: head, Consensus: app.IConsensusAddress,
			Candidate: deploymentGeneration, Cause: deploymentCauseStaticShape}, nil
	}
	pending, err := s.observeDeployment(t.Context(), app, reader, observerFacts(101))
	require.NoError(t, err)
	require.Equal(t, deploymentPending, pending.Assessment.State)
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, reader, 102))
	require.Empty(t, writes, "the first fresh mismatch is still pending")
	confirmed, err := s.observeDeployment(t.Context(), app, reader, observerFacts(102))
	require.NoError(t, err)
	require.Equal(t, deploymentGeneration, confirmed.Assessment.State)
	prefix := checkpointEpoch(0, "0x100")
	prefix.Status, prefix.FirstBlock, prefix.LastBlock = model.EpochStatus_InputsProcessed, 0, 9
	straddler := checkpointEpoch(1, "0x101")
	straddler.Status, straddler.FirstBlock, straddler.LastBlock = model.EpochStatus_Closed, 90, 110
	future := checkpointEpoch(2, "0x102")
	future.Status, future.FirstBlock, future.LastBlock = model.EpochStatus_InputsProcessed, 101, 120
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(false, nil).Twice()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(true, nil).Twice()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{prefix, straddler, future}, uint64(3), nil).Twice()
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, reader, 102))
	require.Equal(t, []uint64{1}, writes, "protected IP must not stall a later eligible straddler")
	// The existing normal validator owns this atomic claim/proof publication.
	// This unit fixture supplies that provider result, rather than inventing a producer.
	prefix.Status = model.EpochStatus_ClaimComputed
	straddler.Status = model.EpochStatus_ClaimForeclosed
	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, nil, reader, 102))
	require.Equal(t, []uint64{1, 0}, writes)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	require.Equal(t, model.DeploymentTierGeneration, *repo.deploymentRows[app.ID].ConfirmedTier)
	repo.AssertExpectations(t)
}

func TestDeploymentDrainSkipsOnlyPureConditionalRefusal(t *testing.T) {
	dependency := errors.New("independent drain dependency")
	for _, tc := range []struct {
		name   string
		fault  error
		writes int
	}{
		{"pure", repository.ErrNoUpdate, 2},
		{"wrapped", fmt.Errorf("refusal: %w", repository.ErrNoUpdate), 1},
		{"joined", errors.Join(repository.ErrNoUpdate, dependency), 1},
		{"busy", errors.Join(repository.ErrLockTimeout, dependency), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.ForecloseBlock = 100
			setTestDeployment(s, repo, app, 1, deploymentGeneration, new(model.DeploymentTierGeneration))
			first, second := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x101")
			repo.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{first, second}, uint64(2), nil).Once()
			var requests []repository.DeploymentDrainRequest
			repo.deploymentDrain = func(_ context.Context, request repository.DeploymentDrainRequest) error {
				requests = append(requests, request)
				if len(requests) == 1 {
					return tc.fault
				}
				return nil
			}
			err := s.forecloseGenerationClaims(t.Context(), app, 1)
			if tc.name == "pure" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.fault)
			}
			require.Len(t, requests, tc.writes)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			require.False(t, s.publicationStopped())
		})
	}
}

func TestDeploymentDrainProtectsOnlyEpochsStrictlyBeforeForeclosure(t *testing.T) {
	for _, last := range []uint64{99, 100, 101} {
		t.Run(fmt.Sprint(last), func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.ForecloseBlock = 100
			setTestDeployment(s, repo, app, 1, deploymentGeneration, new(model.DeploymentTierGeneration))
			epoch := checkpointEpoch(0, "0x100")
			epoch.Status, epoch.FirstBlock, epoch.LastBlock = model.EpochStatus_InputsProcessed, 0, last
			repo.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
			writes := 0
			repo.deploymentDrain = func(context.Context, repository.DeploymentDrainRequest) error { writes++; return nil }
			require.NoError(t, s.forecloseGenerationClaims(t.Context(), app, 1))
			expected := 1
			if last < 100 {
				expected = 0
			}
			require.Equal(t, expected, writes)
		})
	}
}
