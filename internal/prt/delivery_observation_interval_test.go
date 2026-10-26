// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

type deliveryIntervalFixture struct {
	*deliveryFixture
	checkpoint *observerCheckpointFixture
	facts      txsender.BlockRef
}

func newDeliveryIntervalFixture(t *testing.T) *deliveryIntervalFixture {
	t.Helper()
	f := newDeliveryFixture(t)
	f.app.LastTournamentCheckBlock = 50
	f.app.LastEpochCheckBlock = 40 // The EVM Reader has stopped before this tournament interval.
	f.epoch.LastBlock = 10
	client := &ethClientMock{}
	header := &types.Header{Number: big.NewInt(100)}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(header, nil).Maybe()
	f.d.client = client
	checkpoint := &observerCheckpointFixture{t: t, s: f.d.Service, repo: f.repo.prtRepositoryMock,
		factory: f.factory, consensus: f.consensus, client: client, app: f.app}
	t.Cleanup(func() {
		f.repo.AssertExpectations(t)
		f.factory.AssertExpectations(t)
		f.consensus.AssertExpectations(t)
		client.AssertExpectations(t)
	})
	return &deliveryIntervalFixture{f, checkpoint, txsender.BlockRef{Number: 100, Hash: header.Hash()}}
}

func (f *deliveryIntervalFixture) levels(levels uint64) {
	f.checkpoint.t.Helper()
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(f.facts.Number))).
		Return(levels, nil).Once()
}

func TestDeliveryIntervalIncludesEmptyRootAndChildScans(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.levels(2)
	root := f.checkpoint.tournament(f.epoch, *f.epoch.TournamentAddress, 0, 2, 90, 100, &TournamentEvents{}, nil)
	childAddress := common.HexToAddress("0x101")
	child := f.checkpoint.tournament(f.epoch, childAddress, 1, 2, 88, 100, &TournamentEvents{}, nil)
	f.checkpoint.children(f.epoch, root, child)
	f.repo.targets = []testTournamentScan{
		{EpochIndex: f.epoch.Index, Address: root.Address, FromBlock: 51, ThroughBlock: 100},
		{EpochIndex: f.epoch.Index, Address: child.Address, FromBlock: 51, ThroughBlock: 100},
	}
	require.NoError(t, f.d.observeWindow(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, f.facts))
	require.Len(t, f.repo.windows, 2, "stored parent and child commit their own independent intervals")
	var scans []testTournamentScan
	for _, interval := range f.repo.windows {
		require.Equal(t, f.app.ID, interval.ApplicationID)
		require.Equal(t, uint64(50), interval.ExpectedPreviousCursor)
		require.Equal(t, uint64(100), interval.ThroughBlock)
		require.Len(t, interval.Batches, 1)
		scans = append(scans, interval.CompletedScans...)
	}
	require.ElementsMatch(t, f.repo.targets, scans)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)

}

func TestDeliveryIntervalIncludesNewRootHistoryBeforeCursor(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.repo.publishedRoots = nil
	f.levels(1)
	address := *f.epoch.TournamentAddress
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).
		Return((*model.Tournament)(nil), nil).Once()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
	opts := mock.MatchedBy(resultCallOptsAtBlock(100))
	adapter.On("Descriptor", opts).Return(TournamentDescriptor{
		BaseCycle: big.NewInt(0), Kind: model.TournamentKindLeaf, StartInstant: f.epoch.LastBlock,
	}, nil).Once()
	adapter.On("Standing", opts).Return(TournamentStanding{
		State: model.TournamentStandingRootWinner, HasCandidate: true, Candidate: *f.epoch.Commitment,
		FinalState: *f.epoch.MachineHash, FinishedAt: 49,
	}, nil).Once()
	expectTournamentAuxiliaryReads(adapter, opts, RootLevel, model.TournamentStandingRootWinner)
	adapter.On("RetrieveAllEvents", mock.MatchedBy(func(opts *bind.FilterOpts) bool {
		return opts.Start == 10 && opts.End != nil && *opts.End == 100
	})).Return(&TournamentEvents{}, nil).Once()
	adapter.On("StructuralEventCounts", opts).Return(zeroStructuralEventCounts(), nil).Once()
	f.checkpoint.emptyParticipants(f.epoch, address)
	f.repo.targets = []testTournamentScan{{
		EpochIndex: f.epoch.Index, Address: address, FromBlock: 10, ThroughBlock: 100,
	}}
	require.NoError(t, f.d.observeWindow(t.Context(), f.app, []*model.Epoch{f.epoch}, f.consensus, f.facts))
	require.Len(t, f.repo.windows, 1)
	require.Zero(t, f.repo.windows[0].ExpectedPreviousCursor, "new root has no previous own cursor")
	require.Equal(t, f.repo.targets, f.repo.windows[0].CompletedScans)
	require.Len(t, f.repo.windows[0].Batches, 1)
	require.Equal(t, address, f.repo.windows[0].Batches[0].Tournament.Address)
	adapter.AssertExpectations(t)
}

