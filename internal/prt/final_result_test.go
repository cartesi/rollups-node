// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	finalMachineResultComponent = "machine"
	finalOutputsResultComponent = "outputs"
	matchingFinalResult         = "matching"
)

func TestTournamentFinalResultPlansForeignCommitment(t *testing.T) {
	for _, kind := range []actionKind{stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, kind == acceptTournament)
			view.stage.WinnerCommitment = common.HexToHash("0x600")
			view.accept.IsClaimStagingPeriodOver = true
			f.snapshot(view)
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
			require.NoError(t, err)
			require.NotNil(t, call)
			require.Equal(t, kind, call.Key.Kind)
			contractABI, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
			require.NoError(t, err)
			method, err := contractABI.MethodById(call.Data[:4])
			require.NoError(t, err)
			if kind == stageTournament {
				require.Equal(t, "stageTournamentResult", method.Name)
				proof, err := f.epoch.StateProof()
				require.NoError(t, err)
				require.Equal(t, *f.epoch.MachineHash, proof.MachineHash)
				require.Equal(t, *f.epoch.TxBufferDataBlock, proof.TxBufferDataBlock)
			} else {
				require.Equal(t, "acceptStagedTournamentResult", method.Name)
			}
			report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, *call})
			require.NoError(t, err)
			require.True(t, report.Attempted)
			require.NotEmpty(t, f.backend.published)
			require.Equal(t, call.Data, f.backend.published[0].Data())
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestTournamentFinalResultObserverIgnoresCommitmentIdentity(t *testing.T) {
	for _, falseState := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching state", true: "false state"}[falseState], func(t *testing.T) {
			service, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			foreign, final := common.HexToHash("0x600"), *epoch.MachineHash
			if falseState {
				final[0] ^= 1
			}
			root := &model.Tournament{Snapshot: model.TournamentSnapshot{
				AsOfBlock: 100, FinishedAtBlock: 90, WinnerCommitment: &foreign, FinalStateHash: &final,
			}}
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
				Return(root, nil).Once()
			statusWrite := repo.On("UpdateApplicationStatus", mock.Anything, app.ID,
				model.ApplicationStatus_Diverged, mock.Anything).Return(nil)
			if falseState {
				statusWrite.Once()
			} else {
				statusWrite.Maybe()
			}
			if falseState {
				expectStoredProtection(t, repo, app, epoch)
			}
			deferred, err := service.reconcileAcceptedEpochs(t.Context(), app, []*model.Epoch{epoch}, nil, 100)
			if falseState {
				require.ErrorContains(t, err, "inconsistent machine hash")
				require.True(t, deferred)
				require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
			} else {
				require.NoError(t, err)
				require.False(t, deferred)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestTournamentFinalResultObservationChecksBothComponents(t *testing.T) {
	for _, component := range []string{matchingFinalResult, finalMachineResultComponent, finalOutputsResultComponent} {
		t.Run(component, func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, true)
			view.stage.WinnerCommitment = common.HexToHash("0x600")
			switch component {
			case finalMachineResultComponent:
				view.stage.WinnerPostEpochMachineStateHash[0] ^= 1
				view.sealed.StagedPostEpochMachineStateHash = view.stage.WinnerPostEpochMachineStateHash
				view.accept.StagedPostEpochMachineStateHash = view.stage.WinnerPostEpochMachineStateHash
			case finalOutputsResultComponent:
				view.sealed.StagedPostEpochOutputsMerkleRoot[0] ^= 1
				view.accept.StagedPostEpochOutputsMerkleRoot = view.sealed.StagedPostEpochOutputsMerkleRoot
			}
			require.NoError(t, validateDaveConsensusSnapshot(view, 100))
			if component == matchingFinalResult {
				f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
					view.sealed.StagingBlockNumber).Return(nil).Once()
			} else {
				f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID,
					model.ApplicationStatus_Diverged, mock.Anything).Return(nil).Once()
				f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index,
					view.sealed.StagingBlockNumber).Return(nil).Maybe()
			}
			if component != matchingFinalResult {
				expectStoredProtection(t, f.repo.prtRepositoryMock, f.app, f.epoch)
			}
			err := f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view)
			if component == matchingFinalResult {
				require.NoError(t, err)
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				require.Equal(t, model.EpochStatus_ClaimStaged, f.epoch.Status)
			} else {
				require.ErrorContains(t, err, "inconsistent")
				require.Equal(t, model.ApplicationStatus_Diverged, f.app.Status)
				f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
			f.repo.AssertExpectations(t)
		})
	}
}

func TestTournamentFinalResultAcceptsForeignCommitment(t *testing.T) {
	f := newAcceptanceReceiptFixture(t)
	foreign := common.HexToHash("0x600")
	f.root.Snapshot.WinnerCommitment = &foreign
	f.expectReceipt()
	f.expectAcceptance()
	deferred, err := f.service.reconcileAcceptedEpochs(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, 100)
	require.NoError(t, err)
	require.False(t, deferred)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentFinalResultRechecksAfterSigning(t *testing.T) {
	for _, kind := range []actionKind{stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, kind == acceptTournament)
			view.stage.WinnerCommitment = common.HexToHash("0x600")
			view.accept.IsClaimStagingPeriodOver = true
			f.snapshot(view)
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
			require.NoError(t, err)
			require.NotNil(t, call)
			factory := deliverySigningFactory{TransactOptsFactory: f.d.txOptsFactory, afterSign: func() {
				f.app.Status = model.ApplicationStatus_Failed
			}}
			sender, err := txsender.New[actionID](f.backend, factory, 31337, f.d.deliveryPolicy, f.d.guardPublication, f.d.Logger)
			require.NoError(t, err)
			report, err := sender.PublishNext(t.Context(), singleAction{f.d, *call})
			require.Error(t, err)
			require.False(t, report.Attempted)
			require.Empty(t, f.backend.published)
			require.Equal(t, model.ApplicationStatus_Failed, f.app.Status)
		})
	}
}

func TestTournamentFinalResultPlansRejectFalseState(t *testing.T) {
	f := newDeliveryFixture(t)
	view := resultTestSnapshot(f.epoch, false)
	view.stage.WinnerCommitment = common.HexToHash("0x600")
	view.stage.WinnerPostEpochMachineStateHash[0] ^= 1
	f.snapshot(view)
	call, err := f.d.planAction(t.Context(), f.app, stageTournament, f.block, observerFacts(100))
	require.ErrorContains(t, err, "inconsistent machine hash")
	require.Nil(t, call)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
