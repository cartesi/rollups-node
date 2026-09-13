// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type observerCheckpointFixture struct {
	t         *testing.T
	s         *Service
	repo      *prtRepositoryMock
	factory   *adapterFactoryMock
	consensus *daveConsensusAdapterMock
	client    *ethClientMock
	app       *Application
}

func newObserverCheckpointFixture(t *testing.T) *observerCheckpointFixture {
	t.Helper()
	s, repo := newPRTServiceMock()
	f := &observerCheckpointFixture{t: t, s: s, repo: repo, factory: &adapterFactoryMock{},
		consensus: &daveConsensusAdapterMock{}, client: &ethClientMock{}, app: prtRevertTestApp()}
	f.app.LastTournamentCheckBlock = 50
	f.app.LastEpochCheckBlock = 100
	s.adapterFactory, s.client = f.factory, f.client
	f.factory.On("CreateDaveConsensusAdapter", f.app.IConsensusAddress).Return(f.consensus, nil)
	t.Cleanup(func() {
		f.repo.AssertExpectations(t)
		f.factory.AssertExpectations(t)
		f.consensus.AssertExpectations(t)
		f.client.AssertExpectations(t)
	})
	return f
}

func (f *observerCheckpointFixture) epochs(epochs ...*Epoch) {
	f.t.Helper()
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
		repository.Pagination{}, false).Return(epochs, uint64(len(epochs)), nil).Once()
}

func checkpointEpoch(index uint64, address string) *Epoch {
	epoch := resultTestEpoch(EpochStatus_ClaimComputed)
	epoch.Index = index
	epoch.LastBlock = 10
	epoch.TournamentAddress = new(common.HexToAddress(address))
	return epoch
}

func (f *observerCheckpointFixture) tournament(
	epoch *Epoch, address common.Address, level, levels, finish, scanEnd uint64, events *TournamentEvents, scanError error,
) *Tournament {
	f.t.Helper()
	tournament := &Tournament{ApplicationID: f.app.ID, EpochIndex: epoch.Index, Address: address,
		Level: level, MaxLevel: levels, StartInstant: epoch.LastBlock, Snapshot: TournamentSnapshot{FinishedAtBlock: finish}}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(tournament, nil).Once()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	kind := TournamentKindNonLeaf
	if level+1 == levels {
		kind = TournamentKindLeaf
	}
	tournament.Kind = kind
	adapter.On("Descriptor", mock.MatchedBy(resultCallOptsAtBlock(scanEnd))).
		Return(TournamentDescriptor{BaseCycle: big.NewInt(0), Level: level, Kind: kind, StartInstant: epoch.LastBlock}, nil).Once()
	candidate := common.HexToHash("0x777")
	if epoch.Commitment != nil {
		candidate = *epoch.Commitment
	}
	state := TournamentStandingRootWinner
	if level != uint64(RootLevel) {
		state = TournamentStandingInnerWinner
	}
	finishedAt := finish
	if finishedAt == 0 {
		finishedAt = scanEnd
	}
	adapter.On("Standing", mock.MatchedBy(resultCallOptsAtBlock(scanEnd))).Return(TournamentStanding{
		State: state, HasCandidate: true, Candidate: candidate, FinishedAt: finishedAt,
	}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, mock.MatchedBy(resultCallOptsAtBlock(scanEnd)), TournamentLevel(level), state)
	end := scanEnd
	start := max(epoch.LastBlock, f.app.LastTournamentCheckBlock+1)
	if start <= end {
		adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
			return opts != nil && opts.Context != nil && opts.Start == start && opts.End != nil && *opts.End == end
		})).Return(events, scanError).Once()
	}
	if scanError == nil {
		if start > tournament.StartInstant {
			adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(start-1))).
				Return(zeroStructuralEventCounts(), nil).Once()
			adapter.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(start-1))).
				Return(canonicalBondRecovery(BondDispositionTournamentRunning, common.Address{}, 0), nil).Once()
		}
		adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(end))).
			Return(structuralCountsForEvents(events), nil).Once()
		f.emptyParticipants(epoch, address)
	}
	f.t.Cleanup(func() { adapter.AssertExpectations(f.t) })
	return tournament
}

