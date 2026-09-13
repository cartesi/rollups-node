// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func structuralCountsForEvents(events *TournamentEvents) StructuralEventCounts {
	return StructuralEventCounts{
		CommitmentJoined:   new(big.Int).SetUint64(uint64(len(events.CommitmentJoined))),
		MatchCreated:       new(big.Int).SetUint64(uint64(len(events.MatchCreated))),
		MatchAdvanced:      new(big.Int).SetUint64(uint64(len(events.MatchAdvanced))),
		LeafMatchSealed:    new(big.Int).SetUint64(uint64(len(events.LeafMatchSealed))),
		MatchDeleted:       new(big.Int).SetUint64(uint64(len(events.MatchDeleted))),
		NewInnerTournament: new(big.Int).SetUint64(uint64(len(events.NewInnerTournament))),
	}
}

func TestTournamentObservationRejectsIncompleteFinancialEvents(t *testing.T) {
	for _, omission := range []string{"refund", "recovery", "previous recovery view"} {
		t.Run(omission, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			epoch := checkpointEpoch(0, "0x100")
			f.epochs(epoch)
			opts := mock.MatchedBy(resultCallOptsAtBlock(100))
			previousOpts := mock.MatchedBy(resultCallOptsAtBlock(50))
			f.consensus.On("TournamentLevelCount", opts).Return(uint64(1), nil).Once()
			address := *epoch.TournamentAddress
			projection := &model.Tournament{Address: address, MaxLevel: 1, Kind: model.TournamentKindLeaf, StartInstant: 10}
			before := *projection
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(projection, nil).Once()
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
			adapter.On("Descriptor", opts).Return(TournamentDescriptor{BaseCycle: new(big.Int), Kind: model.TournamentKindLeaf,
				StartInstant: 10}, nil).Once()
			adapter.On("Standing", opts).Return(TournamentStanding{State: model.TournamentStandingRootWinner,
				HasCandidate: true, Candidate: *epoch.Commitment, FinishedAt: 90}, nil).Once()
			disposition := model.BondDispositionRecoverable
			if omission == "recovery" {
				disposition = model.BondDispositionRecovered
			}
			adapter.On("BondRecovery", opts).Return(canonicalBondRecovery(disposition, common.HexToAddress("0x777"), 0), nil).Once()
			events := &TournamentEvents{}
			if omission == "refund" {
				events.MatchDeleted = []*itournament.ITournamentMatchDeleted{{}}
			}
			adapter.On("RetrieveAllEvents", mock.Anything).Return(events, nil).Once()
			adapter.On("StructuralEventCounts", previousOpts).Return(zeroStructuralEventCounts(), nil).Once()
			if omission == "previous recovery view" {
				adapter.On("BondRecovery", previousOpts).Return(BondRecovery{}, fmt.Errorf("previous recovery view unavailable")).Once()
			} else {
				adapter.On("BondRecovery", previousOpts).
					Return(canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0), nil).Once()
				adapter.On("StructuralEventCounts", opts).Return(structuralCountsForEvents(events), nil).Once()
			}
			_, err := f.s.checkEpochs(t.Context(), f.app, 100)
			require.Error(t, err)
			require.Equal(t, before, *projection)
			require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			require.Equal(t, uint8(1), f.s.observationFailures[f.app.ID].failedHeads)
			f.repo.AssertNotCalled(t, "StoreTournamentEvents", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			adapter.AssertExpectations(t)
		})
	}
}

