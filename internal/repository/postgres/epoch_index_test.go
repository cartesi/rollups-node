// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
)

func TestGetLastNonOpenEpochIndex(t *testing.T) {
	r := newEpochPublicationRepository(t)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	other := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
	check := func(want *uint64) {
		t.Helper()
		for _, identifier := range []string{app.Name, app.IApplicationAddress.Hex()} {
			got, err := r.GetLastNonOpenEpochIndex(t.Context(), identifier)
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	}
	store := func(app *model.Application, index uint64, status model.EpochStatus) {
		t.Helper()
		initialStatus := status
		if status == model.EpochStatus_ClaimAccepted || status == model.EpochStatus_ClaimStaged {
			initialStatus = model.EpochStatus_Closed
		}
		epoch := repotest.NewEpochBuilder(app.ID).WithIndex(index).WithStatus(initialStatus).Build()
		require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name,
			map[*model.Epoch][]*model.Input{epoch: {}}, 100))
		if status == model.EpochStatus_ClaimStaged {
			repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, model.EpochStatus_ClaimComputed)
			require.NoError(t, r.UpdateEpochReconciledStaged(t.Context(), app.ID, index, 100))
		} else if initialStatus != status {
			repotest.AdvanceEpochStatus(t.Context(), t, r, app.Name, epoch, status)
		}
	}

	check(nil)
	store(app, 10, model.EpochStatus_Open)
	store(other, 50, model.EpochStatus_Closed)
	check(nil) // Neither an open epoch nor another application's epoch is a boundary.
	store(app, 0, model.EpochStatus_Closed)
	check(new(uint64(0)))
	store(app, 3, model.EpochStatus_ClaimAccepted)
	check(new(uint64(3)))
	store(app, 2, model.EpochStatus_ClaimStaged)
	check(new(uint64(3))) // Insertion order must not determine the boundary.
	store(app, 4, model.EpochStatus_ClaimForeclosed)
	check(new(uint64(4)))
}
