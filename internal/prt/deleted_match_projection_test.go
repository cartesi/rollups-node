// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func certifiedDeletedMatch() *model.Match {
	return &model.Match{
		IDHash: common.HexToHash("0xabc"), TournamentAddress: common.HexToAddress("0x100"),
		CommitmentOne: common.HexToHash("0x11"), CommitmentTwo: common.HexToHash("0x22"),
		LeftOfTwo: common.HexToHash("0x33"), BlockNumber: 20, TxHash: common.HexToHash("0x44"),
		LogIndex: 2, EliminableAt: 30, Winner: model.WinnerCommitment_ONE,
		DeletionReason: model.MatchDeletionReason_TIMEOUT, DeletionBlockNumber: 35,
		DeletionTxHash: new(common.HexToHash("0x55")), DeletionLogIndex: new(uint64(3)),
		Snapshot:  model.MatchSnapshot{AsOfBlock: 40, Phase: model.MatchPhaseUninitialized, TimeoutOutcome: model.MatchTimeoutNone},
		CreatedAt: time.Unix(20, 0), UpdatedAt: time.Unix(40, 0),
	}
}

func matchDeletionEvent(match *model.Match) *itournament.ITournamentMatchDeleted {
	return &itournament.ITournamentMatchDeleted{
		MatchIdHash: match.IDHash, One: match.CommitmentOne, Two: match.CommitmentTwo, Reason: 1, WinnerCommitment: 1,
		Raw: types.Log{BlockNumber: match.DeletionBlockNumber, TxHash: *match.DeletionTxHash, Index: uint(*match.DeletionLogIndex)},
	}
}

