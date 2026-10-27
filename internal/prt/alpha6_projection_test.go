// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const projectionAdvanceCase = "advance"
const projectionChildCase = "child"
const projectionSealCase = "seal"

func TestContextualProjectionChecksEachEventPair(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	for _, kind := range []string{projectionAdvanceCase, projectionSealCase, projectionChildCase} {
		for _, side := range []string{"one", "two"} {
			t.Run(kind+"/"+side, func(t *testing.T) {
				match := &model.Match{IDHash: common.HexToHash("0x33"), CommitmentOne: one, CommitmentTwo: two}
				f := newEventBatchFixture(t, nil, []*model.Match{match})
				f.tournament.Height = 68
				a, b := one, two
				if side == "one" {
					a[0]++
				} else {
					b[0]++
				}
				f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(one), [32]byte(two)).Return(
					ObservedMatchSnapshot{Phase: model.MatchPhaseReadyToSeal, ReadyToSeal: &ReadyToSealMatch{
						SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0)}}, nil).Maybe()
				events := &TournamentEvents{}
				switch kind {
				case projectionAdvanceCase:
					events.MatchAdvanced = []*itournament.ITournamentMatchAdvanced{{MatchIdHash: match.IDHash, One: a, Two: b,
						SegmentStartPosition: big.NewInt(0), CurrentHeight: 67}}
				case projectionSealCase:
					events.LeafMatchSealed = []*itournament.ITournamentLeafMatchSealed{{MatchIdHash: match.IDHash, One: a, Two: b,
						DivergencePosition: big.NewInt(0)}}
				case projectionChildCase:
					f.tournament.Kind = model.TournamentKindNonLeaf
					events.NewInnerTournament = []*itournament.ITournamentNewInnerTournament{{MatchIdHash: match.IDHash, One: a, Two: b}}
				}
				got, err := f.project(events)
				require.ErrorContains(t, err, "creation pair")
				require.Nil(t, got)
				require.Empty(t, f.adapter.Calls)
			})
		}
	}
}

func TestContextualProjectionRejectsWrongKindAndAdvanceHeight(t *testing.T) {
	for _, test := range []struct {
		name   string
		kind   model.TournamentKind
		height uint64
		event  string
	}{
		{"seal nonleaf", model.TournamentKindNonLeaf, 68, projectionSealCase},
		{"child leaf", model.TournamentKindLeaf, 68, projectionChildCase},
		{"advance equal", model.TournamentKindLeaf, 68, projectionAdvanceCase},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := &model.Match{IDHash: common.HexToHash("0x33"), CommitmentOne: common.HexToHash("0x11"),
				CommitmentTwo: common.HexToHash("0x22")}
			f := newEventBatchFixture(t, nil, []*model.Match{m})
			f.tournament.Kind, f.tournament.Height = test.kind, test.height
			f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(m.CommitmentOne), [32]byte(m.CommitmentTwo)).Return(
				ObservedMatchSnapshot{Phase: model.MatchPhaseReadyToSeal, ReadyToSeal: &ReadyToSealMatch{
					SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0)}}, nil).Maybe()
			e := &TournamentEvents{}
			switch test.event {
			case projectionSealCase:
				e.LeafMatchSealed = []*itournament.ITournamentLeafMatchSealed{{MatchIdHash: m.IDHash, One: m.CommitmentOne,
					Two: m.CommitmentTwo, DivergencePosition: big.NewInt(0)}}
			case projectionChildCase:
				e.NewInnerTournament = []*itournament.ITournamentNewInnerTournament{{MatchIdHash: m.IDHash,
					One: m.CommitmentOne, Two: m.CommitmentTwo}}
			case projectionAdvanceCase:
				e.MatchAdvanced = []*itournament.ITournamentMatchAdvanced{{MatchIdHash: m.IDHash, One: m.CommitmentOne,
					Two:           m.CommitmentTwo,
					CurrentHeight: 68, SegmentStartPosition: big.NewInt(0)}}
			}
			got, err := f.project(e)
			require.Error(t, err)
			require.Nil(t, got)
			require.Empty(t, f.adapter.Calls)
		})
	}
}

