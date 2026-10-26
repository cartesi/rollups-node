// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prtprotection"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func protectionOpts(facts txsender.BlockRef) any {
	return mock.MatchedBy(func(opts *bind.CallOpts) bool {
		return opts.BlockHash == facts.Hash && opts.BlockNumber == nil && !opts.Pending
	})
}

// Existing observation tests explicitly declare their additional diagnostic
// reads. Separate adapters prevent diagnostic clocks from consuming projection
// expectations. No standing, event, repository, or unknown read is defaulted.
func expectProtectionReads(t *testing.T, factory *adapterFactoryMock, consensus *daveConsensusAdapterMock,
	facts txsender.BlockRef, epochs ...*model.Epoch,
) {
	t.Helper()
	var sealed CurrentSealedEpoch
	if len(epochs) > 0 {
		epoch := epochs[len(epochs)-1]
		sealed.EpochNumber = epoch.Index
		sealed.InputIndexLowerBound, sealed.InputIndexUpperBound = epoch.InputIndexLowerBound, epoch.InputIndexUpperBound
		if epoch.TournamentAddress != nil {
			sealed.Tournament = *epoch.TournamentAddress
		}
	}
	call := consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(sealed, nil).Once()
	consensus.ExpectedCalls = append([]*mock.Call{call}, consensus.ExpectedCalls[:len(consensus.ExpectedCalls)-1]...)
	for _, epoch := range epochs {
		if epoch.TournamentAddress == nil || epoch.LastBlock > facts.Number || epoch.ClaimTransactionHash != nil ||
			!slices.Contains(model.NonTerminalEpochStatuses(), epoch.Status) {
			continue
		}
		adapter := &tournamentAdapterMock{}
		call := factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
		factory.ExpectedCalls = append([]*mock.Call{call}, factory.ExpectedCalls[:len(factory.ExpectedCalls)-1]...)
		adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{
			Level: 0, StartInstant: epoch.LastBlock, Allowance: 300,
		}, nil).Once()
		t.Cleanup(func() { adapter.AssertExpectations(t) })
	}
}

func expectStoredProtection(t *testing.T, repo *prtRepositoryMock, app *model.Application, epochs ...*model.Epoch) {
	t.Helper()
	repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), repository.EpochFilter{
		HasTournament: new(true), Status: model.NonTerminalEpochStatuses(),
	}, repository.Pagination{}, false).Run(func(mock.Arguments) {
		// This query starts after the health transition. Earlier domain reads
		// retain their own expectations and cannot consume these clock reads.
		for _, epoch := range epochs {
			if epoch.TournamentAddress == nil || epoch.ClaimTransactionHash != nil ||
				!slices.Contains(model.NonTerminalEpochStatuses(), epoch.Status) {
				continue
			}
			call := repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(
				&model.Tournament{ApplicationID: app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
					StartInstant: epoch.LastBlock, Allowance: 300}, nil).Once()
			repo.ExpectedCalls = append([]*mock.Call{call}, repo.ExpectedCalls[:len(repo.ExpectedCalls)-1]...)
		}
	}).Return(epochs, uint64(len(epochs)), nil).Once()
}

func TestProtectionIndependentPinnedClocksKeepSiblingAfterFailure(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	first, second := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x101")
	facts := observerFacts(100)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(CurrentSealedEpoch{
		EpochNumber: second.Index, Tournament: *second.TournamentAddress,
	}, nil).Once()
	for _, epoch := range []*model.Epoch{first, second} {
		adapter := &tournamentAdapterMock{}
		f.factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
		if epoch == first {
			adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{}, errors.New("descriptor unavailable")).Once()
		} else {
			adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{StartInstant: 10, Allowance: 250}, nil).Once()
		}
		t.Cleanup(func() { adapter.AssertExpectations(t) })
	}
	f.s.observeProtection(t.Context(), f.app, []*model.Epoch{first, second}, f.consensus, facts, nil)
	require.Equal(t, 2, strings.Count(output.String(), "PRT protection stopped"))
	require.Contains(t, output.String(), "epoch_index=0 root_tournament="+first.TournamentAddress.Hex())
	require.Contains(t, output.String(), "clock_available=false")
	require.Contains(t, output.String(), "epoch_index=1 root_tournament="+second.TournamentAddress.Hex())
	require.Contains(t, output.String(), "join_close_block=260")
	require.True(t, f.s.Ready())
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
	f.consensus.AssertNotCalled(t, "CanStageTournamentResult", mock.Anything)
	f.consensus.AssertNotCalled(t, "CanAcceptStagedTournamentResult", mock.Anything)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestProtectionCurrentSealedFallbackSurvivesDiscoveryFailure(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	f.app.ExecutionParameters.AdvanceMaxDeadline = 2 * time.Second
	facts := observerFacts(100)
	cause := errors.New("projection repository unavailable")
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)}, repository.Pagination{}, false).
		Return([]*model.Epoch(nil), uint64(0), cause).Once()
	root := common.HexToAddress("0x777")
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(CurrentSealedEpoch{
		EpochNumber: 7, Tournament: root, InputIndexLowerBound: 20, InputIndexUpperBound: 23,
	}, nil).Once()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", root).Return(adapter, nil).Once()
	adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{StartInstant: 90, Allowance: 300}, nil).Once()
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	require.ErrorIs(t, f.s.observeApplication(t.Context(), f.app, facts), cause)
	require.Contains(t, output.String(), "epoch_index=7 root_tournament="+root.Hex())
	require.Contains(t, output.String(), "join_close_block=390")
	require.Contains(t, output.String(), "pending_execution_inputs=3")
	require.Contains(t, output.String(), "execution_budget_ns=6000000000")
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
	f.consensus.AssertNotCalled(t, "CanStageTournamentResult", mock.Anything)
	f.consensus.AssertNotCalled(t, "CanAcceptStagedTournamentResult", mock.Anything)
	f.repo.AssertNotCalled(t, "StoreTournamentInterval", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	adapter.AssertExpectations(t)
}