func TestTournamentEventBatchReusesCertifiedDeletionWithoutReadOrWrite(t *testing.T) {
	stored := certifiedDeletedMatch()
	before := *stored
	commitment := &model.Commitment{Commitment: stored.CommitmentOne, FinalStateHash: common.HexToHash("0x66")}
	f := newEventBatchFixture(t, []*model.Commitment{commitment}, []*model.Match{stored})
	f.app.LastTournamentCheckBlock = 50
	f.adapter.On("CommitmentStanding", mock.MatchedBy(resultCallOptsAtBlock(100)), [32]byte(commitment.Commitment)).
		Return(CommitmentStanding{Joined: true, FinalState: commitment.FinalStateHash, ClockAllowance: 7}, nil).Once()

	batch, err := f.project(&TournamentEvents{})
	require.NoError(t, err)
	require.Empty(t, batch.Matches, "the unchanged match must not reach the repository write batch")
	require.Equal(t, before, *stored, "keep the certified block, deletion facts, and timestamps")
	require.Len(t, batch.Commitments, 1, "a deleted match does not make its commitments immutable")
	require.Equal(t, uint64(100), batch.Commitments[0].Snapshot.AsOfBlock)
	f.adapter.AssertNotCalled(t, "MatchSnapshot", mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentEventBatchReusesDeletionAtPublishedHead(t *testing.T) {
	stored := certifiedDeletedMatch()
	stored.DeletionBlockNumber, stored.Snapshot.AsOfBlock = 100, 100
	before := *stored
	f := newEventBatchFixture(t, nil, []*model.Match{stored})
	f.app.LastTournamentCheckBlock = 100

	batch, err := f.project(&TournamentEvents{})
	require.NoError(t, err)
	require.Empty(t, batch.Matches, "deletion, snapshot, cursor, and head can have the same block")
	require.Equal(t, before, *stored)
	f.adapter.AssertNotCalled(t, "MatchSnapshot", mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentEventBatchValidatesFirstDeletionAtPinnedBlock(t *testing.T) {
	for _, test := range []struct {
		name string
		view ObservedMatchSnapshot
		fail bool
	}{
		{name: "deleted view", view: ObservedMatchSnapshot{
			Phase: model.MatchPhaseUninitialized, TimeoutOutcome: model.MatchTimeoutNone,
		}},
		{name: "live view contradicts deletion", view: ObservedMatchSnapshot{Phase: model.MatchPhaseBisecting}, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stored := certifiedDeletedMatch()
			event := matchDeletionEvent(stored)
			event.Raw.BlockNumber = 100
			stored.DeletionReason, stored.Winner = model.MatchDeletionReason_NOT_DELETED, model.WinnerCommitment_NONE
			stored.DeletionBlockNumber, stored.DeletionTxHash, stored.DeletionLogIndex = 0, nil, nil
			stored.Snapshot = model.MatchSnapshot{AsOfBlock: 40, Phase: model.MatchPhaseSealed,
				Sealed: &model.MatchSealedSnapshot{}, TimeoutOutcome: model.MatchTimeoutNone}
			before := *stored
			f := newEventBatchFixture(t, nil, []*model.Match{stored})
			f.app.LastTournamentCheckBlock = 50
			f.adapter.On("MatchSnapshot", mock.MatchedBy(resultCallOptsAtBlock(100)),
				[32]byte(stored.CommitmentOne), [32]byte(stored.CommitmentTwo)).Return(test.view, nil).Once()

			batch, err := f.project(&TournamentEvents{MatchDeleted: []*itournament.ITournamentMatchDeleted{event}})
			if test.fail {
				require.ErrorContains(t, err, "deleted match has phase")
				require.Nil(t, batch)
			} else {
				require.NoError(t, err)
				require.Len(t, batch.Matches, 1)
				require.Equal(t, uint64(100), batch.Matches[0].Snapshot.AsOfBlock)
				require.Equal(t, model.MatchPhaseUninitialized, batch.Matches[0].Snapshot.Phase)
				require.Equal(t, event.Raw.TxHash, *batch.Matches[0].DeletionTxHash)
			}
			require.Equal(t, before, *stored, "publication must not mutate the repository-owned row")
		})
	}
}

func TestTournamentEventBatchRefreshesUncertifiedDeletions(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.Match, *model.Application)
	}{
		{"deletion transaction is absent", func(v *model.Match, _ *model.Application) { v.DeletionTxHash = nil }},
		{"empty transaction hash", func(v *model.Match, _ *model.Application) { v.DeletionTxHash = new(common.Hash) }},
		{"missing log index", func(v *model.Match, _ *model.Application) { v.DeletionLogIndex = nil }},
		{"missing deletion block", func(v *model.Match, _ *model.Application) { v.DeletionBlockNumber = 0 }},
		{"not deleted", func(v *model.Match, _ *model.Application) { v.DeletionReason = model.MatchDeletionReason_NOT_DELETED }},
		{"deletion reason is unknown", func(v *model.Match, _ *model.Application) { v.DeletionReason = "UNKNOWN" }},
		{"deletion winner is unknown", func(v *model.Match, _ *model.Application) { v.Winner = "UNKNOWN" }},
		{"missing snapshot", func(v *model.Match, _ *model.Application) { v.Snapshot.AsOfBlock = 0 }},
		{"deletion after snapshot", func(v *model.Match, _ *model.Application) { v.DeletionBlockNumber = 41 }},
		{"snapshot after cursor", func(v *model.Match, _ *model.Application) { v.Snapshot.AsOfBlock = 51 }},
		{"head below published cursor", func(_ *model.Match, app *model.Application) { app.LastTournamentCheckBlock = 101 }},
		{"live phase", func(v *model.Match, _ *model.Application) { v.Snapshot.Phase = model.MatchPhaseBisecting }},
		{"retained timeout", func(v *model.Match, _ *model.Application) { v.Snapshot.TimeoutOutcome = model.MatchTimeoutOneWins }},
		{"retained charge", func(v *model.Match, _ *model.Application) { v.Snapshot.DeferredCharge = 1 }},
		{"retained bisection", func(v *model.Match, _ *model.Application) { v.Snapshot.Bisection = &model.MatchBisectionSnapshot{} }},
		{"retained seal", func(v *model.Match, _ *model.Application) { v.Snapshot.Sealed = &model.MatchSealedSnapshot{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			stored := certifiedDeletedMatch()
			f := newEventBatchFixture(t, nil, []*model.Match{stored})
			f.app.LastTournamentCheckBlock = 50
			test.change(stored, f.app)
			before := *stored
			cause := errors.New("pinned snapshot unavailable")
			f.adapter.On("MatchSnapshot", mock.MatchedBy(resultCallOptsAtBlock(100)),
				[32]byte(stored.CommitmentOne), [32]byte(stored.CommitmentTwo)).Return(ObservedMatchSnapshot{}, cause).Once()

			batch, err := f.project(&TournamentEvents{})
			require.ErrorIs(t, err, cause, "uncertified state must not bypass the pinned read")
			require.Nil(t, batch)
			require.Equal(t, before, *stored)
		})
	}
}

func TestTournamentEventBatchKeepsNewFactsForCertifiedDeletion(t *testing.T) {
	const (
		matchCreationEventName = "creation"
		matchAdvanceEventName  = "advance"
		leafSealEventName      = "leaf seal"
		matchDeletionEventName = "deletion"
	)
	for _, eventName := range []string{matchCreationEventName, matchAdvanceEventName, leafSealEventName, matchDeletionEventName} {
		t.Run(eventName, func(t *testing.T) {
			stored := certifiedDeletedMatch()
			before := *stored
			f := newEventBatchFixture(t, nil, []*model.Match{stored})
			f.app.LastTournamentCheckBlock = 50
			events := &TournamentEvents{}
			switch eventName {
			case matchCreationEventName:
				events.MatchCreated = []*itournament.ITournamentMatchCreated{{
					MatchIdHash: stored.IDHash, One: stored.CommitmentOne, Two: stored.CommitmentTwo,
					LeftOfTwo: stored.LeftOfTwo, EliminableAt: stored.EliminableAt,
					Raw: types.Log{BlockNumber: stored.BlockNumber, TxHash: stored.TxHash, Index: uint(stored.LogIndex)},
				}}
			case matchAdvanceEventName:
				events.MatchAdvanced = []*itournament.ITournamentMatchAdvanced{{
					MatchIdHash: stored.IDHash, SegmentStartPosition: big.NewInt(8),
					Raw: types.Log{BlockNumber: 30, TxHash: common.HexToHash("0x77"), Index: 1},
				}}
			case leafSealEventName:
				events.LeafMatchSealed = []*itournament.ITournamentLeafMatchSealed{{
					MatchIdHash: stored.IDHash, EliminableAt: 34,
					Raw: types.Log{BlockNumber: 30, TxHash: common.HexToHash("0x77"), Index: 1},
				}}
			case matchDeletionEventName:
				events.MatchDeleted = []*itournament.ITournamentMatchDeleted{matchDeletionEvent(stored)}
			}
			f.adapter.On("MatchSnapshot", mock.MatchedBy(resultCallOptsAtBlock(100)),
				[32]byte(stored.CommitmentOne), [32]byte(stored.CommitmentTwo)).Return(ObservedMatchSnapshot{
				Phase: model.MatchPhaseUninitialized, TimeoutOutcome: model.MatchTimeoutNone,
			}, nil).Once()

			batch, err := f.project(events)
			require.NoError(t, err)
			require.Len(t, batch.Matches, 1, "an incoming fact must not be hidden by snapshot reuse")
			require.Equal(t, uint64(100), batch.Matches[0].Snapshot.AsOfBlock)
			require.Equal(t, before, *stored)
			if eventName == matchAdvanceEventName {
				require.Len(t, batch.MatchAdvances, 1)
				require.Equal(t, big.NewInt(8), batch.MatchAdvances[0].SegmentStartPosition.ToBig())
			}
			if eventName == leafSealEventName {
				require.NotNil(t, batch.Matches[0].LeafSeal)
				require.Equal(t, uint64(34), batch.Matches[0].LeafSeal.EliminableAt)
			}
		})
	}
}

func TestTournamentEventBatchRejectsConflictingCertifiedDeletion(t *testing.T) {
	stored := certifiedDeletedMatch()
	before := *stored
	f := newEventBatchFixture(t, nil, []*model.Match{stored})
	f.app.LastTournamentCheckBlock = 50
	event := matchDeletionEvent(stored)
	event.Raw.Index++
	batch, err := f.project(&TournamentEvents{MatchDeleted: []*itournament.ITournamentMatchDeleted{event}})
	require.ErrorContains(t, err, "conflicting deletion events")
	require.Nil(t, batch)
	require.Equal(t, before, *stored)
	require.Empty(t, f.adapter.Calls)
}

func TestDeletedParentMatchStillObservesLiveChild(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
	stored := certifiedDeletedMatch()
	before := *stored
	parent, adapter := expectDeletedMatchTournament(t, f, epoch, *epoch.TournamentAddress, 0, []*model.Match{stored})
	child, _ := expectDeletedMatchTournament(t, f, epoch, common.HexToAddress("0x101"), 1, nil)
	f.children(epoch, parent, child)
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).
		Run(func(args mock.Arguments) {
			batches := args.Get(2).([]*repository.TournamentEventBatch)
			require.Len(t, batches, 2)
			require.Equal(t, parent.Address, batches[0].Tournament.Address)
			require.Empty(t, batches[0].Matches)
			require.Equal(t, child.Address, batches[1].Tournament.Address)
			require.Equal(t, model.TournamentStandingMatchesActive, batches[1].Tournament.Snapshot.Standing)
			require.Equal(t, uint64(100), batches[1].Tournament.Snapshot.AsOfBlock)
		}).Return(nil).Once()

	_, _, err := f.s.observeApplicationTournaments(t.Context(), f.app, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	require.Equal(t, before, *stored)
	adapter.AssertNotCalled(t, "MatchSnapshot", mock.Anything, mock.Anything, mock.Anything)
}

func expectDeletedMatchTournament(
	t *testing.T, f *observerCheckpointFixture, epoch *model.Epoch, address common.Address, level uint64, matches []*model.Match,
) (*model.Tournament, *tournamentAdapterMock) {
	t.Helper()
	kind := model.TournamentKindNonLeaf
	if level == 1 {
		kind = model.TournamentKindLeaf
	}
	tournament := &model.Tournament{Address: address, Level: level, MaxLevel: 2, Kind: kind, StartInstant: 10}
	adapter := &tournamentAdapterMock{}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(tournament, nil).Once()
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{
		BaseCycle: big.NewInt(0), Level: level, Kind: kind, StartInstant: 10,
	}, nil).Once()
	adapter.On("Standing", opts).Return(TournamentStanding{State: model.TournamentStandingMatchesActive}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, opts, TournamentLevel(level), model.TournamentStandingMatchesActive)
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == 51 && opts.End != nil && *opts.End == 100
	})).Return(&TournamentEvents{}, nil).Once()
	counts := zeroStructuralEventCounts()
	if len(matches) != 0 {
		counts.MatchCreated.SetUint64(1)
		counts.MatchDeleted.SetUint64(1)
	}
	adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(50))).Return(counts, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(counts, nil).Once()
	adapter.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(50))).
		Return(canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0), nil).Once()
	filterAddress := address.Hex()
	f.repo.On("ListCommitments", mock.Anything, f.app.Name,
		repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &filterAddress}, repository.Pagination{}, false).
		Return([]*model.Commitment{}, uint64(0), nil).Once()
	f.repo.On("ListMatches", mock.Anything, f.app.Name,
		repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &filterAddress}, repository.Pagination{}, false).
		Return(matches, uint64(len(matches)), nil).Once()
	t.Cleanup(func() { adapter.AssertExpectations(t) })
	return tournament, adapter
}