func TestTournamentObservationPublishesLateRecovery(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	epoch.Status = model.EpochStatus_ClaimAccepted
	f.epochs(epoch)
	opts, previousOpts := mock.MatchedBy(resultCallOptsAtBlock(100)), mock.MatchedBy(resultCallOptsAtBlock(50))
	f.consensus.On("TournamentLevelCount", opts).Return(uint64(1), nil).Once()
	address := *epoch.TournamentAddress
	projection := &model.Tournament{Address: address, MaxLevel: 1, Kind: model.TournamentKindLeaf, StartInstant: 10,
		Snapshot: model.TournamentSnapshot{AsOfBlock: 50, FinishedAtBlock: 40, Standing: model.TournamentStandingRootWinner,
			BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionRecoverable}}}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(projection, nil).Once()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{BaseCycle: new(big.Int), Kind: model.TournamentKindLeaf,
		StartInstant: 10}, nil).Once()
	adapter.On("Standing", opts).Return(TournamentStanding{State: model.TournamentStandingRootWinner,
		HasCandidate: true, Candidate: *epoch.Commitment, FinishedAt: 40}, nil).Once()
	adapter.On("BondRecovery", opts).Return(canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil).Once()
	adapter.On("BondRecovery", previousOpts).
		Return(canonicalBondRecovery(model.BondDispositionRecoverable, common.HexToAddress("0x777"), 0), nil).Once()
	events := &TournamentEvents{BondRecovered: []*itournament.ITournamentBondRecovered{{
		Commitment: *epoch.Commitment, Claimer: common.HexToAddress("0x777"), Payment: new(big.Int), Burned: big.NewInt(8),
	}}}
	adapter.On("RetrieveAllEvents", mock.Anything).Return(events, nil).Once()
	adapter.On("StructuralEventCounts", previousOpts).Return(zeroStructuralEventCounts(), nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
	f.emptyParticipants(epoch, address)
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
		mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
			return len(batches) == 1 && len(batches[0].BondEvents) == 1 &&
				batches[0].Tournament.Snapshot.BondRecovery.Disposition == model.BondDispositionRecovered &&
				batches[0].Tournament.Snapshot.AsOfBlock == 100
		}), uint64(100)).Return(nil).Once()
	_, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	require.Equal(t, uint64(50), projection.Snapshot.AsOfBlock)
	adapter.AssertExpectations(t)
}