func expectTournamentAuxiliaryReads(adapter *tournamentAdapterMock, opts any, level TournamentLevel, state TournamentStandingState) {
	disposition := BondDispositionTournamentRunning
	if isTerminalTournamentStanding(state) {
		disposition = BondDispositionRecoverable
		if state == TournamentStandingRootFailed || state == TournamentStandingInnerEliminableNoWinner {
			disposition = BondDispositionNoWinner
		}
	}
	if level != RootLevel {
		result := InnerResult{Disposition: InnerTournamentUnsettled}
		switch state {
		case TournamentStandingInnerWinner:
			result = InnerResult{Disposition: InnerTournamentWinner, PausedAllowance: 1}
		case TournamentStandingInnerEliminableWinnerExpired, TournamentStandingInnerEliminableNoWinner:
			result.Disposition = InnerTournamentEliminable
		case TournamentStandingMatchesActive, TournamentStandingAwaitingClosure,
			TournamentStandingRootWinner, TournamentStandingRootFailed:
		}
		adapter.On("InnerResult", opts).Return(result, nil).Once()
	}
	adapter.On("BondRecovery", opts).Return(canonicalBondRecovery(disposition, common.HexToAddress("0x777"), 0), nil).Once()
}

func (f *observerCheckpointFixture) emptyParticipants(epoch *Epoch, address common.Address) {
	f.t.Helper()
	filterAddress := address.Hex()
	f.repo.On("ListCommitments", mock.Anything, f.app.Name,
		repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &filterAddress}, repository.Pagination{}, false).
		Return([]*Commitment{}, uint64(0), nil).Once()
	f.repo.On("ListMatches", mock.Anything, f.app.Name,
		repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &filterAddress}, repository.Pagination{}, false).
		Return([]*Match{}, uint64(0), nil).Once()
}

func (f *observerCheckpointFixture) children(epoch *Epoch, parent *Tournament, children ...*Tournament) {
	f.t.Helper()
	for _, child := range children {
		child.ParentTournamentAddress = &parent.Address
		child.ParentMatchIDHash = new(common.HexToHash("0xabc"))
	}
	f.repo.On("ListTournaments", mock.Anything, f.app.Name, repository.TournamentFilter{
		EpochIndex: &epoch.Index, ParentTournamentAddress: &parent.Address, Level: new(parent.Level + 1),
	}, repository.Pagination{}, false).Return(children, uint64(len(children)), nil).Once()
}

func (f *observerCheckpointFixture) acceptance(epoch *Epoch, tournament *Tournament, writeError error) {
	f.t.Helper()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(tournament, nil).Once()
	log := types.Log{Address: f.app.IConsensusAddress, TxHash: *epoch.ClaimTransactionHash, BlockNumber: 100}
	receipt := &types.Receipt{TxHash: *epoch.ClaimTransactionHash, BlockNumber: big.NewInt(100),
		Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{&log}}
	f.client.On("TransactionReceipt", mock.Anything, *epoch.ClaimTransactionHash).Return(receipt, nil).Once()
	f.consensus.On("ParseEpochSealed", log).Return(&idaveconsensus.IDaveConsensusEpochSealed{
		EpochNumber: new(big.Int).SetUint64(epoch.Index + 1), InitialMachineStateHash: *epoch.MachineHash,
		OutputsMerkleRoot: *epoch.TxBufferDataBlock, Raw: log,
	}, nil).Once()
	f.repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, f.app.ID, epoch.Index, epoch.ClaimTransactionHash).
		Run(func(mock.Arguments) { require.Equal(f.t, uint64(100), f.app.LastTournamentCheckBlock) }).Return(writeError).Once()
}

func (f *observerCheckpointFixture) unaccepted(epoch *Epoch, finish uint64) {
	f.t.Helper()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(
		&Tournament{Address: *epoch.TournamentAddress, Snapshot: TournamentSnapshot{
			FinishedAtBlock: finish, WinnerCommitment: epoch.Commitment,
		}}, nil).Once()
}

func TestObserverCheckpointCommitsAllRootsAndChildren(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	first, second := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x200")
	first.ClaimTransactionHash = new(common.HexToHash("0x900"))
	f.epochs(first, second)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
	childAddress := common.HexToAddress("0x101")
	childEpoch := *first
	childEpoch.LastBlock = 60 // The child is created inside the parent scan window, 51..100.
	parentEvents := &TournamentEvents{NewInnerTournament: []*itournament.ITournamentNewInnerTournament{{
		ChildTournament: childAddress, MatchIdHash: common.HexToHash("0xabc"),
		Raw: types.Log{BlockNumber: childEpoch.LastBlock},
	}}, PartialBondRefund: []*itournament.ITournamentPartialBondRefund{{Value: new(big.Int)}}}
	parent := f.tournament(first, *first.TournamentAddress, 0, 2, 90, 100, parentEvents, nil)
	child := f.tournament(&childEpoch, childAddress, 1, 2, 88, 100, &TournamentEvents{}, nil)
	f.children(first, parent, child)
	secondRoot := f.tournament(second, *second.TournamentAddress, 0, 2, 95, 100, &TournamentEvents{}, nil)
	lastChild := f.tournament(second, common.HexToAddress("0x201"), 1, 2, 89, 100, &TournamentEvents{}, nil)
	f.children(second, secondRoot, lastChild)
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
		mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
			if len(batches) != 4 {
				return false
			}
			for i, expected := range []*Tournament{parent, child, secondRoot, lastChild} {
				if batches[i].Tournament.Address != expected.Address {
					return false
				}
			}
			return true
		}), uint64(100)).Return(nil).Once()
	f.acceptance(first, parent, nil)
	f.unaccepted(second, 95)
	deferActions, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.NoError(t, err)
	require.False(t, deferActions)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
}

