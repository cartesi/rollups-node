// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

// Store the observed projections only after the complete-window write succeeds.
// The normal fake records writes but does not expose their resulting read view.
type initialRootRepository struct {
	*deliveryRepoFake
}

func TestDeliveryFirstRootDiscoveredAfterEmptyObserveDoesNotNeedAnotherBlock(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.app.LastTournamentCheckBlock = 0
	f.repo.publishedRoots = nil
	f.d.repository = &initialRootRepository{f.repo}
	f.checkpoint.epochs()
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(f.facts)).Return(CurrentSealedEpoch{}, nil).Once()
	expectProtectionReads(t, f.factory, f.consensus, f.facts)
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.facts))
	require.Zero(t, f.app.LastTournamentCheckBlock, "an empty initial observation cannot suppress same-head root discovery")
	require.Empty(t, f.repo.windows)
	f.consensus.AssertNotCalled(t, "TournamentLevelCount", mock.Anything)

	// The reader now publishes this head's first sealed epoch. Its computation
	// is still pending; no additional chain block is available.
	f.epoch.Status = model.EpochStatus_Closed
	f.epoch.LastBlock = f.facts.Number
	f.checkpoint.epochs(f.epoch)
	f.levels(1)
	address := *f.epoch.TournamentAddress
	f.repo.targets = []testTournamentScan{{
		EpochIndex: f.epoch.Index, Address: address, FromBlock: f.facts.Number, ThroughBlock: f.facts.Number,
	}}
	f.block = f.facts
	view := resultTestSnapshot(f.epoch, false)
	view.stage.IsFinished = false
	f.snapshot(view)
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil)
	opts := mock.MatchedBy(resultCallOptsAtBlock(f.facts.Number))
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{
		BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, Level: 0,
		Height: model.Log2EpochComputationHashLeafCount, StartInstant: f.facts.Number, Allowance: 300,
	}, nil).Twice()
	adapter.On("Standing", opts).Return(TournamentStanding{
		State: model.TournamentStandingAwaitingClosure, AcceptsJoins: true,
	}, nil).Twice()
	expectTournamentAuxiliaryReads(adapter, opts, RootLevel, model.TournamentStandingAwaitingClosure)
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == f.facts.Number && opts.End != nil && *opts.End == f.facts.Number
	})).Return(&TournamentEvents{}, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
	f.checkpoint.emptyParticipants(f.epoch, address)
	expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.facts))
	require.Len(t, f.repo.windows, 1)
	window := f.repo.windows[0]
	require.Zero(t, window.ExpectedPreviousCursor)
	require.Equal(t, f.repo.targets, window.CompletedScans)
	require.Len(t, window.Batches, 1)
	require.Equal(t, f.facts.Number, f.app.LastTournamentCheckBlock)
	require.Equal(t, model.EpochStatus_Closed, f.epoch.Status)

	// Local computation can finish at this same head. The published root now
	// supplies the prerequisite for an eligible join, without cursor repair.
	f.epoch.Status = model.EpochStatus_ClaimComputed
	adapter.On("CommitmentStanding", opts, [32]byte(*f.epoch.Commitment)).Return(CommitmentStanding{}, nil).Once()
	adapter.On("BondValue", opts).Return(big.NewInt(50), nil).Once()
	call, err := f.d.planAction(t.Context(), f.app, joinTournament, f.block, f.facts)
	require.NoError(t, err)
	require.NotNil(t, call)
	require.Equal(t, joinTournament, call.Key.Kind)
	require.Equal(t, f.epoch.Index, call.Key.Epoch)
	require.Equal(t, address, call.To)
	require.Equal(t, uint64(400), call.Deadline)
	require.Empty(t, f.backend.published)
	adapter.AssertExpectations(t)
}

func TestDeliveryDoesNotInventRootWithoutSealedEpochEvidence(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.repo.publishedRoots = nil
	require.NoError(t, f.d.observeWindow(t.Context(), f.app, nil, f.consensus, f.facts))
	require.Empty(t, f.repo.windows)
	require.Empty(t, f.backend.published)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
	f.consensus.AssertNotCalled(t, "TournamentLevelCount", mock.Anything)
}

func TestDeliveryInitialRootIsObservedAtItsSealingBlockWithoutLocalProof(t *testing.T) {
	statuses := []model.EpochStatus{
		model.EpochStatus_Closed, model.EpochStatus_InputsProcessed, model.EpochStatus_ClaimComputed,
	}
	for _, status := range statuses {
		for _, discoveredAfterList := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/target_discovered_after_list=%t", status, discoveredAfterList), func(t *testing.T) {
				f := newDeliveryIntervalFixture(t)
				f.epoch.LastBlock = f.facts.Number
				f.epoch.Status = status
				f.epoch.Commitment, f.epoch.MachineHash, f.epoch.TxBufferDataBlock = nil, nil, nil
				f.epoch.CommitmentProof = nil
				f.app.LastEpochCheckBlock = f.facts.Number - 1 // frozen reader does not cap this known root
				f.levels(1)
				address := *f.epoch.TournamentAddress
				f.repo.publishedRoots = nil
				loads := 1
				f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).
					Return((*model.Tournament)(nil), nil).Times(loads)
				adapter := &tournamentAdapterMock{}
				f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
				opts := mock.MatchedBy(resultCallOptsAtBlock(f.facts.Number))
				adapter.On("Descriptor", opts).Return(TournamentDescriptor{
					BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, Level: 0,
					Height: model.Log2EpochComputationHashLeafCount, StartInstant: f.facts.Number, Allowance: 300,
				}, nil).Once()
				adapter.On("Standing", opts).Return(TournamentStanding{
					State: model.TournamentStandingAwaitingClosure, AcceptsJoins: true,
				}, nil).Once()
				expectTournamentAuxiliaryReads(adapter, opts, RootLevel, model.TournamentStandingAwaitingClosure)
				adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
					return opts.Start == f.facts.Number && opts.End != nil && *opts.End == f.facts.Number
				})).Return(&TournamentEvents{}, nil).Once()
				adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
				f.checkpoint.emptyParticipants(f.epoch, address)
				f.repo.targets = []testTournamentScan{{
					EpochIndex: f.epoch.Index, Address: address, FromBlock: f.facts.Number, ThroughBlock: f.facts.Number,
				}}
				var epochs []*model.Epoch
				if discoveredAfterList {
					require.NoError(t, f.d.observeWindow(t.Context(), f.app, nil, f.consensus, f.facts))
				}
				{
					f.checkpoint.epochs(f.epoch)
					var err error
					epochs, _, err = getTournamentObservationEpochs(t.Context(), f.repo, f.app.Name)
					require.NoError(t, err)
					require.Len(t, epochs, 1, "observation selects a known root independently of local proof readiness")
				}
				require.NoError(t, f.d.observeWindow(t.Context(), f.app, epochs, f.consensus, f.facts))
				require.Len(t, f.repo.windows, 1)
				window := f.repo.windows[0]
				require.Equal(t, f.repo.targets, window.CompletedScans)
				require.Len(t, window.Batches, 1)
				require.Equal(t, address, window.Batches[0].Tournament.Address)
				require.Equal(t, f.facts.Number, window.Batches[0].Tournament.Snapshot.AsOfBlock)
				require.Equal(t, f.facts.Number, f.app.LastTournamentCheckBlock)
				require.Equal(t, status, f.epoch.Status)
				require.Empty(t, f.backend.published)
				adapter.AssertExpectations(t)
			})
		}
	}
}
