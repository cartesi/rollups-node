// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/require"
)

func TestPRTFreshSealedEpochWaitsForIngestionWithoutPlanningError(t *testing.T) {
	f := newDeliveryFixture(t)
	f.snapshot(resultTestSnapshot(f.epoch, false))
	f.repo.stateRead = func(context.Context, string, uint64) (*repository.EpochCompletionState, error) {
		return &repository.EpochCompletionState{Application: f.app}, nil
	}
	call, err := f.d.planAction(t.Context(), f.app, joinTournament, f.block, observerFacts(100))
	require.Nil(t, call)
	require.ErrorIs(t, err, txsender.ErrIneligible, "a sealed epoch can precede its database row during normal ingestion")
}

func TestPRTMissingEpochRetainsSignedWorkUntilIngestion(t *testing.T) {
	f, call, _ := preparePRTAction(t, stageTournament)
	submitPRTAction(t, f, call)
	f.repo.stateRead = func(context.Context, string, uint64) (*repository.EpochCompletionState, error) {
		return &repository.EpochCompletionState{Application: f.app}, nil
	}
	eligibility, err := f.d.Recheck(t.Context(), f.block, call.Key)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.True(t, txsender.WaitOnly(err))
	require.Equal(t, txsender.ProtocolWait, eligibility)
	// This read supplies no independent evidence for a priority replacement.
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.True(t, txsender.WaitOnly(err))
	require.False(t, report.Attempted)
	require.Equal(t, call.Key, report.Current.Action)
	require.False(t, report.Current.Neutral)
	require.Len(t, f.backend.published, 1)
	f.repo.stateRead = nil
	report, err = f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.True(t, report.Attempted)
	require.Len(t, f.backend.published, 2)
	require.Equal(t, f.backend.raw[0], f.backend.raw[1])
}
