// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTournamentRetirementRequiresImmutableCommittedSnapshot(t *testing.T) {
	base := Tournament{Snapshot: TournamentSnapshot{AsOfBlock: 50, FinishedAtBlock: 40,
		Standing: TournamentStandingRootWinner, BondRecovery: TournamentBondRecovery{Disposition: BondDispositionRecovered}}}
	for _, test := range []struct {
		name    string
		change  func(*Tournament)
		retired bool
	}{
		{"recovered root", func(*Tournament) {}, true},
		{"failed root", func(v *Tournament) {
			v.Snapshot.Standing, v.Snapshot.BondRecovery.Disposition = TournamentStandingRootFailed, BondDispositionNoWinner
		}, true},
		{"unfinished", func(v *Tournament) { v.Snapshot.FinishedAtBlock = 0 }, false},
		{"finish not covered", func(v *Tournament) { v.Snapshot.FinishedAtBlock = 51 }, false},
		{"snapshot not committed", func(v *Tournament) { v.Snapshot.AsOfBlock = 51 }, false},
		{"recoverable bond", func(v *Tournament) { v.Snapshot.BondRecovery.Disposition = BondDispositionRecoverable }, false},
		{"inner winner still expires", func(v *Tournament) {
			v.Level, v.CreationEvent, v.Snapshot.Standing = 1, &TournamentCreationEvent{}, TournamentStandingInnerWinner
		}, false},
		{"expired winner recovered", func(v *Tournament) {
			v.Level, v.CreationEvent, v.Snapshot.Standing = 1, &TournamentCreationEvent{}, TournamentStandingInnerEliminableWinnerExpired
		}, true},
		{"missing child creation", func(v *Tournament) {
			v.Level, v.Snapshot.Standing = 1, TournamentStandingInnerEliminableNoWinner
		}, false},
		{"child no winner", func(v *Tournament) {
			v.Level, v.CreationEvent, v.Snapshot.Standing = 1, &TournamentCreationEvent{}, TournamentStandingInnerEliminableNoWinner
			v.Snapshot.BondRecovery.Disposition = BondDispositionNoWinner
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.change(&value)
			require.Equal(t, test.retired, tournamentObservationComplete(&value, 50, 100))
		})
	}
	require.False(t, tournamentObservationComplete(&base, 50, 49), "a later snapshot cannot decide retirement at an older head")
}

func TestRetiredParentStillObservesMutableChildren(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	epoch.Status = EpochStatus_ClaimAccepted
	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
	parent := &Tournament{Address: *epoch.TournamentAddress, MaxLevel: 2, Snapshot: TournamentSnapshot{
		AsOfBlock: 50, FinishedAtBlock: 40, Standing: TournamentStandingRootWinner,
		BondRecovery: TournamentBondRecovery{Disposition: BondDispositionRecovered},
	}}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), parent.Address.Hex()).Return(parent, nil).Once()
	child := f.tournament(epoch, common.HexToAddress("0x101"), 1, 2, 45, 100, &TournamentEvents{}, nil)
	retiredChild := &Tournament{Address: common.HexToAddress("0x102"), Level: 1, MaxLevel: 2,
		CreationEvent: &TournamentCreationEvent{BlockNumber: 10}, Snapshot: TournamentSnapshot{
			AsOfBlock: 50, FinishedAtBlock: 45, Standing: TournamentStandingInnerEliminableNoWinner,
			BondRecovery: TournamentBondRecovery{Disposition: BondDispositionNoWinner},
		}}
	f.children(epoch, parent, child, retiredChild)
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), retiredChild.Address.Hex()).Return(retiredChild, nil).Once()
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
		mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
			return len(batches) == 1 && batches[0].Tournament.Address == child.Address &&
				batches[0].Tournament.Snapshot.AsOfBlock == 100
		}), uint64(100)).Return(nil).Once()
	_, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(50), parent.Snapshot.AsOfBlock)
	require.Equal(t, uint64(50), retiredChild.Snapshot.AsOfBlock)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	f.factory.AssertNotCalled(t, "CreateTournamentAdapter", parent.Address)
	f.factory.AssertNotCalled(t, "CreateTournamentAdapter", retiredChild.Address)
}