func TestDeliveryIntervalReadsRequiredChildOutsideRootList(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.levels(2)
	address := common.HexToAddress("0x102")
	child := f.checkpoint.tournament(f.epoch, address, 1, 2, 88, 100, &TournamentEvents{}, nil)
	child.ParentTournamentAddress = f.epoch.TournamentAddress
	child.ParentMatchIDHash = new(common.HexToHash("0xabc"))
	f.repo.targets = []testTournamentScan{{
		EpochIndex: f.epoch.Index, Address: address, FromBlock: 51, ThroughBlock: 100,
	}}
	require.NoError(t, f.d.observeWindow(t.Context(), f.app, nil, f.consensus, f.facts))
	require.Len(t, f.repo.windows, 1)
	require.Equal(t, f.repo.targets, f.repo.windows[0].CompletedScans)
	require.Len(t, f.repo.windows[0].Batches, 1)
	require.Equal(t, address, f.repo.windows[0].Batches[0].Tournament.Address)
}

func TestDeliveryIntervalMissingRequiredChildDoesNotCommitPrefix(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.levels(2)
	f.repo.publishedRoots = nil
	address := common.HexToAddress("0x103")
	f.repo.storedTournaments = []*model.Tournament{{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index, Address: address,
		Level: 1, MaxLevel: 2, ParentTournamentAddress: f.epoch.TournamentAddress, Snapshot: model.TournamentSnapshot{AsOfBlock: 50}}}
	f.repo.targets = []testTournamentScan{{
		EpochIndex: f.epoch.Index, Address: address, FromBlock: 51, ThroughBlock: 100,
	}}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).
		Return((*model.Tournament)(nil), repository.ErrNotFound).Once()
	require.ErrorIs(t, f.d.observeWindow(t.Context(), f.app, nil, f.consensus, f.facts), repository.ErrNotFound)
	require.Empty(t, f.repo.windows)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
}

func TestDeliveryIntervalRejectsChangedHashBeforeScanning(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.d.defaultBlock = model.DefaultBlock_Latest
	f.facts.Hash = common.HexToHash("0xBAD")
	f.repo.publishedRoots = nil
	f.repo.storedTournaments = []*model.Tournament{{ApplicationID: f.app.ID, EpochIndex: f.epoch.Index,
		Address: *f.epoch.TournamentAddress, Snapshot: model.TournamentSnapshot{AsOfBlock: 50}}}
	require.ErrorIs(t, f.d.observeWindow(t.Context(), f.app, nil, f.consensus, f.facts), txsender.ErrStale)
	require.Empty(t, f.repo.windows)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
	f.consensus.AssertNotCalled(t, "TournamentLevelCount", mock.Anything)
}

func TestDeliveryIntervalFailedPublicationPreservesCursor(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.levels(1)
	f.checkpoint.tournament(f.epoch, *f.epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.windowError = repository.ErrTournamentIntervalStale
	require.ErrorIs(t, f.d.observeWindow(t.Context(), f.app, nil, f.consensus, f.facts), repository.ErrTournamentIntervalStale)
	require.Len(t, f.repo.windows, 1)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
}

func TestDeliveryObservesEnabledTerminalApplicationButStopsDisabledApplication(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled terminal"
		}
		t.Run(name, func(t *testing.T) {
			f := newDeliveryIntervalFixture(t)
			f.app.Enabled = enabled
			f.app.Status = model.ApplicationStatus_Failed
			f.app.ForecloseBlock = 30
			if !enabled {
				require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.facts))
				require.Empty(t, f.repo.windows)
				require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
				f.repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				return
			}
			f.checkpoint.epochs(f.epoch)
			f.levels(1)
			f.checkpoint.tournament(f.epoch, *f.epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
			expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
			require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.facts))
			require.Len(t, f.repo.windows, 1)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
			require.Equal(t, uint64(40), f.app.LastEpochCheckBlock)
			require.Equal(t, model.ApplicationStatus_Failed, f.app.Status)
			require.Empty(t, f.backend.published, "observation does not publish actions for a terminal application")
		})
	}
}

func TestDeliveryDisabledApplicationStopsAndClearsObservationFailure(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	f.app.Enabled = false
	f.app.Status = model.ApplicationStatus_Failed
	f.d.observationFailures[f.app.ID] = tournamentObservationFailure{failedHeads: tournamentObservationFailureThreshold}
	err := f.d.observeApplication(t.Context(), f.app, f.facts)
	require.NoError(t, err)
	require.NotContains(t, f.d.observationFailures, f.app.ID)
	require.Empty(t, f.repo.windows)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
	f.repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
