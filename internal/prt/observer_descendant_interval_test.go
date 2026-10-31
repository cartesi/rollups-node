// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewDescendantFirstScanCommitsWithItsDiscoveringInterval(t *testing.T) {
	for _, fault := range []string{"none", "child read", "transaction"} {
		t.Run(fault, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			epoch := checkpointEpoch(0, "0x100")
			child := common.HexToAddress("0x200")
			rootEvents := descendantParticipantEvents(51, false)
			rootEvents.NewInnerTournament = []*itournament.ITournamentNewInnerTournament{{
				MatchIdHash: rootEvents.MatchCreated[0].MatchIdHash, ChildTournament: child,
				One: rootEvents.MatchCreated[0].One, Two: rootEvents.MatchCreated[0].Two,
				Descriptor: itournament.ITournamentTournamentDescriptor{BaseCycle: big.NewInt(0), Level: 1, Kind: 0, StartInstant: 60},
				BondValue:  big.NewInt(1),
				Raw:        types.Log{Address: *epoch.TournamentAddress, BlockNumber: 60, TxHash: common.HexToHash("0xa0"), Index: 0},
			}}
			rootEvents.PartialBondRefund = []*itournament.ITournamentPartialBondRefund{{
				Value: big.NewInt(0), Raw: types.Log{BlockNumber: 60, TxHash: common.HexToHash("0xa0"), Index: 1},
			}}
			root := f.tournament(epoch, *epoch.TournamentAddress, 0, 2, 100, 100, rootEvents, nil)
			// The fixture created this adapter immediately before returning root.
			rootAdapter := f.factory.ExpectedCalls[len(f.factory.ExpectedCalls)-1].ReturnArguments.Get(0).(*tournamentAdapterMock)
			expectDescendantParticipantViews(rootAdapter, rootEvents)
			f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), child.Hex()).
				Return((*model.Tournament)(nil), nil).Twice()

			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", child).Return(adapter, nil).Once()
			opts := mock.MatchedBy(resultCallOptsAtBlock(100))
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{
				BaseCycle: big.NewInt(0), Level: 1, Kind: model.TournamentKindLeaf, StartInstant: 60,
			}, nil).Once()
			adapter.On("Standing", opts).Return(TournamentStanding{
				State: model.TournamentStandingMatchesActive,
			}, nil).Once()
			expectTournamentAuxiliaryReads(adapter, opts, TournamentLevel(1), model.TournamentStandingMatchesActive)
			adapter.On("BondValue", opts).Return(big.NewInt(1), nil).Once()
			childEvents := descendantParticipantEvents(61, true)
			failure := errors.New("descendant interval unavailable")
			var readErr error
			if fault == "child read" {
				readErr = failure
			}
			adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
				return opts.Start == 60 && opts.End != nil && *opts.End == 100
			})).Return(childEvents, readErr).Once()
			if readErr == nil {
				adapter.On("StructuralEventCounts", opts).Return(structuralCountsForEvents(childEvents), nil).Once()
				f.emptyParticipants(epoch, child)
				expectDescendantParticipantViews(adapter, childEvents)
				var storeErr error
				if fault == "transaction" {
					storeErr = failure
				}
				f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID,
					mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
						if len(batches) != 2 {
							return false
						}
						parent, descendant := batches[0].Tournament, batches[1].Tournament
						return parent.Address == root.Address && parent.Snapshot.AsOfBlock == 100 &&
							descendant.Address == child && descendant.Snapshot.AsOfBlock == 100 &&
							descendant.ParentTournamentAddress != nil && *descendant.ParentTournamentAddress == root.Address &&
							descendant.CreationEvent != nil && descendant.CreationEvent.BlockNumber == 60
					}), uint64(100)).Return(storeErr).Once()
			}

			err := f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100))
			if fault == "none" {
				require.NoError(t, err)
				require.Equal(t, uint64(100), f.repo.committedTournaments[root.Address].Snapshot.AsOfBlock)
				require.Equal(t, uint64(100), f.repo.committedTournaments[child].Snapshot.AsOfBlock)
			} else {
				require.ErrorIs(t, err, failure)
				require.Empty(t, f.repo.committedTournaments, "a failed descendant never publishes a parent prefix")
				require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
			}
			require.Equal(t, uint64(50), root.Snapshot.AsOfBlock, "the stored input row remains unchanged")
			adapter.AssertExpectations(t)
		})
	}
}

