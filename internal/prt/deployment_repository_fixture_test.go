// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func cloneDeploymentRow(row model.DeploymentAdmissionState) model.DeploymentAdmissionState {
	if row.ConfirmedTier != nil {
		row.ConfirmedTier = new(*row.ConfirmedTier)
	}
	if row.DeploymentReason != nil {
		row.DeploymentReason = new(*row.DeploymentReason)
	}
	return row
}

func (m *prtRepositoryMock) ForecloseDeploymentEpoch(ctx context.Context, request repository.DeploymentDrainRequest) error {
	if m.deploymentDrain != nil {
		return m.deploymentDrain(ctx, request)
	}
	return m.Called(ctx, request).Error(0)
}

func (m *prtRepositoryMock) GetDeploymentAdmissionState(ctx context.Context,
	identity repository.DeploymentIdentity,
) (*model.DeploymentAdmissionState, error) {
	if m.deploymentRead != nil {
		return m.deploymentRead(ctx, identity)
	}
	m.deploymentMu.Lock()
	defer m.deploymentMu.Unlock()
	row, ok := m.deploymentRows[identity.ApplicationID]
	if !ok {
		return nil, nil
	}
	owned := cloneDeploymentRow(row)
	return &owned, nil
}

func (m *prtRepositoryMock) BeginDeploymentStartup(_ context.Context, identity repository.DeploymentIdentity,
	expected int64,
) (*model.DeploymentAdmissionState, error) {
	m.deploymentMu.Lock()
	defer m.deploymentMu.Unlock()
	row, ok := m.deploymentRows[identity.ApplicationID]
	if !ok || row.ResetGeneration != expected {
		return nil, repository.ErrNoUpdate
	}
	row.ResetGeneration++
	m.deploymentRows[identity.ApplicationID] = row
	owned := cloneDeploymentRow(row)
	return &owned, nil
}

func (m *prtRepositoryMock) AcknowledgeDeploymentAssessment(ctx context.Context,
	request repository.DeploymentAssessment,
) (*model.DeploymentAdmissionState, error) {
	if m.deploymentAck != nil {
		return m.deploymentAck(ctx, request)
	}
	m.deploymentMu.Lock()
	defer m.deploymentMu.Unlock()
	row, ok := m.deploymentRows[request.Identity.ApplicationID]
	if !ok || row.ResetGeneration != request.ResetGeneration {
		return nil, repository.ErrNoUpdate
	}
	previous := cloneDeploymentRow(row)
	if request.ConfirmedTier != nil {
		row.ConfirmedTier, row.DeploymentReason = new(*request.ConfirmedTier), new(request.Reason)
	} else {
		row.ConfirmedTier, row.DeploymentReason = nil, nil
	}
	m.deploymentRows[request.Identity.ApplicationID] = row
	return &previous, nil
}

func (m *prtRepositoryMock) GetDeploymentCompletionState(ctx context.Context,
	identity repository.DeploymentIdentity, epoch uint64,
) (*repository.DeploymentCompletionState, error) {
	if m.deploymentCompletion != nil {
		return m.deploymentCompletion(ctx, identity, epoch)
	}
	completion, err := m.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), epoch)
	if err != nil {
		return nil, err
	}
	row, err := m.GetDeploymentAdmissionState(ctx, identity)
	return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, err
}

func admitTestDeployment(s *Service, repo *prtRepositoryMock, app *model.Application) {
	row := model.DeploymentAdmissionState{ApplicationID: app.ID, ResetGeneration: 1}
	if repo.deploymentRows == nil {
		repo.deploymentRows = map[int64]model.DeploymentAdmissionState{}
	}
	repo.deploymentRows[app.ID] = row
	key := deploymentIdentity(app)
	s.deployments.publish(key, deploymentSnapshot{Acknowledged: true, ResetGeneration: 1,
		Assessment: deploymentAssessment{State: deploymentCompatible}})
}

func newDeploymentObserverService() *Service {
	s, repo := newPRTServiceMock()
	s.deployments = deploymentCache{}
	app := prtRevertTestApp()
	repo.deploymentRows = map[int64]model.DeploymentAdmissionState{app.ID: {ApplicationID: app.ID}}
	return s
}

func setTestDeployment(s *Service, repo *prtRepositoryMock, app *model.Application,
	generation int64, state deploymentState, saved *model.DeploymentTier,
) {
	repo.deploymentMu.Lock()
	repo.deploymentRows[app.ID] = model.DeploymentAdmissionState{ApplicationID: app.ID,
		ResetGeneration: generation, ConfirmedTier: saved}
	if saved != nil {
		row := repo.deploymentRows[app.ID]
		row.DeploymentReason = new("confirmed fixture evidence")
		repo.deploymentRows[app.ID] = row
	}
	repo.deploymentMu.Unlock()
	key := deploymentIdentity(app)
	s.deployments.publish(key, deploymentSnapshot{Acknowledged: true, ResetGeneration: generation,
		Assessment: deploymentAssessment{State: state}})
}

func (r *deliveryRepoFake) GetDeploymentCompletionState(ctx context.Context,
	identity repository.DeploymentIdentity, epoch uint64,
) (*repository.DeploymentCompletionState, error) {
	if r.deploymentCompletion != nil {
		return r.deploymentCompletion(ctx, identity, epoch)
	}
	completion, err := r.GetEpochCompletionState(ctx, identity.ApplicationAddress.Hex(), epoch)
	if err != nil {
		return nil, err
	}
	row, err := r.GetDeploymentAdmissionState(ctx, identity)
	return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, err
}
