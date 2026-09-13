// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type eventBatchFixture struct {
	*observerCheckpointFixture
	epoch      *Epoch
	tournament *Tournament
	adapter    *tournamentAdapterMock
}

func newEventBatchFixture(t *testing.T, commitments []*Commitment, matches []*Match) *eventBatchFixture {
	t.Helper()
	s, repo := newPRTServiceMock()
	base := &observerCheckpointFixture{t: t, s: s, repo: repo, app: prtRevertTestApp()}
	epoch := checkpointEpoch(0, "0x100")
	f := &eventBatchFixture{observerCheckpointFixture: base, epoch: epoch,
		tournament: &Tournament{Address: *epoch.TournamentAddress}, adapter: &tournamentAdapterMock{}}
	address := f.tournament.Address.Hex()
	repo.On("ListCommitments", mock.Anything, base.app.Name,
		repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &address}, repository.Pagination{}, false).
		Return(commitments, uint64(len(commitments)), nil).Once()
	repo.On("ListMatches", mock.Anything, base.app.Name,
		repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &address}, repository.Pagination{}, false).
		Return(matches, uint64(len(matches)), nil).Once()
	t.Cleanup(func() { repo.AssertExpectations(t); f.adapter.AssertExpectations(t) })
	return f
}

func (f *eventBatchFixture) project(events *TournamentEvents) (*repository.TournamentEventBatch, error) {
	return f.s.tournamentEventBatch(f.t.Context(), f.app, f.epoch, f.tournament, f.adapter, events, 100)
}

