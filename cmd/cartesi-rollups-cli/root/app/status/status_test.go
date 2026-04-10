// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package status

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
)

type enableRepositoryStub struct {
	setEnabledCalls  int
	markRunningCalls int
	lastAppID        int64
	lastEnabled      bool
	err              error
}

func (s *enableRepositoryStub) SetApplicationEnabled(
	_ context.Context,
	appID int64,
	enabled bool,
) error {
	s.setEnabledCalls++
	s.lastAppID = appID
	s.lastEnabled = enabled
	return s.err
}

func (s *enableRepositoryStub) MarkApplicationRunning(
	_ context.Context,
	appID int64,
) error {
	s.markRunningCalls++
	s.lastAppID = appID
	return s.err
}

func TestEnableApplicationTransitionsStoppedApp(t *testing.T) {
	t.Parallel()

	repo := &enableRepositoryStub{}
	app := &model.Application{
		ID:      42,
		Enabled: false,
		Health:  model.ApplicationHealth_Stopped,
	}

	err := enableApplication(context.Background(), repo, app)
	require.NoError(t, err)
	assert.Equal(t, 1, repo.setEnabledCalls)
	assert.Equal(t, 1, repo.markRunningCalls)
	assert.Equal(t, int64(42), repo.lastAppID)
	assert.True(t, repo.lastEnabled)
}

func TestEnableApplicationRecoversFailedEnabledApp(t *testing.T) {
	t.Parallel()

	repo := &enableRepositoryStub{}
	app := &model.Application{
		ID:      43,
		Enabled: true,
		Health:  model.ApplicationHealth_Failed,
	}

	err := enableApplication(context.Background(), repo, app)
	require.NoError(t, err)
	assert.Equal(t, 0, repo.setEnabledCalls)
	assert.Equal(t, 1, repo.markRunningCalls)
	assert.Equal(t, int64(43), repo.lastAppID)
}

func TestEnableApplicationNoopsForRunningEnabledApp(t *testing.T) {
	t.Parallel()

	repo := &enableRepositoryStub{}
	app := &model.Application{
		ID:      44,
		Enabled: true,
		Health:  model.ApplicationHealth_Running,
	}

	err := enableApplication(context.Background(), repo, app)
	require.NoError(t, err)
	assert.Equal(t, 0, repo.setEnabledCalls)
	assert.Equal(t, 0, repo.markRunningCalls)
}
