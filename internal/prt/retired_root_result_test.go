// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetiredRootDoesNotHideLaterStaging(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.LastTournamentCheckBlock = 90
	epoch := checkpointEpoch(0, "0x100")
	f.epochs(epoch)
	root := resultBoundaryTournament(f, epoch, *epoch.Commitment)
	root.Snapshot.AsOfBlock = 90
	root.Snapshot.FinishedAtBlock = 80
	root.Snapshot.Standing = model.TournamentStandingRootWinner
	root.Snapshot.AcceptsJoins = false
	root.Snapshot.WinnerCommitment = epoch.Commitment
	root.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
	require.True(t, tournamentObservationComplete(root, 90, 100))
	f.repo.storedTournaments = []*model.Tournament{root}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), root.Address.Hex()).Return(root, nil)
	snapshot := resultTestSnapshot(epoch, true)
	snapshot.sealed.StagingBlockNumber = 100
	f.expectResultBoundarySnapshot(epoch, snapshot, 100)
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, epoch.Index, uint64(100)).Return(nil).Once()

	expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), epoch)
	require.NoError(t, f.s.observeApplication(t.Context(), f.app, observerFacts(100)))
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	require.Equal(t, uint64(90), root.Snapshot.AsOfBlock, "an immutable root needs no new tournament scan")
}

func TestRetiredRootDoesNotHideLaterAcceptance(t *testing.T) {
	f := newAcceptanceReceiptFixture(t)
	root := f.repo.ExpectedCalls[0].ReturnArguments.Get(0).(*model.Tournament)
	root.Snapshot.AsOfBlock = 90
	root.Snapshot.Standing = model.TournamentStandingRootWinner
	root.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
	require.True(t, tournamentObservationComplete(root, 90, 100))
	f.expectReceipt()
	f.expectAcceptance()

	deferred, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
	require.NoError(t, err)
	require.False(t, deferred, "a canonical consensus acceptance at finalized100 can follow retirement at90")
}