func TestStandingProjectionKeepsProspectiveAndTerminalHistories(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	parent := &model.Match{CommitmentOne: one, CommitmentTwo: two}
	for _, test := range []struct {
		name              string
		state             model.TournamentStandingState
		level             uint64
		candidate, parent *common.Hash
		result, expiry    uint64
	}{
		{"before closure", model.TournamentStandingAwaitingClosure, 0, &one, nil, 0, 0},
		{"root failed", model.TournamentStandingRootFailed, 0, nil, nil, 30, 0},
		{"root winner", model.TournamentStandingRootWinner, 0, &one, nil, 30, 0},
		{"inner no winner", model.TournamentStandingInnerEliminableNoWinner, 1, nil, nil, 30, 0},
		{"inner expired", model.TournamentStandingInnerEliminableWinnerExpired, 1, &one, nil, 30, 0},
		{"inner winner", model.TournamentStandingInnerWinner, 1, &one, &two, 30, 50},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := &model.Tournament{Level: test.level, StartInstant: 10, Allowance: 20, Snapshot: model.TournamentSnapshot{
				Standing: test.state, Candidate: test.candidate, ParentCommitment: test.parent, FinishedAtBlock: test.result,
				WinnerExpiresAt: test.expiry}}
			e := &itournament.ITournamentStandingChanged{MatchCount: big.NewInt(0), ResultAt: 30, Raw: types.Log{BlockNumber: 20, Index: 2}}
			if test.candidate != nil {
				e.Dangling = *test.candidate
			}
			if test.level == 1 && test.candidate != nil {
				e.ParentCommitment = two
				e.WinnerExpiresAt = 50
			}
			batch := &repository.TournamentEventBatch{Tournament: row}
			require.NoError(t, appendTournamentStandingEvents(batch, &model.Application{ID: 7}, &model.Epoch{Index: 8},
				&TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{e}}, nil, parent))
			require.Len(t, batch.StandingChanges, 1)
			got := batch.StandingChanges[0]
			require.Equal(t, uint64(30), got.ResultAt)
			require.Equal(t, e.WinnerExpiresAt, got.WinnerExpiresAt)
			if test.level == 1 && test.candidate != nil {
				require.Equal(t, &two, got.ParentCommitment)
			} else {
				require.Nil(t, got.ParentCommitment)
			}
			require.Equal(t, test.candidate, got.Dangling)
		})
	}
}

func TestStandingProjectionUsesFinalLogAndFullWidthCount(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	row := &model.Tournament{Snapshot: model.TournamentSnapshot{Standing: model.TournamentStandingMatchesActive}}
	matches := map[common.Hash]*model.Match{{1}: {DeletionReason: model.MatchDeletionReason_NOT_DELETED}}
	e := &TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{
		{MatchCount: big.NewInt(1), Raw: types.Log{BlockNumber: 20, TxHash: one, Index: 9}},
		{MatchCount: big.NewInt(0), Dangling: two, ResultAt: 10, Raw: types.Log{BlockNumber: 20, TxHash: two, Index: 3}},
	}}
	require.NoError(t, appendTournamentStandingEvents(&repository.TournamentEventBatch{Tournament: row}, &model.Application{},
		&model.Epoch{}, e, matches, nil))
	reversed := &TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{e.StandingChanged[1], e.StandingChanged[0]}}
	require.NoError(t, appendTournamentStandingEvents(&repository.TournamentEventBatch{Tournament: row}, &model.Application{},
		&model.Epoch{}, reversed, matches, nil))
	for _, change := range []struct {
		name string
		edit func(*itournament.ITournamentStandingChanged)
	}{
		{"count high bits", func(s *itournament.ITournamentStandingChanged) {
			s.MatchCount = new(big.Int).Add(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 64))
		}},
		{"wrong candidate", func(s *itournament.ITournamentStandingChanged) { s.Dangling = one }},
		{"root parent", func(s *itournament.ITournamentStandingChanged) { s.ParentCommitment = one }},
	} {
		t.Run(change.name, func(t *testing.T) {
			first := *e.StandingChanged[0]
			change.edit(&first)
			err := appendTournamentStandingEvents(&repository.TournamentEventBatch{Tournament: row}, &model.Application{}, &model.Epoch{},
				&TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{&first}}, matches, nil)
			require.Error(t, err)
		})
	}
}

