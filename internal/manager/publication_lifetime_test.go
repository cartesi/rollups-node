// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func foreclosedPublicationManager(t *testing.T, consensus model.Consensus, undrained bool) (
	*MachineManager, *MockMachineRepository, *DummyMachineInstanceMock,
) {
	t.Helper()
	app := &model.Application{ID: 17, Name: "closed-before-foreclosure", Enabled: true,
		Status: model.ApplicationStatus_OK, ConsensusType: consensus,
		ForecloseBlock: 80, LastInputCheckBlock: 80, LastEpochCheckBlock: 80, ProcessedInputs: 2}
	repo := &MockMachineRepository{}
	repo.On("ListApplications", mock.Anything, repository.ExecutableApplicationsFilter(), repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), nil)
	repo.On("ListApplications", mock.Anything, foreclosedMachineDrainFilter(), repository.Pagination{}, false).
		Return([]*model.Application{app}, uint64(1), nil)
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(undrained, nil)
	m := NewMachineManager(repo, slog.New(slog.NewTextHandler(io.Discard, nil)), false, 1)
	instance := &DummyMachineInstanceMock{application: app, processedInputs: 2}
	m.addMachine(app.ID, instance)
	t.Cleanup(m.Close)
	return m, repo, instance
}

func TestForeclosedMachineWaitsForProofPublication(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(string(consensus), func(t *testing.T) {
			m, repo, instance := foreclosedPublicationManager(t, consensus, false)
			app := instance.Application()
			// The reader closed a pre-F epoch after its inputs already committed.
			// Only proof publication needs the resident machine now.
			repo.On("HasUnpublishedEpochBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(true, nil).Once()
			repo.On("HasUnpublishedEpochBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
			require.NoError(t, m.UpdateMachines(t.Context()))
			require.True(t, m.HasMachine(app.ID), "CLOSED with accepted inputs still needs its publication runtime")
			require.Zero(t, instance.closeCalls)
			// CLOSED -> INPUTS_PROCESSED discharges machine ownership. Validator
			// and claim reconciliation may still have work, entirely in the DB.
			require.NoError(t, m.UpdateMachines(t.Context()))
			require.False(t, m.HasMachine(app.ID), "do not retain the runtime until CLAIM_COMPUTED or claim drain")
			require.Equal(t, 1, instance.closeCalls)
			repo.AssertExpectations(t)
		})
	}
}

func TestForeclosedMachinePublicationReadErrorKeepsRuntime(t *testing.T) {
	m, repo, instance := foreclosedPublicationManager(t, model.Consensus_Authority, false)
	app := instance.Application()
	failure := errors.New("publication progress unavailable")
	repo.On("HasUnpublishedEpochBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, failure).Once()
	repo.On("HasUnpublishedEpochBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
	require.ErrorIs(t, m.UpdateMachines(t.Context()), failure)
	require.True(t, m.HasMachine(app.ID), "an unavailable progress read cannot authorize teardown")
	require.Zero(t, instance.closeCalls)
	require.NoError(t, m.UpdateMachines(t.Context()))
	require.False(t, m.HasMachine(app.ID))
	repo.AssertExpectations(t)
}

func TestForeclosedMachinePublicationDoesNotExpandPRTOrUndrainedPath(t *testing.T) {
	for _, test := range []struct {
		name      string
		consensus model.Consensus
		undrained bool
	}{
		{"PRT remains input-drain only", model.Consensus_PRT, false},
		{"pending A/Q inputs already retain runtime", model.Consensus_Authority, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, repo, instance := foreclosedPublicationManager(t, test.consensus, test.undrained)
			require.NoError(t, m.UpdateMachines(context.Background()))
			require.Equal(t, test.undrained, m.HasMachine(instance.Application().ID))
			repo.AssertNotCalled(t, "HasUnpublishedEpochBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}