func TestProtectionUnavailableClockRetainsExactIdentity(t *testing.T) {
	for _, test := range []struct {
		name       string
		descriptor TournamentDescriptor
	}{
		{"inner", TournamentDescriptor{Level: 1, StartInstant: 10, Allowance: 50}},
		{"wrong boundary", TournamentDescriptor{StartInstant: 11, Allowance: 50}},
		{"overflow", TournamentDescriptor{StartInstant: 10, Allowance: math.MaxUint64}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			epoch := checkpointEpoch(9, "0x999")
			facts := observerFacts(100)
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
			adapter.On("Descriptor", protectionOpts(facts)).Return(test.descriptor, nil).Once()
			clock := f.s.readProtectionClock(withLiveBlock(t.Context(), facts.Number, facts.Hash), f.app,
				protectionTarget{epoch: epoch, index: epoch.Index, root: *epoch.TournamentAddress}, facts)
			require.Equal(t, epoch.Index, clock.EpochIndex)
			require.Equal(t, *epoch.TournamentAddress, clock.Root)
			require.Nil(t, clock.ClosingBlock)
			require.Error(t, clock.Err)
			adapter.AssertExpectations(t)
		})
	}
}

func TestProtectionExecutionStopKeepsValidPrefix(t *testing.T) {
	for _, status := range []model.ApplicationStatus{model.ApplicationStatus_MachineHalted, model.ApplicationStatus_InvalidOutputsRoot} {
		t.Run(status.String(), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.app.Status = status
			first, second := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x101")
			facts := observerFacts(100)
			expectProtectionReads(t, f.factory, f.consensus, facts, first, second)
			f.repo.On("GetEpochCompletionState", mock.Anything, f.app.IApplicationAddress.Hex(), first.Index).
				Return(&repository.EpochCompletionState{Application: f.app, Epoch: first, Problem: prtTestProblem(status, 1)}, nil).Once()
			var output bytes.Buffer
			f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			f.s.observeProtection(t.Context(), f.app, []*model.Epoch{first, second}, f.consensus, facts, nil)
			require.Equal(t, 1, strings.Count(output.String(), "PRT protection stopped"))
			require.Contains(t, output.String(), "epoch_index=1 root_tournament="+second.TournamentAddress.Hex())
			require.Equal(t, status, f.app.Status)
		})
	}
}

