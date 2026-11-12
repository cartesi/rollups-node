// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
)

const failedRootAlert = "Root tournament finished without a winner; consensus cannot advance."

func TestFailedRootAlertRepeatsAcrossTicksRestartAndForeclosure(t *testing.T) {
	for _, submit := range []bool{false, true} {
		t.Run(fmt.Sprintf("submitting=%t", submit), func(t *testing.T) {
			f, client := tickFixture(t)
			f.d.submissionEnabled = submit
			root := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index,
				Address: *f.epoch.TournamentAddress, StartInstant: f.epoch.LastBlock, Allowance: 90,
				Snapshot: model.TournamentSnapshot{AsOfBlock: 100, FinishedAtBlock: 90,
					Standing:     model.TournamentStandingRootFailed,
					BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionNoWinner}}}
			f.repo.storedTournaments = []*model.Tournament{root}
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
				repository.Pagination{}, false).Return([]*model.Epoch{f.epoch}, uint64(1), nil)
			view := resultTestSnapshot(f.epoch, false)
			view.stage.IsTournamentFailed = true
			view.stage.WinnerCommitment, view.stage.WinnerPostEpochMachineStateHash = common.Hash{}, common.Hash{}
			f.block = observerFacts(100)
			f.snapshot(view)
			var output bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&output, nil))
			observer := f.d.Service
			beforeApp, beforeEpoch, beforeRoot := *f.app, *f.epoch, *root
			for turn := range 4 {
				if turn == 2 {
					observer, _ = newPRTServiceMock()
					observer.repository, observer.adapterFactory, observer.client = f.repo, f.factory, client
					observer.submissionEnabled, observer.defaultBlock = submit, model.DefaultBlock_Finalized
					observer.Logger = f.d.Logger
				}
				if turn == 3 {
					f.app.ForecloseBlock, f.app.LastInputCheckBlock, f.app.LastEpochCheckBlock = 100, 100, 100
					f.repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, f.app.ID, uint64(100)).Return(false, nil).Once()
					f.repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, f.app.ID, uint64(100)).Return(false, nil).Once()
				}
				output.Reset()
				tickHead(client, 100)
				reschedule, err := observer.Tick(t.Context())
				require.NoError(t, err)
				require.False(t, reschedule)
				if turn < 3 {
					require.Equal(t, 1, strings.Count(output.String(), failedRootAlert), "turn %d", turn)
					var alert string
					for _, line := range strings.Split(output.String(), "\n") {
						if strings.Contains(line, failedRootAlert) {
							alert = line
							break
						}
					}
					require.Contains(t, alert, "level=ERROR")
					require.Contains(t, alert, "If a guardian is configured, ask the guardian now to confirm the failure")
					require.Contains(t, alert, "IApplication.foreclose()")
					require.Contains(t, alert, "New inputs and portal deposits can still be accepted until foreclosure.")
					require.Contains(t, alert, "No protocol deadline applies to this call.")
					require.Contains(t, alert, "application="+f.app.Name)
					require.Contains(t, alert, "address="+f.app.IApplicationAddress.Hex())
					require.Contains(t, alert, "epoch_index=3")
					require.Contains(t, alert, "tournament="+root.Address.Hex())
					require.Equal(t, beforeApp, *f.app)
				} else {
					require.NotContains(t, output.String(), failedRootAlert)
				}
				require.Equal(t, beforeEpoch, *f.epoch)
				require.Equal(t, beforeRoot, *root)
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			}
			f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
			require.Empty(t, f.repo.windows, "a retired root must not be observed again")
			require.Empty(t, f.backend.published)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertExpectations(t)
			f.consensus.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestFailedRootAlertIgnoresOtherStandingAndUncoveredFacts(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.Application, *model.Tournament)
	}{
		{"winning root has no failure", func(_ *model.Application, root *model.Tournament) {
			root.Snapshot.Standing = model.TournamentStandingRootWinner
			root.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
		}},
		{"inner without winner", func(_ *model.Application, root *model.Tournament) {
			root.Level, root.CreationEvent = 1, &model.TournamentCreationEvent{}
			root.Snapshot.Standing = model.TournamentStandingInnerEliminableNoWinner
		}},
		{"root awaiting closure", func(_ *model.Application, root *model.Tournament) {
			root.Snapshot.Standing, root.Snapshot.FinishedAtBlock = model.TournamentStandingAwaitingClosure, 0
		}},
		{"snapshot beyond head", func(_ *model.Application, root *model.Tournament) { root.Snapshot.AsOfBlock = 101 }},
		{"finish beyond snapshot", func(_ *model.Application, root *model.Tournament) { root.Snapshot.FinishedAtBlock = 101 }},
		{disabledApplicationCase, func(app *model.Application, _ *model.Tournament) { app.Enabled = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			root := &model.Tournament{ApplicationID: f.app.ID, Address: common.HexToAddress("0x200"),
				Snapshot: model.TournamentSnapshot{AsOfBlock: 100, FinishedAtBlock: 90,
					Standing:     model.TournamentStandingRootFailed,
					BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionNoWinner}}}
			test.change(f.app, root)
			f.repo.storedTournaments = []*model.Tournament{root}
			var output bytes.Buffer
			f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, nil, f.consensus, observerFacts(100)))
			require.NotContains(t, output.String(), failedRootAlert)
			f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
		})
	}
}

