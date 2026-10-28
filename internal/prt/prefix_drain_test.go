// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPRTForeclosurePreservesPrefixAwaitingValidator(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_OK,
		model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.Status = status
			app.ForecloseBlock, app.LastEpochCheckBlock, app.LastInputCheckBlock = 100, 100, 100
			epoch := checkpointEpoch(0, "0x100")
			epoch.Status = model.EpochStatus_InputsProcessed
			if status != model.ApplicationStatus_OK {
				repo.On("GetEpochCompletionState", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).
					Return(&repository.EpochCompletionState{Application: app, Epoch: epoch, Problem: prtTestProblem(status, 1)}, nil).Once()
			}
			repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
			repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(true, nil).Once()
			repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
				repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()

			require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 100))
			repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func TestPRTTerminalPrefixAcceptancePrecedesForeclosureDrain(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_MachineHalted
	f.app.ForecloseBlock, f.app.LastEpochCheckBlock, f.app.LastInputCheckBlock = 100, 100, 100
	f.app.LastTournamentCheckBlock = 100
	epoch := checkpointEpoch(0, "0x100")
	epoch.ClaimTransactionHash = new(common.HexToHash("0xabc"))
	problem := prtTestProblem(f.app.Status, 1)
	f.repo.On("GetEpochCompletionState", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.Index).
		Return(&repository.EpochCompletionState{Application: f.app, Epoch: epoch, Problem: problem}, nil).Once()
	tournament := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
		Snapshot: model.TournamentSnapshot{FinishedAtBlock: 90, AsOfBlock: 100, WinnerCommitment: epoch.Commitment}}
	f.acceptance(epoch, tournament, nil)
	var order []string
	for _, call := range f.repo.ExpectedCalls {
		if call.Method == "UpdateEpochWithAcceptedClaim" {
			call.Run(func(mock.Arguments) { order = append(order, "accepted earlier epoch") })
		}
	}
	suffix := &model.Epoch{ApplicationID: f.app.ID, Index: 1, FirstBlock: 20, LastBlock: 30, Status: model.EpochStatus_Closed}
	f.repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, f.app.ID, uint64(100)).Return(false, nil).Once()
	f.repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, f.app.ID, uint64(100)).Return(true, nil).Once()
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch, suffix}, uint64(2), nil).Once()
	f.repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, f.app.ID, suffix.Index).
		Run(func(mock.Arguments) { order = append(order, "foreclosed stopped suffix") }).Return(nil).Once()

	require.NoError(t, f.s.drainForeclosedClaims(t.Context(), f.app, []*model.Epoch{epoch, suffix}, f.consensus, 100))

	require.Equal(t, []string{"accepted earlier epoch", "foreclosed stopped suffix"}, order)
	f.repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, f.app.ID, epoch.Index)
	require.Equal(t, model.ApplicationStatus_MachineHalted, f.app.Status)
}

func TestPRTTerminalPrefixDrainWaitsForOwningRootObservation(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "root interval incomplete", true: "root absent"}[missing], func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.Status = model.ApplicationStatus_MachineHalted
			app.ForecloseBlock, app.LastEpochCheckBlock, app.LastInputCheckBlock = 100, 100, 100
			epoch := checkpointEpoch(0, "0x100")
			repo.On("GetEpochCompletionState", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).
				Return(&repository.EpochCompletionState{Application: app, Epoch: epoch, Problem: prtTestProblem(app.Status, 1)}, nil).Once()
			var root *model.Tournament
			if !missing {
				root = &model.Tournament{Address: *epoch.TournamentAddress,
					Snapshot: model.TournamentSnapshot{AsOfBlock: 99, FinishedAtBlock: 90, WinnerCommitment: epoch.Commitment}}
			}
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(root, nil).Once()
			repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(false, nil).Once()
			repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(true, nil).Once()

			require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch}, nil, 100))

			repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false)
			repo.AssertExpectations(t)
		})
	}
}

func TestPRTForeclosureDoesNotTreatGlobalStopAsCutoffEvidence(t *testing.T) {
	for _, change := range []string{disabledApplicationCase, failedApplicationCase, divergedApplicationCase,
		corruptedApplicationCase, removedApplicationCase} {
		t.Run(change, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.Status = model.ApplicationStatus_MachineHalted
			app.ForecloseBlock = 100
			epoch := checkpointEpoch(0, "0x100")
			epoch.Status = model.EpochStatus_InputsProcessed
			current := *app
			state := &repository.EpochCompletionState{Application: &current, Epoch: epoch, Problem: prtTestProblem(app.Status, 1)}
			switch change {
			case disabledApplicationCase:
				current.Enabled = false
			case failedApplicationCase:
				current.Status = model.ApplicationStatus_Failed
			case divergedApplicationCase:
				current.Status = model.ApplicationStatus_Diverged
			case corruptedApplicationCase:
				current.Status = model.ApplicationStatus_Corrupted
			case removedApplicationCase:
				state = nil
			}
			repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
				repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
			repo.On("GetEpochCompletionState", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).Return(state, nil).Once()

			require.ErrorIs(t, s.foreclosePendingClaimEpochs(t.Context(), app), txsender.ErrIneligible)

			repo.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}
