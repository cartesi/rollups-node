// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type foreclosedBondFixture struct {
	s          *Service
	repo       *prtRepositoryMock
	client     *ethClientMock
	consensus  *daveConsensusAdapterMock
	tournament *tournamentAdapterMock
	app        *model.Application
	epoch      *model.Epoch
	projection *model.Tournament
	sealed     CurrentSealedEpoch
	owned      common.Address
}

func newForeclosedBondFixture(t *testing.T, disposition model.BondDisposition) *foreclosedBondFixture {
	t.Helper()
	factory := &adapterFactoryMock{}
	f := &foreclosedBondFixture{
		app: prtForeclosedApp(1, 100), epoch: resultTestEpoch(model.EpochStatus_ClaimStaged),
		client: &ethClientMock{}, consensus: &daveConsensusAdapterMock{}, tournament: &tournamentAdapterMock{},
		owned: common.HexToAddress("0x600"),
	}
	f.app.LastTournamentCheckBlock = 120
	f.s = newRootBondTestService(f.owned, factory)
	f.s.client, f.s.submissionEnabled = f.client, true
	f.repo = f.s.repository.(*prtRepositoryMock)
	f.sealed = CurrentSealedEpoch{EpochNumber: f.epoch.Index, Tournament: *f.epoch.TournamentAddress}
	f.projection = &model.Tournament{
		ApplicationID: f.app.ID, EpochIndex: f.epoch.Index, Address: *f.epoch.TournamentAddress,
		Snapshot: model.TournamentSnapshot{
			AsOfBlock: 120, FinishedAtBlock: 110, WinnerCommitment: f.epoch.Commitment,
			BondRecovery: model.TournamentBondRecovery{Disposition: disposition, Claimer: &f.owned},
		},
	}
	factory.On("CreateDaveConsensusAdapter", f.app.IConsensusAddress).Return(f.consensus, nil)
	t.Cleanup(func() {
		f.repo.AssertExpectations(t)
		f.client.AssertExpectations(t)
		f.consensus.AssertExpectations(t)
		f.tournament.AssertExpectations(t)
		factory.AssertExpectations(t)
	})
	return f
}

func (f *foreclosedBondFixture) expectObservedTick(unreconciled bool, loadProjection bool) {
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
		repository.Pagination{}, false).Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
	f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(120))).Return(f.sealed, nil).Once()
	if loadProjection {
		f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.TournamentAddress.Hex()).
			Return(f.projection, nil).Once()
	}
	f.repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, f.app.ID, f.app.ForecloseBlock).Return(false, nil).Once()
	f.repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, f.app.ID, f.app.ForecloseBlock).
		Return(unreconciled, nil).Once()
	if unreconciled {
		f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.TournamentAddress.Hex()).
			Return(f.projection, nil).Once()
		f.repo.On("ListEpochs", mock.Anything, f.app.Name,
			repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false).
			Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
		f.repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, f.app.ID, f.epoch.Index).
			Run(func(mock.Arguments) { f.epoch.Status = model.EpochStatus_ClaimForeclosed }).Return(nil).Once()
	}
}

func (f *foreclosedBondFixture) expectRecoveryBroadcast(tx *types.Transaction) {
	f.client.On("BlockNumber", mock.Anything).Return(uint64(125), nil).Once()
	f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(125))).Return(f.sealed, nil).Once()
	f.s.adapterFactory.(*adapterFactoryMock).On("CreateTournamentAdapter", f.projection.Address).Return(f.tournament, nil).Once()
	f.tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(125))).
		Return(canonicalBondRecovery(model.BondDispositionRecoverable, f.owned, 1), nil).Once()
	f.tournament.On("TryRecoveringBond", mock.Anything).Return(tx, nil).Once()
}