func descendantParticipantEvents(start uint64, leaf bool) *TournamentEvents {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	matchID := common.HexToHash("0x33")
	events := &TournamentEvents{
		CommitmentJoined: []*itournament.ITournamentCommitmentJoined{
			{Commitment: one, FinalStateHash: one, Raw: types.Log{BlockNumber: start}},
			{Commitment: two, FinalStateHash: two, Raw: types.Log{BlockNumber: start + 1}},
		},
		MatchCreated: []*itournament.ITournamentMatchCreated{{
			MatchIdHash: matchID, One: one, Two: two, Raw: types.Log{BlockNumber: start + 4},
		}},
	}
	events.StandingChanged = []*itournament.ITournamentStandingChanged{
		{MatchCount: new(big.Int), Dangling: one, ResultAt: 100, Raw: types.Log{BlockNumber: start, Index: 1}},
		{MatchCount: big.NewInt(1), Raw: types.Log{BlockNumber: start + 1, Index: 1}},
	}
	if leaf {
		events.LeafMatchSealed = []*itournament.ITournamentLeafMatchSealed{{
			MatchIdHash: matchID, One: one, Two: two, DivergencePosition: big.NewInt(0), Raw: types.Log{BlockNumber: 80},
		}}
		events.PartialBondRefund = []*itournament.ITournamentPartialBondRefund{{Value: big.NewInt(0), Raw: types.Log{BlockNumber: 80}}}
	}
	return events
}

func expectDescendantParticipantViews(adapter *tournamentAdapterMock, events *TournamentEvents) {
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	for _, event := range events.CommitmentJoined {
		adapter.On("CommitmentStanding", opts, event.Commitment).
			Return(CommitmentStanding{Joined: true, FinalState: event.FinalStateHash}, nil).Once()
	}
	match := events.MatchCreated[0]
	adapter.On("MatchSnapshot", opts, match.One, match.Two).Return(ObservedMatchSnapshot{
		Phase: model.MatchPhaseSealed, Sealed: &SealedMatch{DivergencePosition: big.NewInt(0), DivergenceCycle: big.NewInt(0)},
	}, nil).Once()
}

