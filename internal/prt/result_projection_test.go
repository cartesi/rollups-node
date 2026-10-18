// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResultSnapshotAllowsZeroFinalState(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	epoch.MachineHash = new(common.Hash{})
	snapshot := resultTestSnapshot(epoch, false)
	require.NoError(t, validateDaveConsensusSnapshot(snapshot, 20))
	require.NoError(t, matchConsensusSnapshotToEpoch(epoch, snapshot))
}

func TestFailedRootRepeatedObservationAndForeclosure(t *testing.T) {
	app := prtRevertTestApp()
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	snapshot := resultTestSnapshot(epoch, false)
	snapshot.stage.IsTournamentFailed = true
	snapshot.stage.WinnerCommitment = common.Hash{}
	snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
	service, repo := newPRTServiceMock()
	for range 2 {
		require.NoError(t, service.recordTournamentResult(t.Context(), app, epoch, snapshot))
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
	}

	app.ForecloseBlock = 100
	app.LastEpochCheckBlock = 100
	app.LastInputCheckBlock = 100
	app.LastTournamentCheckBlock = 100
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(true, nil).Once()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(
		&model.Tournament{Address: *epoch.TournamentAddress, Snapshot: model.TournamentSnapshot{
			AsOfBlock: 100, FinishedAtBlock: 90, Standing: model.TournamentStandingRootFailed,
		}}, nil).Once()
	repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).Return(nil).Once()

	require.NoError(t, service.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 100))
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}