func (f *foreclosedBondFixture) tick(t *testing.T) {
	t.Helper()
	require.NoError(t, f.s.handleForeclosedApp(t.Context(), f.app, func() (uint64, error) { return 120, nil }))
	// Foreclosure permits bond recovery, never another join/stage/accept.
	require.Empty(t, f.s.pendingTransactions)
	f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
}

func TestForeclosedRootBondDiscoveryWithoutAcceptAttempt(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimStaged, model.EpochStatus_ClaimForeclosed} {
		t.Run(status.String(), func(t *testing.T) {
			f := newForeclosedBondFixture(t, model.BondDispositionRecoverable)
			f.epoch.Status = status
			f.expectObservedTick(status == model.EpochStatus_ClaimStaged, true)
			tx := types.NewTx(&types.LegacyTx{Nonce: 1})
			f.expectRecoveryBroadcast(tx)
			require.Empty(t, f.s.rootBondRecoveries)
			f.tick(t)
			require.Len(t, f.s.rootBondRecoveries[f.app.ID], 1)
			require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID][0].TxHash)
			require.Equal(t, model.EpochStatus_ClaimForeclosed, f.epoch.Status)
		})
	}
}

func TestForeclosedRunningRootBecomesRecoverableAfterClaimDrain(t *testing.T) {
	f := newForeclosedBondFixture(t, model.BondDispositionTournamentRunning)
	f.projection.Snapshot.FinishedAtBlock = 0
	f.expectObservedTick(true, true)
	f.tick(t)
	require.Empty(t, f.s.rootBondRecoveries)
	require.Empty(t, f.s.discoveredForeclosedRootBonds)
	require.Equal(t, model.EpochStatus_ClaimForeclosed, f.epoch.Status)

	f.projection.Snapshot.FinishedAtBlock = 115
	f.projection.Snapshot.BondRecovery.Disposition = model.BondDispositionRecoverable
	f.expectObservedTick(false, true)
	f.expectRecoveryBroadcast(types.NewTx(&types.LegacyTx{Nonce: 1}))
	f.tick(t)
	require.Len(t, f.s.rootBondRecoveries[f.app.ID], 1)
}

func TestForeclosedRootDiscoveryDoesNotQueueIneligibleBonds(t *testing.T) {
	for _, test := range []struct {
		name   string
		update func(*foreclosedBondFixture)
	}{
		{name: "foreign claimer", update: func(f *foreclosedBondFixture) {
			f.projection.Snapshot.BondRecovery.Claimer = new(common.HexToAddress("0x700"))
		}},
		{name: "recovered", update: func(f *foreclosedBondFixture) {
			f.projection.Snapshot.BondRecovery.Disposition = model.BondDispositionRecovered
		}},
		{name: "no winner", update: func(f *foreclosedBondFixture) {
			f.projection.Snapshot.BondRecovery.Disposition = model.BondDispositionNoWinner
		}},
		{name: "projection beyond published cursor", update: func(f *foreclosedBondFixture) {
			f.projection.Snapshot.AsOfBlock = 121
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newForeclosedBondFixture(t, model.BondDispositionRecoverable)
			f.epoch.Status = model.EpochStatus_ClaimForeclosed
			test.update(f)
			for range 2 {
				f.expectObservedTick(false, true)
				f.tick(t)
			}
			require.Empty(t, f.s.rootBondRecoveries)
			require.Empty(t, f.s.discoveredForeclosedRootBonds)
			f.client.AssertNotCalled(t, "BlockNumber", mock.Anything)
		})
	}
}

