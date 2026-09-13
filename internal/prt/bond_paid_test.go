// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestPublishedRecoveryWaitsForPaidObservation(t *testing.T) {
	f := newPublishedBondFixture(t)
	var logs bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	candidate := bondCandidate(0)
	rows := []repository.RootBondRecoveryCandidate{candidate}
	f.expectBoundary(0)
	f.expectPage(100, nil, 0, rows, nil)
	f.expectPreflight(t, candidate, canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))

	// Published state still says RECOVERABLE. Neither repeated latest reads nor
	// a lagging provider may cause another preflight or payment attempt.
	for _, latest := range []uint64{120, 110, 130} {
		f.expectBoundary(0)
		f.expectPage(100, nil, 0, rows, nil)
		require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, latest))
	}
	require.Equal(t, 1, strings.Count(logs.String(), "Root tournament bond is recovered"))
	require.Equal(t, 3, strings.Count(logs.String(), "level=DEBUG msg=\"Root bond payment awaits published observation\""))

	// The marker is not a permanent payment verdict. Once observation reaches
	// that block, a recoverable row can be checked again after a changed view.
	f.app.LastTournamentCheckBlock = 120
	f.expectBoundary(0)
	f.expectPage(120, nil, 0, rows, nil)
	f.expectBroadcast(t, candidate)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 120, 120))
	require.Empty(t, f.s.paidRootBondObservations)
}

func TestPublishedRecoveryPrunesPaidRootsAbsentFromQuery(t *testing.T) {
	f := newPublishedBondFixture(t)
	f.s.recordPaidRootBond(f.app.ID, bondCandidate(0).Tournament, 120)
	f.app.LastTournamentCheckBlock = 120
	f.expectBoundary(0)
	f.expectPage(120, nil, 0, nil, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 120, 130))
	require.Empty(t, f.s.paidRootBondObservations, "published paid roots no longer appear in the query")
}

func TestPublishedRecoveryPaidMarkersUseBothObservationBounds(t *testing.T) {
	for _, bounds := range []struct{ configured, published uint64 }{{100, 120}, {120, 100}} {
		f := newPublishedBondFixture(t)
		candidate := bondCandidate(0)
		f.s.recordPaidRootBond(f.app.ID, candidate.Tournament, 120)
		f.app.LastTournamentCheckBlock = bounds.published
		f.expectBoundary(0)
		f.expectPage(100, nil, 0, []repository.RootBondRecoveryCandidate{candidate}, nil)
		require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, bounds.configured, 130))
		require.Equal(t, uint64(120), f.s.paidRootBondObservations[f.app.ID][candidate.Tournament])
	}
}

func TestPublishedRecoveryPaidMarkersAreIsolatedAndLostOnRestart(t *testing.T) {
	f := newPublishedBondFixture(t)
	candidate := bondCandidate(0)
	// A marker for another application must not suppress this application's root.
	f.s.recordPaidRootBond(f.app.ID+1, candidate.Tournament, 120)
	f.expectBoundary(0)
	f.expectPage(100, nil, 0, []repository.RootBondRecoveryCandidate{candidate}, nil)
	f.expectPreflight(t, candidate, canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Len(t, f.s.paidRootBondObservations, 2)

	// Reconstruct the service, as restart does. It must check the published row
	// again; no paid verdict is persisted outside the observed contract state.
	f = newPublishedBondFixture(t)
	f.expectBoundary(0)
	f.expectPage(100, nil, 0, []repository.RootBondRecoveryCandidate{candidate}, nil)
	f.expectBroadcast(t, candidate)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
}
