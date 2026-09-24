// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResultPublicationMakesProgressWithContinuousObserverLag(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	tournament := resultBoundaryTournament(f, epoch, *epoch.Commitment)
	var stageWriteCoverage []uint64
	f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, epoch.Index, uint64(100)).
		Run(func(mock.Arguments) {
			require.Equal(t, uint64(100), tournament.Snapshot.AsOfBlock, "publish the result window before its staged marker")
			stageWriteCoverage = append(stageWriteCoverage, f.app.LastTournamentCheckBlock)
		}).Return(nil).Once()

	for _, published := range []uint64{99, 100, 101} {
		configured := published + 1
		f.app.LastEpochCheckBlock = published
		f.expectResultBoundaryWindow(epoch, tournament, published)
		staged := published >= 100
		snapshot := resultTestSnapshot(epoch, staged)
		if staged {
			snapshot.sealed.StagingBlockNumber = 100
		} else {
			snapshot.stage.IsFinished = false
			snapshot.stage.WinnerCommitment = common.Hash{}
			snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
		}
		// Bind every result read to the block that was actually published. A
		// read at the moving configured head cannot establish this checkpoint.
		f.expectResultBoundarySnapshot(epoch, snapshot, published)
		require.NoError(t, f.s.validateApplication(t.Context(), f.app, configured))
		require.Equal(t, published, f.app.LastTournamentCheckBlock)
		require.Equal(t, published, tournament.Snapshot.AsOfBlock)
		require.Less(t, f.app.LastTournamentCheckBlock, configured, "the observer never catches the configured head")
		require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		if staged {
			require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
			require.Equal(t, new(uint64(100)), epoch.StagedAtBlock)
			require.Equal(t, []uint64{100}, stageWriteCoverage, "record once at the next tick, then retain the marker")
		} else {
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
			require.Nil(t, epoch.StagedAtBlock)
			require.Empty(t, stageWriteCoverage, "block 99 must not publish the stage from block 100")
		}
	}
	f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.Empty(t, f.s.pendingTransactions, "reader mode must not send transactions")
}