func TestObserverCheckpointKeepsFinishedSubtreesCurrent(t *testing.T) {
	for _, finish := range []uint64{0, 49, 50, 51} {
		t.Run(new(big.Int).SetUint64(finish).String(), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			var output bytes.Buffer
			f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			epoch := checkpointEpoch(0, "0x100")
			f.epochs(epoch)
			f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
			parent := f.tournament(epoch, *epoch.TournamentAddress, 0, 2, finish, 100, &TournamentEvents{}, nil)
			child := f.tournament(epoch, common.HexToAddress("0x101"), 1, 2, finish, 100, &TournamentEvents{}, nil)
			f.children(epoch, parent, child)
			f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
				mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
					return len(batches) == 2
				}), uint64(100)).Return(nil).Once()
			f.unaccepted(epoch, finish)
			_, err := f.s.checkEpochs(t.Context(), f.app, 100)
			require.NoError(t, err)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
			finishedLogs := strings.Count(output.String(), "Found finished tournament")
			if finish == 0 {
				require.Equal(t, 2, finishedLogs, "each newly finished clone reports its transition")
			} else {
				require.Zero(t, finishedLogs, "a current refresh must not repeat the finish announcement")
			}
		})
	}
}

func TestGatherTournamentDataKeepsNewFinishedProjectionBelowCursor(t *testing.T) {
	// This checks the gather helper's contract. Normal atomic publication does
	// not advance a cursor without the corresponding tournament projection.
	s, repo := newPRTServiceMock()
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 50
	epoch := checkpointEpoch(0, "0x100")
	address := *epoch.TournamentAddress
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), address.Hex()).
		Return((*Tournament)(nil), nil).Once()
	adapter := &tournamentAdapterMock{}
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{
		BaseCycle: big.NewInt(0), Kind: TournamentKindLeaf, StartInstant: epoch.LastBlock,
	}, nil).Once()
	adapter.On("Standing", opts).Return(TournamentStanding{
		State: TournamentStandingRootWinner, HasCandidate: true, Candidate: *epoch.Commitment,
		FinalState: *epoch.MachineHash, FinishedAt: 49,
	}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, opts, RootLevel, TournamentStandingRootWinner)
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == epoch.LastBlock && opts.End != nil && *opts.End == 100
	})).Return(&TournamentEvents{}, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
	f := &observerCheckpointFixture{t: t, s: s, repo: repo, app: app}
	f.emptyParticipants(epoch, address)
	factory := &adapterFactoryMock{}
	factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	s.adapterFactory = factory

	batches, err := s.gatherTournamentData(t.Context(), app, epoch, RootLevel, nil, nil, address, 1, 100)
	require.NoError(t, err)
	require.Len(t, batches, 1, "a new projection must reach the caller even when no event scan is needed")
	require.Equal(t, address, batches[0].Tournament.Address)
	require.Equal(t, uint64(49), batches[0].Tournament.Snapshot.FinishedAtBlock)
	require.Equal(t, epoch.Commitment, batches[0].Tournament.Snapshot.WinnerCommitment)
	require.Equal(t, epoch.MachineHash, batches[0].Tournament.Snapshot.FinalStateHash)
	require.Equal(t, uint64(50), app.LastTournamentCheckBlock)
	require.Len(t, repo.Calls, 3, "gather reads projections but must not publish them or their cursor")
	repo.AssertExpectations(t)
	adapter.AssertExpectations(t)
	factory.AssertExpectations(t)
}

