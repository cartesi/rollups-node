// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublishedRecoveryWaitsForPaidObservation(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.adapter.recovery.Disposition = model.BondDispositionRecovered
	for _, head := range []uint64{120, 110, 130} {
		f.block.Number = head
		call, err := f.plan(t.Context())
		require.NoError(t, err)
		require.Nil(t, call, "RECOVERED live state refuses duplicate payment despite a stale published candidate")
	}
	require.Empty(t, f.backend.published)
	require.Empty(t, f.repo.windows, "live suppression cannot persist payment")
	f.adapter.recovery.Disposition = model.BondDispositionRecoverable
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.NotNil(t, call, "a changed canonical live state reverses temporary suppression")
}

func TestPublishedRecoveryPrunesPaidRootsAbsentFromQuery(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.repo.recoveries = nil
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.Nil(t, call)
	require.Empty(t, f.d.rootBondRecoveryScans, "an empty published page ends its finite sweep")
	require.Empty(t, f.adapter.reads)
}

func TestPublishedRecoveryUsesProtectedFactsAndPerRowCoverage(t *testing.T) {
	for _, bounds := range []struct{ facts, cursor uint64 }{{100, 120}, {120, 100}} {
		f := newRecoveryPortFixture(t)
		r := &recoveryRepositoryPort{deliveryRepoFake: f.repo}
		f.d.repository = r
		f.app.LastTournamentCheckBlock = bounds.cursor
		call, err := f.d.planAction(t.Context(), f.app, recoverBond, f.block, txsender.BlockRef{Number: bounds.facts})
		require.NoError(t, err)
		require.NotNil(t, call)
		require.Equal(t, bounds.facts, r.queries[0].observed)
	}
}

func TestPublishedRecoveryPaidMarkersAreIsolatedAndLostOnRestart(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.adapter.recovery.Disposition = model.BondDispositionRecovered
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.Nil(t, call)
	restart := newRecoveryPortFixture(t)
	call, err = restart.plan(t.Context())
	require.NoError(t, err)
	require.NotNil(t, call, "restart reconstructs from protected bond state rather than stale local paid markers")
	require.Empty(t, restart.repo.windows)
}