func TestStandingProjectionChecksCurrentWinnerAndHistoricalParentMembership(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	for _, fault := range []string{"no fault", "parent", "expiry", "closure", "terminal"} {
		t.Run(fault, func(t *testing.T) {
			row := &model.Tournament{Level: 1, StartInstant: 10, Allowance: 20,
				Snapshot: model.TournamentSnapshot{Standing: model.TournamentStandingInnerWinner, Candidate: &one, ParentCommitment: &two,
					FinishedAtBlock: 30, WinnerExpiresAt: 50}}
			e := &itournament.ITournamentStandingChanged{MatchCount: big.NewInt(0), Dangling: one, ParentCommitment: two, ResultAt: 30,
				WinnerExpiresAt: 50}
			switch fault {
			case "parent":
				e.ParentCommitment = common.HexToHash("0x33")
			case "expiry":
				e.WinnerExpiresAt++
			case "closure":
				e.ResultAt = 29
			case "terminal":
				e.ResultAt = 31
			}
			err := appendTournamentStandingEvents(&repository.TournamentEventBatch{Tournament: row}, &model.Application{}, &model.Epoch{},
				&TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{e}}, nil, &model.Match{CommitmentOne: one,
					CommitmentTwo: two})
			if fault == "no fault" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestContestedStatesCompareOnlyCurrentSealedParent(t *testing.T) {
	zero, other := common.Hash{}, common.HexToHash("0x22")
	child := &model.Tournament{ContestedFinalStateOne: &zero, ContestedFinalStateTwo: &other}
	parent := &model.Match{Snapshot: model.MatchSnapshot{Phase: model.MatchPhaseSealed,
		Sealed: &model.MatchSealedSnapshot{FinalStateOne: zero, FinalStateTwo: other}}}
	require.NoError(t, validateChildContestedStates(child, parent))
	parent.Snapshot.Sealed.FinalStateTwo = zero
	require.Error(t, validateChildContestedStates(child, parent))
	parent.Snapshot = model.MatchSnapshot{Phase: model.MatchPhaseUninitialized}
	require.NoError(t, validateChildContestedStates(child, parent))
	child.ContestedFinalStateOne = nil
	require.Error(t, validateChildContestedStates(child, parent))
}

func TestStoredStandingBaselineUsesActualRows(t *testing.T) {
	require.NoError(t, validateStoredStandingCount(repository.TournamentEventCounts{CommitmentJoined: 7, MatchDeleted: 3,
		StandingChanged: 10}))
	require.Error(t, validateStoredStandingCount(repository.TournamentEventCounts{CommitmentJoined: 7, MatchDeleted: 3,
		StandingChanged: 9}))
	require.Error(t, validateStoredStandingCount(repository.TournamentEventCounts{CommitmentJoined: ^uint64(0), MatchDeleted: 1,
		StandingChanged: 0}))
}

func TestSealProjectionPreservesFullWidthAndZeroValues(t *testing.T) {
	for _, test := range []struct {
		name     string
		position *big.Int
		valid    bool
	}{
		{"zero", big.NewInt(0), true},
		{"wide valid", new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 65), big.NewInt(7)), true},
		{"last", new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 68), big.NewInt(1)), true},
		{"end", new(big.Int).Lsh(big.NewInt(1), 68), false},
		{"same low64 invalid", new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 68), big.NewInt(7)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
			m := &model.Match{IDHash: common.HexToHash("0x33"), CommitmentOne: one, CommitmentTwo: two}
			f := newEventBatchFixture(t, nil, []*model.Match{m})
			f.tournament.Height = 68
			viewCall := f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(one), [32]byte(two)).Return(
				ObservedMatchSnapshot{Phase: model.MatchPhaseSealed, Sealed: &SealedMatch{
					DivergencePosition: new(big.Int).Set(test.position), DivergenceCycle: big.NewInt(0), FinalStateTwo: two}}, nil)
			if test.valid {
				viewCall.Once()
			} else {
				viewCall.Maybe()
			}

			e := &itournament.ITournamentLeafMatchSealed{MatchIdHash: m.IDHash, One: one, Two: two, DivergencePosition: test.position,
				FinalStateTwo: two, DeadlineOne: 71, DeadlineTwo: 83, EliminableAt: 83, Raw: types.Log{BlockNumber: 50,
					TxHash: common.HexToHash("0x44"), Index: 2}}
			batch, err := f.project(&TournamentEvents{LeafMatchSealed: []*itournament.ITournamentLeafMatchSealed{e}})
			if !test.valid {
				require.ErrorContains(t, err, "divergence position")
				require.Nil(t, batch)
				return
			}
			require.NoError(t, err)
			seal := batch.Matches[0].LeafSeal
			require.NotNil(t, seal)
			require.Zero(t, test.position.Cmp(seal.DivergencePosition.ToBig()))
			require.Zero(t, seal.AgreeState)
			require.Zero(t, seal.FinalStateOne)
			require.Equal(t, two, seal.FinalStateTwo)
			require.Equal(t, uint64(71), seal.DeadlineOne)
			require.Equal(t, uint64(83), seal.DeadlineTwo)
		})
	}
}