func TestTournamentEventBatchKeepsFactsAndCurrentViewsSeparate(t *testing.T) {
	f := newEventBatchFixture(t, nil, nil)
	one, two, matchID := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	finalOne, finalTwo := common.HexToHash("0x44"), common.HexToHash("0x55")
	tx := common.HexToHash("0x900")
	logAt := func(index uint) types.Log { return types.Log{BlockNumber: 100, TxHash: tx, Index: index} }
	large := new(big.Int).Lsh(big.NewInt(1), 200)
	events := &TournamentEvents{
		CommitmentJoined: []*itournament.ITournamentCommitmentJoined{
			{Commitment: one, FinalStateHash: finalOne, Submitter: common.HexToAddress("0xa"), Raw: logAt(0)},
			{Commitment: two, FinalStateHash: finalTwo, Submitter: common.HexToAddress("0xb"), Raw: logAt(1)},
		},
		MatchCreated: []*itournament.ITournamentMatchCreated{{
			MatchIdHash: matchID, One: one, Two: two, LeftOfTwo: finalTwo, EliminableAt: 120, Raw: logAt(2),
		}},
		MatchAdvanced: []*itournament.ITournamentMatchAdvanced{
			{MatchIdHash: matchID, OtherParent: finalOne, LeftNode: one, SegmentStartPosition: large, EliminableAt: 121, Raw: logAt(3)},
			{MatchIdHash: matchID, OtherParent: finalOne, LeftNode: two,
				SegmentStartPosition: big.NewInt(8), EliminableAt: 122, Raw: logAt(4)},
		},
		LeafMatchSealed: []*itournament.ITournamentLeafMatchSealed{{MatchIdHash: matchID, EliminableAt: 123, Raw: logAt(5)}},
		MatchDeleted:    []*itournament.ITournamentMatchDeleted{{MatchIdHash: matchID, One: one, Two: two, Reason: 1, Raw: logAt(6)}},
		PartialBondRefund: []*itournament.ITournamentPartialBondRefund{{
			Recipient: common.HexToAddress("0xc"), Value: large, Success: false, Raw: logAt(7),
		}},
		BondRecovered: []*itournament.ITournamentBondRecovered{{
			Commitment: one, Claimer: common.HexToAddress("0xa"), Payment: big.NewInt(0), Burned: big.NewInt(7), Raw: logAt(8),
		}},
	}
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	f.adapter.On("CommitmentStanding", opts, [32]byte(one)).Return(CommitmentStanding{
		Joined: true, FinalState: finalOne, ClockRunning: true, ClockDeadline: 110, ClockAllowance: 30,
	}, nil).Once()
	f.adapter.On("CommitmentStanding", opts, [32]byte(two)).Return(CommitmentStanding{
		Joined: true, FinalState: finalTwo, ClockAllowance: 20,
	}, nil).Once()
	f.adapter.On("MatchSnapshot", opts, [32]byte(one), [32]byte(two)).Return(ObservedMatchSnapshot{
		Phase: MatchPhaseUninitialized, TimeoutOutcome: MatchTimeoutNone,
	}, nil).Once()
	batch, err := f.project(events)
	require.NoError(t, err)
	require.Len(t, batch.Commitments, 2)
	require.Equal(t, common.HexToAddress("0xa"), batch.Commitments[0].SubmitterAddress)
	require.Zero(t, batch.Commitments[0].Snapshot.Claimer, "recovery clears the current claimer, not the join submitter")
	require.True(t, batch.Commitments[0].Snapshot.ClockRunning, "raw retained clocks are not a liveness classification")
	require.Equal(t, uint64(100), batch.Commitments[0].Snapshot.AsOfBlock)
	require.Len(t, batch.MatchAdvances, 2, "the same parent hash can occur in distinct events")
	require.Equal(t, uint64(3), batch.MatchAdvances[0].LogIndex)
	require.Equal(t, uint64(4), batch.MatchAdvances[1].LogIndex)
	require.Equal(t, large, batch.MatchAdvances[0].SegmentStartPosition.ToBig())
	require.Len(t, batch.Matches, 1)
	match := batch.Matches[0]
	require.Equal(t, uint64(120), match.EliminableAt, "creation data must not become a later deadline")
	require.Equal(t, uint64(123), match.LeafSeal.EliminableAt)
	require.Equal(t, uint64(5), match.LeafSeal.LogIndex)
	require.Equal(t, tx, *match.DeletionTxHash)
	require.Equal(t, uint64(6), *match.DeletionLogIndex)
	require.Equal(t, MatchDeletionReason_TIMEOUT, match.DeletionReason)
	require.Equal(t, MatchPhaseUninitialized, match.Snapshot.Phase)
	require.Nil(t, match.Snapshot.Bisection)
	require.Nil(t, match.Snapshot.Sealed)
	require.Len(t, batch.BondEvents, 2)
	require.False(t, batch.BondEvents[0].Refund.Success)
	require.Nil(t, batch.BondEvents[0].Recovery)
	require.Nil(t, batch.BondEvents[1].Refund)
	require.Zero(t, batch.BondEvents[1].Recovery.Payment.ToBig().Sign())
	large.SetUint64(0)
	events.MatchDeleted[0].Raw.TxHash = common.Hash{}
	require.Equal(t, 201, batch.MatchAdvances[0].SegmentStartPosition.ToBig().BitLen())
	require.Equal(t, 201, batch.BondEvents[0].Refund.Value.ToBig().BitLen())
	require.Equal(t, tx, *match.DeletionTxHash)
	f.repo.AssertNotCalled(t, "StoreTournamentEvents", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentEventBatchRejectsUnknownOrContradictoryDeletion(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	for _, test := range []struct {
		name           string
		reason, winner uint8
		badPair        bool
	}{
		{name: "unknown reason", reason: 255}, {name: "unknown winner", winner: 3}, {name: "different pair", badPair: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stored := &Match{IDHash: common.HexToHash("0x33"), CommitmentOne: one, CommitmentTwo: two,
				DeletionReason: MatchDeletionReason_NOT_DELETED}
			before := *stored
			f := newEventBatchFixture(t, nil, []*Match{stored})
			event := &itournament.ITournamentMatchDeleted{MatchIdHash: stored.IDHash, One: one, Two: two,
				Reason: test.reason, WinnerCommitment: test.winner, Raw: types.Log{BlockNumber: 100}}
			if test.badPair {
				event.Two = one
			}
			_, err := f.project(&TournamentEvents{MatchDeleted: []*itournament.ITournamentMatchDeleted{event}})
			require.Error(t, err)
			require.Equal(t, before, *stored)
			require.Empty(t, f.adapter.Calls)
		})
	}
}

func TestTournamentEventBatchReadFailuresDoNotMutateStoredRows(t *testing.T) {
	for _, failed := range []string{"commitment", "match"} {
		t.Run(failed, func(t *testing.T) {
			commitment := &Commitment{Commitment: common.HexToHash("0x11"), FinalStateHash: common.HexToHash("0x22")}
			match := &Match{IDHash: common.HexToHash("0x33"), CommitmentOne: commitment.Commitment}
			beforeCommitment, beforeMatch := *commitment, *match
			f := newEventBatchFixture(t, []*Commitment{commitment}, []*Match{match})
			cause := errors.New("snapshot unavailable")
			var commitmentError error
			if failed == "commitment" {
				commitmentError = cause
			}
			f.adapter.On("CommitmentStanding", mock.Anything, [32]byte(commitment.Commitment)).Return(
				CommitmentStanding{Joined: true, FinalState: commitment.FinalStateHash, ClockAllowance: 7}, commitmentError).Once()
			if commitmentError == nil {
				f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(match.CommitmentOne), [32]byte(match.CommitmentTwo)).
					Return(ObservedMatchSnapshot{}, cause).Once()
			}
			_, err := f.project(&TournamentEvents{})
			require.ErrorIs(t, err, cause)
			require.Equal(t, beforeCommitment, *commitment)
			require.Equal(t, beforeMatch, *match)
		})
	}
}

func TestTournamentCreationEventChecksCloneStartAndKeepsIdentity(t *testing.T) {
	matchID := common.HexToHash("0x11")
	child := &Tournament{Address: common.HexToAddress("0x100"), ParentMatchIDHash: &matchID, StartInstant: 50}
	event := &itournament.ITournamentNewInnerTournament{MatchIdHash: matchID, ChildTournament: child.Address,
		Raw: types.Log{BlockNumber: 50, TxHash: common.HexToHash("0x22"), Index: 4}}
	require.NoError(t, applyTournamentCreationEvent(child, event))
	require.Equal(t, uint64(4), child.CreationEvent.LogIndex)
	before := *child.CreationEvent
	require.NoError(t, applyTournamentCreationEvent(child, event), "an exact repeated fact is harmless")
	event.Raw.Index++
	require.ErrorContains(t, applyTournamentCreationEvent(child, event), "conflicting creation events")
	require.Equal(t, before, *child.CreationEvent)
	event.Raw.BlockNumber++
	require.ErrorContains(t, applyTournamentCreationEvent(child, event), "descriptor does not match")
}