func TestProtectionBacklogExactStoredExecutionBudgets(t *testing.T) {
	for _, test := range []struct {
		name                    string
		lower, upper, processed uint64
		deadline                time.Duration
		pending                 uint64
		budget                  string
	}{
		{"stored deadline", 10, 20, 12, 7 * time.Second, 8, "56000000000"},
		{"previous epoch cursor", 10, 20, 1, time.Second, 10, "10000000000"},
		{"validation remains", 10, 20, 30, time.Second, 0, "0"},
		{"no finite deadline", 10, 20, 12, 0, 8, "unbounded"},
		{"duration overflow", 0, math.MaxUint64, 0, time.Hour, math.MaxUint64, "66408278665354385814000000000000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.app.ProcessedInputs = test.processed
			f.app.ExecutionParameters.AdvanceMaxDeadline = test.deadline
			epoch := checkpointEpoch(2, "0x200")
			epoch.Status = model.EpochStatus_InputsProcessed
			var output bytes.Buffer
			f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			clock := prtprotection.Clock{EpochIndex: 2, Root: *epoch.TournamentAddress, ClosingBlock: new(uint64(100)), Source: "test"}
			target := protectionTarget{epoch, epoch.Index, *epoch.TournamentAddress, test.lower, test.upper}
			f.s.reportSealBacklog(f.app, target, clock, observerFacts(100))
			require.Contains(t, output.String(), "pending_execution_inputs="+new(big.Int).SetUint64(test.pending).String())
			require.Contains(t, output.String(), "execution_budget_ns="+test.budget)
			require.Contains(t, output.String(), "validation_pending=true")
			require.Contains(t, output.String(), "join_window_closed=true")
			require.NotContains(t, output.String(), "eta")
			require.True(t, f.s.Ready())
		})
	}
	_, err := pendingExecutionInputs(20, 10, 0)
	require.Error(t, err)
}

func TestProtectionGlobalStopKeepsSiblingApplications(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	second := *f.app
	second.ID++
	second.Name = "second"
	second.IApplicationAddress = common.HexToAddress("0x999")
	filter := repository.ApplicationFilter{ConsensusType: new(model.Consensus_PRT), Enabled: new(true)}
	f.repo.On("ListApplicationsByID", mock.Anything, filter, int64(0), second.ID, deliveryApplicationPageSize).
		Return([]*model.Application{f.app, &second}, nil).Once()
	query := repository.EpochFilter{HasTournament: new(true), Status: model.NonTerminalEpochStatuses()}
	cause := errors.New("hard publication failure")
	f.s.stopPublication(cause)
	f.repo.On("ListEpochs", mock.Anything, f.app.IApplicationAddress.Hex(), query, repository.Pagination{}, false).
		Run(func(mock.Arguments) { require.True(t, f.s.publicationStopped()) }).
		Return([]*model.Epoch(nil), uint64(0), errors.New("first application unavailable")).Once()
	epoch := checkpointEpoch(5, "0x555")
	epoch.ApplicationID = second.ID
	f.repo.On("ListEpochs", mock.Anything, second.IApplicationAddress.Hex(), query, repository.Pagination{}, false).
		Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	f.repo.On("GetTournament", mock.Anything, second.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(&model.Tournament{ApplicationID: second.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
			StartInstant: epoch.LastBlock, Allowance: 300}, nil).Once()
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	f.s.reportGlobalProtectionStop(t.Context(), second.ID, cause)
	require.Contains(t, output.String(), "PRT protection deadline discovery unavailable")
	require.Contains(t, output.String(), "application=second")
	require.Contains(t, output.String(), "join_close_block=310")
	require.True(t, f.s.publicationStopped())
	require.ErrorIs(t, f.s.publicationFailure(), cause)
}

func TestProtectionQuietShutdownDoesNotRead(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f.s.observeProtection(ctx, f.app, []*model.Epoch{checkpointEpoch(0, "0x100")}, f.consensus, observerFacts(100), nil)
	f.consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.Anything)
	f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
}

func TestProtectionCurrentSealedIdentitySurvivesWrongLocalBoundary(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	epoch := checkpointEpoch(3, "0x333")
	epoch.LastBlock = 101
	facts := observerFacts(100)
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(CurrentSealedEpoch{
		EpochNumber: epoch.Index, Tournament: *epoch.TournamentAddress,
	}, nil).Once()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
	adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{StartInstant: 90, Allowance: 300}, nil).Once()
	clocks := f.s.observeProtection(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, facts, nil)
	require.Len(t, clocks, 1)
	require.Equal(t, epoch.Index, clocks[0].EpochIndex)
	require.Equal(t, *epoch.TournamentAddress, clocks[0].Root)
	require.Nil(t, clocks[0].ClosingBlock)
	require.ErrorContains(t, clocks[0].Err, "does not match sealed epoch")
	adapter.AssertExpectations(t)
}

func TestProtectionObservedAcceptanceRetiresClockWithoutReceiptRead(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.Status = model.ApplicationStatus_Failed
	epoch := checkpointEpoch(3, "0x333")
	epoch.ClaimTransactionHash = new(common.HexToHash("0x123"))
	facts := observerFacts(100)
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(CurrentSealedEpoch{
		EpochNumber: epoch.Index, Tournament: *epoch.TournamentAddress,
	}, nil).Once()
	clocks := f.s.observeProtection(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, facts, nil)
	require.Empty(t, clocks)
	f.factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
	f.client.AssertNotCalled(t, "TransactionReceipt", mock.Anything, mock.Anything)
}
