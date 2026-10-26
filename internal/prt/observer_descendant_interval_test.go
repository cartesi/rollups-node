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
				Raw: types.Log{BlockNumber: 60, TxHash: common.HexToHash("0xa0"), Index: 0},
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
				State: model.TournamentStandingInnerWinner, HasCandidate: true, Candidate: *epoch.Commitment, FinishedAt: 90,
			}, nil).Once()
			expectTournamentAuxiliaryReads(adapter, opts, TournamentLevel(1), model.TournamentStandingInnerWinner)
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
			MatchIdHash: matchID, Raw: types.Log{BlockNumber: 80},
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