func TestFailedRootAlertWaitsForSuccessfulCommit(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0xa0")
	epoch.Status = model.EpochStatus_Closed
	root := *epoch.TournamentAddress
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Twice()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), root.Hex()).
		Return((*model.Tournament)(nil), nil).Twice()
	for range 2 {
		adapter := &tournamentAdapterMock{}
		f.factory.On("CreateTournamentAdapter", root).Return(adapter, nil).Once()
		adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{BaseCycle: new(big.Int),
			Kind: model.TournamentKindLeaf, Height: 68, StartInstant: 10, Allowance: 20}, nil).Once()
		adapter.On("BondValue", mock.Anything).Return(big.NewInt(1), nil).Once()
		adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: model.TournamentStandingRootFailed,
			FinishedAt: 30}, nil).Once()
		expectTournamentAuxiliaryReads(adapter, mock.Anything, RootLevel, model.TournamentStandingRootFailed)
		adapter.On("RetrieveAllEvents", mock.Anything, mock.Anything).Return(&TournamentEvents{}, nil).Once()
		adapter.On("StructuralEventCounts", mock.Anything).Return(zeroStructuralEventCounts(), nil).Once()
		f.emptyParticipants(epoch, root)
		t.Cleanup(func() { adapter.AssertExpectations(t) })
	}
	failure := errors.New("interval commit failed")
	f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Run(func(mock.Arguments) {
		require.NotContains(t, output.String(), failedRootAlert, "an uncommitted standing cannot produce the alert")
	}).Return(failure).Once()
	require.ErrorIs(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100)), failure)
	require.NotContains(t, output.String(), failedRootAlert)
	require.Empty(t, f.repo.committedTournaments)
	f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Run(func(mock.Arguments) {
		require.NotContains(t, output.String(), failedRootAlert)
	}).Return(nil).Once()
	require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100)))
	require.Equal(t, model.TournamentStandingRootFailed, f.repo.committedTournaments[root].Snapshot.Standing)
	require.Equal(t, 1, strings.Count(output.String(), failedRootAlert))
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
}