func TestTournamentFinancialEventsRequireEachRefund(t *testing.T) {
	for _, test := range []struct {
		name   string
		events TournamentEvents
	}{
		{"advance", TournamentEvents{MatchAdvanced: []*itournament.ITournamentMatchAdvanced{{}}}},
		{"leaf seal", TournamentEvents{LeafMatchSealed: []*itournament.ITournamentLeafMatchSealed{{}}}},
		{"deletion", TournamentEvents{MatchDeleted: []*itournament.ITournamentMatchDeleted{{}}}},
		{"child creation", TournamentEvents{NewInnerTournament: []*itournament.ITournamentNewInnerTournament{{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, validateTournamentFinancialEvents(model.BondDispositionTournamentRunning,
				model.BondDispositionTournamentRunning, &test.events), "PartialBondRefund event count mismatch")
			for _, paid := range []bool{false, true} {
				test.events.PartialBondRefund = []*itournament.ITournamentPartialBondRefund{{Value: big.NewInt(0), Success: paid}}
				require.NoError(t, validateTournamentFinancialEvents(model.BondDispositionTournamentRunning,
					model.BondDispositionTournamentRunning, &test.events))
			}
			test.events.PartialBondRefund = append(test.events.PartialBondRefund, test.events.PartialBondRefund[0])
			require.ErrorContains(t, validateTournamentFinancialEvents(model.BondDispositionTournamentRunning,
				model.BondDispositionTournamentRunning, &test.events), "PartialBondRefund event count mismatch")
		})
	}
}

func TestTournamentFinancialEventsRequireOneRecoveryTransition(t *testing.T) {
	for _, before := range []model.BondDisposition{model.BondDispositionTournamentRunning, model.BondDispositionRecoverable} {
		t.Run(string(before), func(t *testing.T) {
			events := &TournamentEvents{}
			require.ErrorContains(t, validateTournamentFinancialEvents(before, model.BondDispositionRecovered, events),
				"BondRecovered event count mismatch")
			events.BondRecovered = []*itournament.ITournamentBondRecovered{{Payment: new(big.Int), Burned: new(big.Int)}}
			require.NoError(t, validateTournamentFinancialEvents(before, model.BondDispositionRecovered, events))
			events.BondRecovered = append(events.BondRecovered, events.BondRecovered[0])
			require.ErrorContains(t, validateTournamentFinancialEvents(before, model.BondDispositionRecovered, events),
				"BondRecovered event count mismatch")
		})
	}
	for _, disposition := range []model.BondDisposition{model.BondDispositionTournamentRunning, model.BondDispositionNoWinner,
		model.BondDispositionRecoverable, model.BondDispositionRecovered} {
		// Failed payment remains recoverable; an already recovered retry is a no-op.
		require.NoError(t, validateTournamentFinancialEvents(disposition, disposition, &TournamentEvents{}))
		require.Error(t, validateTournamentFinancialEvents(disposition, disposition,
			&TournamentEvents{BondRecovered: []*itournament.ITournamentBondRecovered{{}}}))
	}
	require.ErrorContains(t, validateTournamentFinancialEvents(model.BondDispositionRecovered, model.BondDispositionRecoverable,
		&TournamentEvents{}), "regressed")
}

func TestTournamentEventCountsCheckEveryStructuralStream(t *testing.T) {
	for _, test := range []struct {
		name   string
		events TournamentEvents
	}{
		{tournamentEventCommitmentJoined, TournamentEvents{CommitmentJoined: []*itournament.ITournamentCommitmentJoined{{}}}},
		{tournamentEventMatchCreated, TournamentEvents{MatchCreated: []*itournament.ITournamentMatchCreated{{}}}},
		{tournamentEventMatchAdvanced, TournamentEvents{MatchAdvanced: []*itournament.ITournamentMatchAdvanced{{}}}},
		{tournamentEventLeafMatchSealed, TournamentEvents{LeafMatchSealed: []*itournament.ITournamentLeafMatchSealed{{}}}},
		{tournamentEventMatchDeleted, TournamentEvents{MatchDeleted: []*itournament.ITournamentMatchDeleted{{}}}},
		{tournamentEventNewInnerTournament, TournamentEvents{NewInnerTournament: []*itournament.ITournamentNewInnerTournament{{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, after := zeroStructuralEventCounts(), structuralCountsForEvents(&test.events)
			require.NoError(t, validateTournamentEventCounts(before, after, &test.events))
			err := validateTournamentEventCounts(before, after, &TournamentEvents{})
			require.ErrorContains(t, err, test.name+" event count mismatch")
			err = validateTournamentEventCounts(before, before, &test.events)
			require.ErrorContains(t, err, test.name+" event count mismatch")
		})
	}
}

func TestTournamentEventCountsKeepFullWidthAndRejectInvalidCounters(t *testing.T) {
	before, after := zeroStructuralEventCounts(), zeroStructuralEventCounts()
	before.MatchAdvanced.Lsh(big.NewInt(1), 200)
	after.MatchAdvanced.Add(before.MatchAdvanced, big.NewInt(1))
	beforeCopy, afterCopy := new(big.Int).Set(before.MatchAdvanced), new(big.Int).Set(after.MatchAdvanced)
	events := &TournamentEvents{MatchAdvanced: []*itournament.ITournamentMatchAdvanced{{}}}
	require.NoError(t, validateTournamentEventCounts(before, after, events))
	require.Equal(t, beforeCopy, before.MatchAdvanced)
	require.Equal(t, afterCopy, after.MatchAdvanced)
	for _, invalid := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 256)} {
		after.MatchAdvanced = invalid
		require.Error(t, validateTournamentEventCounts(before, after, events))
	}
	after.MatchAdvanced = new(big.Int).Sub(before.MatchAdvanced, big.NewInt(1))
	require.ErrorContains(t, validateTournamentEventCounts(before, after, events), "count decreased")
}