func TestFinishedNoWinnerTreeKeepsHistoricalChildAfterParentDeletion(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0xa0")
	epoch.Status = model.EpochStatus_Closed
	events := descendantParticipantEvents(51, false)
	pair := events.MatchCreated[0]
	childAddress := common.HexToAddress("0xa1")
	events.NewInnerTournament = []*itournament.ITournamentNewInnerTournament{{MatchIdHash: pair.MatchIdHash, One: pair.One, Two: pair.Two,
		ChildTournament: childAddress, ContestedFinalStateTwo: common.HexToHash("0x77"), BondValue: big.NewInt(1),
		Descriptor: itournament.ITournamentTournamentDescriptor{BaseCycle: big.NewInt(0), Level: 1, Kind: 0, Height: 68, StartInstant: 60,
			Allowance: 20},
		Raw: types.Log{Address: *epoch.TournamentAddress, BlockNumber: 60, TxHash: common.HexToHash("0x66"), Index: 1}}}
	events.MatchDeleted = []*itournament.ITournamentMatchDeleted{{MatchIdHash: pair.MatchIdHash, One: pair.One, Two: pair.Two,
		Reason: 2, WinnerCommitment: 0,
		Raw: types.Log{BlockNumber: 90, TxHash: common.HexToHash("0x88"), Index: 2}}}
	events.StandingChanged = append(events.StandingChanged, &itournament.ITournamentStandingChanged{MatchCount: big.NewInt(0), ResultAt: 90,
		Raw: types.Log{BlockNumber: 90, TxHash: common.HexToHash("0x88"), Index: 3}})
	events.PartialBondRefund = []*itournament.ITournamentPartialBondRefund{{Value: big.NewInt(0)}, {Value: big.NewInt(0)}}
	root := f.tournament(epoch, *epoch.TournamentAddress, 0, 2, 90, 100, events, nil)
	rootAdapter := f.factory.ExpectedCalls[len(f.factory.ExpectedCalls)-1].ReturnArguments.Get(0).(*tournamentAdapterMock)
	for _, call := range rootAdapter.ExpectedCalls {
		switch call.Method {
		case "Standing":
			call.Return(TournamentStanding{State: model.TournamentStandingRootFailed, FinishedAt: 90}, nil)
		case "BondRecovery":
			call.Return(canonicalBondRecovery(model.BondDispositionNoWinner, common.Address{}, 0), nil)
		}
	}
	for _, joined := range events.CommitmentJoined {
		rootAdapter.On("CommitmentStanding", mock.Anything, joined.Commitment).Return(CommitmentStanding{Joined: true,
			FinalState: joined.FinalStateHash}, nil).Once()
	}
	rootAdapter.On("MatchSnapshot", mock.Anything, pair.One,
		pair.Two).Return(ObservedMatchSnapshot{Phase: model.MatchPhaseUninitialized, TimeoutOutcome: model.MatchTimeoutNone}, nil).Once()
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(2), nil).Once()
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), childAddress.Hex()).Return((*model.Tournament)(nil),
		nil).Twice()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", childAddress).Return(adapter, nil).Once()
	adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{BaseCycle: big.NewInt(0), Level: 1,
		Kind: model.TournamentKindLeaf, Height: 68, StartInstant: 60, Allowance: 20}, nil).Once()
	adapter.On("BondValue", mock.Anything).Return(big.NewInt(1), nil).Once()
	adapter.On("Standing", mock.Anything).Return(TournamentStanding{State: model.TournamentStandingInnerEliminableNoWinner,
		FinishedAt: 80}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, mock.Anything, 1, model.TournamentStandingInnerEliminableNoWinner)
	adapter.On("RetrieveAllEvents", mock.Anything).Return(&TournamentEvents{}, nil).Once()
	adapter.On("StructuralEventCounts", mock.Anything).Return(zeroStructuralEventCounts(), nil).Once()
	f.emptyParticipants(epoch, childAddress)
	f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Run(func(args mock.Arguments) {
		batches := args.Get(2).([]*repository.TournamentEventBatch)
		require.Len(t, batches, 2)
		require.Equal(t, model.TournamentStandingRootFailed, batches[0].Tournament.Snapshot.Standing)
		require.Nil(t, batches[0].Tournament.Snapshot.Candidate)
		require.Equal(t, model.MatchPhaseUninitialized, batches[0].Matches[0].Snapshot.Phase)
		child := batches[1].Tournament
		require.NotNil(t, child.CreationEvent)
		require.NotNil(t, child.ContestedFinalStateOne)
		require.Zero(t, *child.ContestedFinalStateOne)
		require.Equal(t, common.HexToHash("0x77"), *child.ContestedFinalStateTwo)
		require.Equal(t, model.TournamentStandingInnerEliminableNoWinner, child.Snapshot.Standing)
		require.Nil(t, child.Snapshot.Candidate)
	}).Return(nil).Once()
	require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, []*model.Epoch{epoch}, f.consensus, observerFacts(100)))
	require.Equal(t, uint64(100), f.repo.committedTournaments[root.Address].Snapshot.AsOfBlock)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	adapter.AssertExpectations(t)
}