func TestFailedRootAlertSurvivesConsensusReadFailures(t *testing.T) {
	failure := errors.New("consensus read unavailable")
	for _, test := range []struct {
		name       string
		cause      error
		deployment bool
	}{
		{"result snapshot unavailable", failure, false},
		{"result snapshot has no code", bind.ErrNoCode, false},
		{"deployment unavailable after restart", failure, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, client := tickFixture(t)
			root := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index,
				Address: *f.epoch.TournamentAddress, StartInstant: f.epoch.LastBlock, Allowance: 90,
				Snapshot: model.TournamentSnapshot{AsOfBlock: 100, FinishedAtBlock: 90,
					Standing:     model.TournamentStandingRootFailed,
					BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionNoWinner}}}
			f.repo.storedTournaments = []*model.Tournament{root}
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
				repository.Pagination{}, false).Return([]*model.Epoch{f.epoch}, uint64(1), nil)
			view := resultTestSnapshot(f.epoch, false)
			view.stage.IsTournamentFailed = true
			view.stage.WinnerCommitment, view.stage.WinnerPostEpochMachineStateHash = common.Hash{}, common.Hash{}
			f.consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil)
			f.consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil).Once()
			f.consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil)
			var output bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&output, nil))
			tickHead(client, 100)
			_, err := f.d.Tick(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(output.String(), failedRootAlert))
			if test.deployment {
				f.d.deployments = deploymentCache{}
				f.consensus.deploymentRead = func(context.Context, txsender.BlockRef) (DeploymentEvidence, error) {
					return DeploymentEvidence{}, test.cause
				}
				f.consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil).Once()
			} else {
				f.consensus.On("CanStageTournamentResult", mock.Anything).Return(CanStageTournamentResult{}, test.cause).Once()
			}
			output.Reset()
			tickHead(client, 100)
			_, err = f.d.Tick(t.Context())
			if errors.Is(test.cause, bind.ErrNoCode) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.cause)
			}
			require.Equal(t, 1, strings.Count(output.String(), failedRootAlert))
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
			require.Empty(t, f.repo.windows)
			f.repo.AssertExpectations(t)
			f.consensus.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestFailedRootAlertInventoryFailurePreservesErrorAndSkipsObservation(t *testing.T) {
	f, client := tickFixture(t)
	f.epoch.Status = model.EpochStatus_Closed
	root := &model.Tournament{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index,
		Address: *f.epoch.TournamentAddress, StartInstant: f.epoch.LastBlock, Allowance: 90,
		Snapshot: model.TournamentSnapshot{AsOfBlock: 100, FinishedAtBlock: 90,
			Standing:     model.TournamentStandingRootFailed,
			BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionNoWinner}}}
	f.repo.storedTournaments = []*model.Tournament{root}
	failure := errors.New("tournament inventory unavailable")
	// A mistaken observation must reach the no-call assertion below.
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Maybe()
	f.repo.On("ListTournaments", mock.Anything, f.app.Name, repository.TournamentFilter{}, repository.Pagination{}, false).
		Return([]*model.Tournament(nil), uint64(0), failure).Once()
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
		repository.Pagination{}, false).Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(observerFacts(100))).Return(CurrentSealedEpoch{
		EpochNumber: f.epoch.Index, Tournament: root.Address,
		InputIndexLowerBound: f.epoch.InputIndexLowerBound, InputIndexUpperBound: f.epoch.InputIndexUpperBound,
	}, nil).Once()
	var output bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&output, nil))
	beforeApp, beforeEpoch, beforeRoot := *f.app, *f.epoch, *root
	tickHead(client, 100)
	reschedule, err := f.d.Tick(t.Context())
	require.ErrorIs(t, err, failure)
	require.False(t, reschedule)
	require.NotContains(t, output.String(), failedRootAlert)
	require.Empty(t, f.repo.windows)
	require.Empty(t, f.backend.published)
	require.Equal(t, beforeApp, *f.app)
	require.Equal(t, beforeEpoch, *f.epoch)
	require.Equal(t, beforeRoot, *root)
	f.consensus.AssertNotCalled(t, "TournamentLevelCount", mock.Anything)
	f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertExpectations(t)
	f.consensus.AssertExpectations(t)
	f.factory.AssertExpectations(t)
	client.AssertExpectations(t)
}