func TestProjectionFoldsRepairAndTwoDeletionsInOneTransaction(t *testing.T) {
	one, two, three := common.HexToHash("0x11"), common.HexToHash("0x22"), common.HexToHash("0x33")
	oldOne := &model.Match{IDHash: common.HexToHash("0xa1"), CommitmentOne: one, CommitmentTwo: two}
	oldTwo := &model.Match{IDHash: common.HexToHash("0xa2"), CommitmentOne: two, CommitmentTwo: three}
	f := newEventBatchFixture(t, nil, []*model.Match{oldOne, oldTwo})
	f.tournament.Snapshot.Standing = model.TournamentStandingMatchesActive
	tx := common.HexToHash("0xbb")
	created := &itournament.ITournamentMatchCreated{MatchIdHash: common.HexToHash("0xa3"), One: one, Two: three,
		ResponderDeadline: 120, EliminableAt: 130, Raw: types.Log{BlockNumber: 90, TxHash: tx, Index: 4}}
	e := &TournamentEvents{MatchCreated: []*itournament.ITournamentMatchCreated{created},
		MatchDeleted: []*itournament.ITournamentMatchDeleted{
			{MatchIdHash: oldOne.IDHash, One: one, Two: two, Reason: 1, WinnerCommitment: 1, Raw: types.Log{BlockNumber: 90,
				TxHash: tx, Index: 5}},
			{MatchIdHash: oldTwo.IDHash, One: two, Two: three, Reason: 1, WinnerCommitment: 0, Raw: types.Log{BlockNumber: 90,
				TxHash: tx, Index: 7}},
		}, StandingChanged: []*itournament.ITournamentStandingChanged{
			{MatchCount: big.NewInt(1), Raw: types.Log{BlockNumber: 90, TxHash: tx, Index: 8}},
			{MatchCount: big.NewInt(2), Raw: types.Log{BlockNumber: 90, TxHash: tx, Index: 6}},
		}}
	for _, m := range []*model.Match{oldOne, oldTwo} {
		f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(m.CommitmentOne),
			[32]byte(m.CommitmentTwo)).Return(ObservedMatchSnapshot{Phase: model.MatchPhaseUninitialized,
			TimeoutOutcome: model.MatchTimeoutNone}, nil).Once()
	}
	f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(one),
		[32]byte(three)).Return(ObservedMatchSnapshot{Phase: model.MatchPhaseReadyToSeal,
		ReadyToSeal: &ReadyToSealMatch{SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0)}}, nil).Once()
	batch, err := f.project(e)
	require.NoError(t, err)
	require.Len(t, batch.Matches, 3)
	require.Len(t, batch.StandingChanges, 2)
	require.Equal(t, uint64(8), batch.StandingChanges[0].LogIndex)
	for _, m := range batch.Matches {
		if m.IDHash == created.MatchIdHash {
			require.Equal(t, uint64(120), m.ResponderDeadline)
		}
	}
	require.Nil(t, oldOne.DeletionTxHash)
	require.Nil(t, oldTwo.DeletionTxHash)
}

