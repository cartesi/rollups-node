// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSupervisorKeepsPRTReadyThroughPerApplicationObservationFailures(t *testing.T) {
	s, repo := newPRTServiceMock()
	require.NoError(t, service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{Name: config.ServicePrt, Logger: s.Logger}, PollInterval: time.Hour,
	}, s))
	started := make(chan struct{})
	// Stop the first tick before it changes observer eligibility. This test
	// isolates the supervisor's use of the PRT-specific readiness method.
	repo.On("GetApplicationIDBoundary", mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { close(started) }).
		Return(int64(0), errors.New("application list unavailable")).Once()
	supervisor, err := service.NewSupervisor(t.Context(), &service.SupervisorConfigs{
		BaseConfigs: service.BaseConfigs{Name: "node", Logger: s.Logger},
		Factories: []service.FactoryFunction{
			func(context.Context, service.Supervisor) (service.SupervisedService, error) { return s, nil },
		},
	})
	require.NoError(t, err)
	finished := make(chan error, 1)
	go func() { finished <- supervisor.Serve() }()
	t.Cleanup(func() {
		supervisor.Stop()
		select {
		case err := <-finished:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Error("supervisor did not stop")
		}
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("PRT service did not start")
	}
	require.Empty(t, supervisor.NotReady())
	app := prtRevertTestApp()
	for head := uint64(100); head < 100+tournamentObservationFailureThreshold; head++ {
		s.recordTournamentObservationFailure(t.Context(), app, head, head, errors.New("observation unavailable"))
	}
	require.Empty(t, supervisor.NotReady(), "per-app failures do not change process prerequisites")
	s.clearTournamentObservationFailure(app.ID)
	require.Empty(t, supervisor.NotReady())
	repo.AssertExpectations(t)
}
