// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
)

func TestDeploymentEpisodeMissingControlRefusesWithoutRepair(t *testing.T) {
	s := newDeploymentObserverService()
	r := s.repository.(*prtRepositoryMock)
	app := prtRevertTestApp()
	delete(r.deploymentRows, app.ID)
	captured, err := s.deploymentEpisode(t.Context(), app)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.False(t, captured.snapshot.Acknowledged)
	require.Empty(t, r.deploymentRows)
}
