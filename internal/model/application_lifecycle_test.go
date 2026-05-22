// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplicationLifecycleHelpers(t *testing.T) {
	app := &Application{Enabled: true, Status: ApplicationStatus_OK}

	require.True(t, app.CanExecute())
	require.True(t, app.NeedsL1Observation())
	require.True(t, app.NeedsForeclosureObservation())
	require.False(t, app.NeedsPostForeclosureObservation())

	app.ForecloseBlock = 42
	require.False(t, app.CanExecute())
	require.True(t, app.NeedsL1Observation())
	require.False(t, app.NeedsForeclosureObservation())
	require.True(t, app.NeedsPostForeclosureObservation())

	app.ForecloseBlock = 0
	app.Status = ApplicationStatus_Diverged
	require.False(t, app.CanExecute())
	require.True(t, app.NeedsL1Observation())
	require.True(t, app.NeedsForeclosureObservation())

	app.Enabled = false
	require.False(t, app.CanExecute())
	require.False(t, app.NeedsL1Observation())
	require.False(t, app.NeedsForeclosureObservation())
}

// TestForeclosureScanCaughtUp pins the single drain-readiness definition shared
// by the claimer, PRT, and manager: it consults last_input_check_block for
// IConsensus apps and last_epoch_check_block for DaveConsensus apps, and the
// foreclose_block boundary is inclusive (cursor == foreclose_block is caught up).
func TestForeclosureScanCaughtUp(t *testing.T) {
	t.Run("IConsensus uses last_input_check_block", func(t *testing.T) {
		app := &Application{ConsensusType: Consensus_Authority, ForecloseBlock: 100}

		app.LastInputCheckBlock = 99
		require.False(t, app.ForeclosureScanCaughtUp(), "below bound: not caught up")

		app.LastInputCheckBlock = 100
		require.True(t, app.ForeclosureScanCaughtUp(), "at bound: caught up (inclusive)")

		app.LastInputCheckBlock = 101
		require.True(t, app.ForeclosureScanCaughtUp(), "past bound: caught up")

		// The epoch cursor must not influence an IConsensus app.
		app.LastInputCheckBlock = 99
		app.LastEpochCheckBlock = 1000
		require.False(t, app.ForeclosureScanCaughtUp(), "epoch cursor is ignored for IConsensus")
	})

	t.Run("DaveConsensus uses last_epoch_check_block", func(t *testing.T) {
		app := &Application{ConsensusType: Consensus_PRT, ForecloseBlock: 100}

		app.LastEpochCheckBlock = 99
		require.False(t, app.ForeclosureScanCaughtUp(), "below bound: not caught up")

		app.LastEpochCheckBlock = 100
		require.True(t, app.ForeclosureScanCaughtUp(), "at bound: caught up (inclusive)")

		// The input cursor must not influence a DaveConsensus app.
		app.LastEpochCheckBlock = 99
		app.LastInputCheckBlock = 1000
		require.False(t, app.ForeclosureScanCaughtUp(), "input cursor is ignored for DaveConsensus")
	})
}
