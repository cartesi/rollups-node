// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
)

func TestDeploymentStartupRetainsSavedRestrictionUntilFreshRecovery(t *testing.T) {
	for _, tier := range []model.DeploymentTier{model.DeploymentTierGeneration, model.DeploymentTierTable} {
		t.Run(string(tier), func(t *testing.T) {
			s := newDeploymentObserverService()
			repo := s.repository.(*prtRepositoryMock)
			app := prtRevertTestApp()
			repo.deploymentRows = map[int64]model.DeploymentAdmissionState{app.ID: {ApplicationID: app.ID,
				ResetGeneration: 7, ConfirmedTier: new(tier), DeploymentReason: new("saved evidence")}}
			var logs bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			cause := errors.New("temporary deployment read")
			reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, head txsender.BlockRef) (
				DeploymentEvidence, error,
			) {
				return DeploymentEvidence{Head: head, Consensus: app.IConsensusAddress, Candidate: deploymentNone}, cause
			}}
			first, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(100))
			require.ErrorIs(t, err, cause)
			require.Equal(t, int64(8), first.ResetGeneration)
			require.Equal(t, deploymentNone, first.Assessment.State)
			require.NotNil(t, repo.deploymentRows[app.ID].ConfirmedTier)
			require.Equal(t, tier, *repo.deploymentRows[app.ID].ConfirmedTier)
			second, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(101))
			require.ErrorIs(t, err, cause)
			require.Equal(t, int64(8), second.ResetGeneration, "transient retries do not start another episode")
			reader.deploymentRead = func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
				return compatibleDeploymentEvidence(head), nil
			}
			current, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(102))
			require.NoError(t, err)
			require.Equal(t, deploymentCompatible, current.Assessment.State)
			require.Nil(t, repo.deploymentRows[app.ID].ConfirmedTier)
			require.Nil(t, repo.deploymentRows[app.ID].DeploymentReason)
			require.Contains(t, logs.String(), "Recovered saved PRT deployment restriction")
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			require.False(t, s.publicationStopped())
		})
	}
}

func TestDeploymentStalePendingPublicationCannotCarryAcrossReset(t *testing.T) {
	s := newDeploymentObserverService()
	repo := s.repository.(*prtRepositoryMock)
	app := prtRevertTestApp()
	reset := true
	reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, head txsender.BlockRef) (
		DeploymentEvidence, error,
	) {
		if reset {
			row := repo.deploymentRows[app.ID]
			row.ResetGeneration++
			repo.deploymentRows[app.ID] = row
			reset = false
		}
		return tableDeploymentEvidence(head), nil
	}}
	_, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(100))
	require.NoError(t, err)
	captured := s.deployments.capture(deploymentIdentity(app))
	require.Equal(t, int64(1), captured.snapshot.ResetGeneration)
	require.Equal(t, deploymentPending, captured.snapshot.Assessment.State)
	next, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(101))
	require.NoError(t, err)
	require.Equal(t, int64(2), next.ResetGeneration)
	require.Equal(t, deploymentPending, next.Assessment.State)
	require.Equal(t, deploymentTestHead(101), next.Assessment.PendingEvidence.Head)
	require.Nil(t, repo.deploymentRows[app.ID].ConfirmedTier)
}

func TestDeploymentFailedAcknowledgmentDoesNotPublishCandidate(t *testing.T) {
	s := newDeploymentObserverService()
	repo := s.repository.(*prtRepositoryMock)
	app := prtRevertTestApp()
	_, err := s.deploymentEpisode(t.Context(), app)
	require.NoError(t, err)
	before := s.deployments.capture(deploymentIdentity(app))
	cause := errors.New("ambiguous assessment commit")
	repo.deploymentAck = func(context.Context, repository.DeploymentAssessment) (*model.DeploymentAdmissionState, error) {
		return nil, cause
	}
	reader := newCompatibleDaveConsensusAdapterMock()
	_, err = s.observeDeployment(t.Context(), app, reader, deploymentTestHead(100))
	require.ErrorIs(t, err, cause)
	require.True(t, s.deployments.same(deploymentIdentity(app), before))
	require.Equal(t, deploymentNone, before.snapshot.Assessment.State)
}

func TestDeploymentUnknownEscalationCountsDistinctHeads(t *testing.T) {
	s := newDeploymentObserverService()
	app := prtRevertTestApp()
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, head txsender.BlockRef) (
		DeploymentEvidence, error,
	) {
		return DeploymentEvidence{Head: head, Consensus: app.IConsensusAddress, Candidate: deploymentNone}, nil
	}}
	for repeat := range 6 {
		_, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(100+uint64(repeat%2)))
		require.NoError(t, err)
	}
	require.NotContains(t, logs.String(), "level=ERROR")
	for head := uint64(102); head < 105; head++ {
		_, err := s.observeDeployment(t.Context(), app, reader, deploymentTestHead(head))
		require.NoError(t, err)
	}
	require.Contains(t, logs.String(), "level=ERROR")
	require.Contains(t, logs.String(), "distinct_heads=5")
}