func TestObserverCheckpointRetriesWholeWindowAfterFailure(t *testing.T) {
	const transactionFailure = "transaction"
	for _, fault := range []string{"child RPC", "last root RPC", transactionFailure} {
		t.Run(fault, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			first, second := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x200")
			failure := errors.New(fault)
			for attempt := range 2 {
				f.epochs(first, second)
				f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
				parent := f.tournament(first, *first.TournamentAddress, 0, 2, 0, 100, &TournamentEvents{}, nil)
				var childError, rootError, writeError error
				if attempt == 0 {
					switch fault {
					case "child RPC":
						childError = failure
					case "last root RPC":
						rootError = failure
					case transactionFailure:
						writeError = failure
					}
				}
				child := f.tournament(first, common.HexToAddress("0x101"), 1, 2, 90, 100, &TournamentEvents{}, childError)
				f.children(first, parent, child)
				if childError == nil {
					lastRoot := f.tournament(second, *second.TournamentAddress, 0, 2, 95, 100, &TournamentEvents{}, rootError)
					if rootError == nil {
						f.children(second, lastRoot)
						f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).
							Return(writeError).Once()
					}
				}
				if attempt == 1 {
					f.unaccepted(first, 100)
				}
				deferActions, err := f.s.checkEpochs(t.Context(), f.app, 100)
				require.Zero(t, parent.Snapshot.FinishedAtBlock, "gather must not mutate repository-owned projections")
				if attempt == 0 {
					require.ErrorIs(t, err, failure)
					require.True(t, deferActions)
					require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
					if fault != transactionFailure {
						f.repo.AssertNotCalled(t, "StoreTournamentEvents", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
					}
				} else {
					require.NoError(t, err)
					require.False(t, deferActions)
					require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
				}
			}
		})
	}
}

func TestObserverCheckpointObservesUnpreparedRootsWithinIngestionFloor(t *testing.T) {
	for _, status := range []EpochStatus{EpochStatus_Closed, EpochStatus_InputsProcessed} {
		t.Run(status.String(), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			ready, unprepared := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x200")
			unprepared.Status, unprepared.LastBlock = status, 80
			unprepared.Commitment, unprepared.MachineHash, unprepared.TxBufferDataBlock = nil, nil, nil
			f.epochs(ready, unprepared)
			f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
			f.tournament(ready, *ready.TournamentAddress, 0, 1, 0, 100, &TournamentEvents{}, nil)
			f.tournament(unprepared, *unprepared.TournamentAddress, 0, 1, 0, 100, &TournamentEvents{}, nil)
			f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
			f.unaccepted(ready, 100)
			_, err := f.s.checkEpochs(t.Context(), f.app, 120)
			require.NoError(t, err)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
		})
	}
	t.Run("ingestion head limits prepared roots", func(t *testing.T) {
		f := newObserverCheckpointFixture(t)
		epoch := checkpointEpoch(0, "0x100")
		f.epochs(epoch)
		f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
		f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 0, 100, &TournamentEvents{}, nil)
		f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
		f.unaccepted(epoch, 100)
		_, err := f.s.checkEpochs(t.Context(), f.app, 120)
		require.NoError(t, err)
		require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	})
}

func TestObserverCheckpointRetriesAcceptanceWithoutNewWindow(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	epoch.ClaimTransactionHash = new(common.HexToHash("0x900"))
	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
	tournament := f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
	failure := errors.New("acceptance write failed")
	f.acceptance(epoch, tournament, failure)
	deferActions, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.ErrorIs(t, err, failure)
	require.True(t, deferActions)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	f.epochs(epoch)
	f.acceptance(epoch, tournament, nil)
	deferActions, err = f.s.checkEpochs(t.Context(), f.app, 100)
	require.NoError(t, err)
	require.False(t, deferActions)
	f.repo.AssertNumberOfCalls(t, "StoreTournamentEvents", 1)
	f.consensus.AssertNumberOfCalls(t, "TournamentLevelCount", 1)
}

func TestObserverCheckpointIncludesUnpreparedCreationBlock(t *testing.T) {
	for _, creation := range []uint64{0, 80} {
		t.Run(new(big.Int).SetUint64(creation).String(), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.app.LastTournamentCheckBlock = 0
			ready, unprepared := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x200")
			ready.LastBlock = creation
			unprepared.Status, unprepared.LastBlock = EpochStatus_Closed, creation
			unprepared.Commitment, unprepared.MachineHash, unprepared.TxBufferDataBlock = nil, nil, nil
			f.epochs(ready, unprepared)
			f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
			f.tournament(ready, *ready.TournamentAddress, 0, 1, 100, 100, &TournamentEvents{}, nil)
			f.tournament(unprepared, *unprepared.TournamentAddress, 0, 1, 100, 100, &TournamentEvents{}, nil)
			f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
				mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool { return len(batches) == 2 }), uint64(100)).
				Return(nil).Once()
			f.unaccepted(ready, 100)
			_, err := f.s.checkEpochs(t.Context(), f.app, 100)
			require.NoError(t, err)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
		})
	}
}

func TestObserverCheckpointDoesNotMoveBackward(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.app.LastTournamentCheckBlock = 100
	f.epochs(checkpointEpoch(0, "0x100"))
	_, err := f.s.checkEpochs(t.Context(), f.app, 90)
	require.NoError(t, err)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	f.repo.AssertNotCalled(t, "StoreTournamentEvents", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.consensus.AssertNotCalled(t, "TournamentLevelCount", mock.Anything)
}
