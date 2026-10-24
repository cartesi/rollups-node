// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestForecloseClaimDoesNotAnnounceFailedWrite(t *testing.T) {
	s, repo, _ := newServiceMock(t)
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	app := withForeclosed(makeApplication(), 105)
	epoch := makeStagedEpoch(app, 3, 50)
	repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).
		Return(repository.ErrNoUpdate).Once()
	err := s.forecloseClaim(t.Context(), app, epoch, 105, "test")
	require.ErrorIs(t, err, repository.ErrNoUpdate)
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	require.NotContains(t, logs.String(), "Claim made terminal by application foreclosure")
	repo.AssertExpectations(t)
}