func TestEmptyStandingWindowDoesNotComparePastHistoryWithChangedClock(t *testing.T) {
	row := &model.Tournament{Snapshot: model.TournamentSnapshot{Standing: model.TournamentStandingRootFailed, FinishedAtBlock: 100}}
	batch := &repository.TournamentEventBatch{Tournament: row}
	require.NoError(t, appendTournamentStandingEvents(batch, &model.Application{}, &model.Epoch{}, &TournamentEvents{},
		map[common.Hash]*model.Match{{1}: {}}, nil))
	require.Empty(t, batch.StandingChanges)
}

func TestAdvanceProjectionKeepsIndependentHeightAndSchedule(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	m := &model.Match{IDHash: common.HexToHash("0xa0"), CommitmentOne: one, CommitmentTwo: two}
	f := newEventBatchFixture(t, nil, []*model.Match{m})
	f.tournament.Height = 68
	f.adapter.On("MatchSnapshot", mock.Anything, [32]byte(one),
		[32]byte(two)).Return(ObservedMatchSnapshot{Phase: model.MatchPhaseReadyToSeal,
		ReadyToSeal: &ReadyToSealMatch{SegmentStartPosition: big.NewInt(0), SegmentStartCycle: big.NewInt(0)}}, nil).Once()
	e := &itournament.ITournamentMatchAdvanced{MatchIdHash: m.IDHash, One: one, Two: two, CurrentHeight: 67, ResponderDeadline: 71,
		EliminableAt: 83, SegmentStartPosition: big.NewInt(7)}
	batch, err := f.project(&TournamentEvents{MatchAdvanced: []*itournament.ITournamentMatchAdvanced{e}})
	require.NoError(t, err)
	require.Equal(t, uint64(67), batch.MatchAdvances[0].CurrentHeight)
	require.Equal(t, uint64(71), batch.MatchAdvances[0].ResponderDeadline)
	require.Equal(t, uint64(83), batch.MatchAdvances[0].EliminableAt)
}

func TestInnerStandingUsesStoredParentPairAndRejectsForeignHistoricalParent(t *testing.T) {
	one, two := common.HexToHash("0x11"), common.HexToHash("0x22")
	f := newEventBatchFixture(t, nil, nil)
	f.tournament.Level = 1
	f.tournament.ParentTournamentAddress = new(common.HexToAddress("0x99"))
	f.tournament.ParentMatchIDHash = new(common.HexToHash("0x9a"))
	f.tournament.Snapshot = model.TournamentSnapshot{Standing: model.TournamentStandingInnerEliminableWinnerExpired, Candidate: &one,
		FinishedAtBlock: 30}
	f.repo.On("GetMatch", mock.Anything, f.app.Name, f.epoch.Index, f.tournament.ParentTournamentAddress.Hex(),
		f.tournament.ParentMatchIDHash.Hex()).Return(&model.Match{CommitmentOne: one, CommitmentTwo: two}, nil).Once()
	e := &itournament.ITournamentStandingChanged{MatchCount: big.NewInt(0), Dangling: one, ParentCommitment: two, ResultAt: 30,
		WinnerExpiresAt: 50}
	batch, err := f.project(&TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{e}})
	require.NoError(t, err)
	require.Equal(t, &two, batch.StandingChanges[0].ParentCommitment)
	e.ParentCommitment = common.HexToHash("0x33")
	require.ErrorContains(t, appendTournamentStandingEvents(&repository.TournamentEventBatch{Tournament: f.tournament}, f.app, f.epoch,
		&TournamentEvents{StandingChanged: []*itournament.ITournamentStandingChanged{e}}, nil, &model.Match{CommitmentOne: one,
			CommitmentTwo: two}), "parent match pair")
}