func TestForeclosedRootDiscoveryDoesNotRetryFailedPaymentPush(t *testing.T) {
	for _, queuedBeforeForeclosure := range []bool{false, true} {
		name := "discovered after foreclosure"
		if queuedBeforeForeclosure {
			name = "queued before foreclosure with observation lag"
		}
		t.Run(name, func(t *testing.T) {
			f := newForeclosedBondFixture(t, model.BondDispositionRecoverable)
			f.epoch.Status = model.EpochStatus_ClaimForeclosed
			if queuedBeforeForeclosure {
				f.s.queueRootBondRecovery(f.app.ID, f.epoch.Index, f.projection.Address)
				f.projection.Snapshot.BondRecovery.Disposition = model.BondDispositionTournamentRunning
			}
			tx := types.NewTx(&types.LegacyTx{Nonce: 1})
			f.expectObservedTick(false, true)
			f.expectRecoveryBroadcast(tx)
			f.tick(t)

			f.expectObservedTick(false, queuedBeforeForeclosure)
			f.client.On("BlockNumber", mock.Anything).Return(uint64(125), nil).Once()
			f.client.On("TransactionByHash", mock.Anything, tx.Hash()).Return(tx, false, nil).Once()
			f.client.On("TransactionReceipt", mock.Anything, tx.Hash()).Return(&types.Receipt{
				TxHash: tx.Hash(), BlockNumber: big.NewInt(125), Status: types.ReceiptStatusSuccessful,
			}, nil).Once()
			f.s.adapterFactory.(*adapterFactoryMock).On("CreateTournamentAdapter", f.projection.Address).Return(f.tournament, nil).Once()
			f.tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(125))).
				Return(canonicalBondRecovery(model.BondDispositionRecoverable, f.owned, 1), nil).Once()
			f.tick(t)
			require.Empty(t, f.s.rootBondRecoveries)

			f.projection.Snapshot.BondRecovery.Disposition = model.BondDispositionRecoverable
			f.expectObservedTick(false, false)
			f.tick(t)
			f.tournament.AssertNumberOfCalls(t, "TryRecoveringBond", 1)
			require.Empty(t, f.s.rootBondRecoveries)
		})
	}
}

func TestForeclosedRootDiscoveryRequiresCurrentRootIdentity(t *testing.T) {
	for _, mismatch := range []string{"epoch", "tournament"} {
		t.Run(mismatch, func(t *testing.T) {
			f := newForeclosedBondFixture(t, model.BondDispositionRecoverable)
			f.epoch.Status = model.EpochStatus_ClaimForeclosed
			if mismatch == "epoch" {
				f.sealed.EpochNumber++
			} else {
				f.sealed.Tournament = common.HexToAddress("0x900")
			}
			f.expectObservedTick(false, false)
			f.tick(t)
			require.Empty(t, f.s.rootBondRecoveries)
			require.Empty(t, f.s.discoveredForeclosedRootBonds)
		})
	}
}

func TestForeclosedRootDiscoveryRetriesReadErrors(t *testing.T) {
	for _, failure := range []string{"current epoch", "published root"} {
		t.Run(failure, func(t *testing.T) {
			f := newForeclosedBondFixture(t, model.BondDispositionRecoverable)
			f.epoch.Status = model.EpochStatus_ClaimForeclosed
			readErr := errors.New("discovery read failed")
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
				repository.Pagination{}, false).Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
			var chainErr error
			if failure == "current epoch" {
				chainErr = readErr
			} else {
				f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), f.projection.Address.Hex()).
					Return((*model.Tournament)(nil), readErr).Once()
			}
			f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(120))).Return(f.sealed, chainErr).Once()
			f.repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, f.app.ID, f.app.ForecloseBlock).Return(false, nil).Once()
			f.repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, f.app.ID, f.app.ForecloseBlock).Return(false, nil).Once()
			require.ErrorIs(t, f.s.handleForeclosedApp(t.Context(), f.app, func() (uint64, error) { return 120, nil }), readErr)
			require.Empty(t, f.s.discoveredForeclosedRootBonds, "a read failure must not consume discovery")
			require.Empty(t, f.s.rootBondRecoveries)

			f.expectObservedTick(false, true)
			f.expectRecoveryBroadcast(types.NewTx(&types.LegacyTx{Nonce: 1}))
			f.tick(t)
			require.Len(t, f.s.rootBondRecoveries[f.app.ID], 1)
		})
	}
}
