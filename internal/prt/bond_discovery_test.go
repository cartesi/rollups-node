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

type publishedBondFixture struct {
	s       *Service
	repo    *prtRepositoryMock
	client  *ethClientMock
	factory *adapterFactoryMock
	app     *model.Application
	owned   common.Address
}

func newPublishedBondFixture(t *testing.T) *publishedBondFixture {
	t.Helper()
	f := &publishedBondFixture{
		factory: &adapterFactoryMock{}, client: &ethClientMock{},
		app: prtRevertTestApp(), owned: common.HexToAddress("0x600"),
	}
	f.app.LastTournamentCheckBlock = 100
	f.s = newRootBondTestService(f.owned, f.factory)
	f.s.submissionEnabled, f.s.client = true, f.client
	f.repo = f.s.repository.(*prtRepositoryMock)
	t.Cleanup(func() {
		f.repo.AssertExpectations(t)
		f.client.AssertExpectations(t)
		f.factory.AssertExpectations(t)
	})
	return f
}

func bondCandidate(epoch uint64) repository.RootBondRecoveryCandidate {
	return repository.RootBondRecoveryCandidate{
		EpochIndex: epoch, Tournament: common.BigToAddress(new(big.Int).SetUint64(epoch + 1)),
	}
}

func (f *publishedBondFixture) expectBoundary(epoch uint64) {
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).
		Return(new(epoch), nil).Once()
}

func (f *publishedBondFixture) expectPage(observed uint64, after *uint64, through uint64,
	rows []repository.RootBondRecoveryCandidate, err error,
) {
	f.repo.On("ListRecoverableRootBonds", mock.Anything, f.app.ID, f.owned,
		observed, after, through, uint64(16)).Return(rows, err).Once()
}

func (f *publishedBondFixture) expectPreflight(t *testing.T, candidate repository.RootBondRecoveryCandidate,
	recovery BondRecovery, err error,
) *tournamentAdapterMock {
	t.Helper()
	tournament := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", candidate.Tournament).Return(tournament, nil).Once()
	tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(120))).Return(recovery, err).Once()
	t.Cleanup(func() { tournament.AssertExpectations(t) })
	return tournament
}

func (f *publishedBondFixture) expectBroadcast(t *testing.T, candidate repository.RootBondRecoveryCandidate) *types.Transaction {
	t.Helper()
	tx := types.NewTx(&types.LegacyTx{Nonce: candidate.EpochIndex})
	tournament := f.expectPreflight(t, candidate, canonicalBondRecovery(model.BondDispositionRecoverable, f.owned, 1), nil)
	tournament.On("TryRecoveringBond", mock.Anything).Return(tx, nil).Once()
	return tx
}

func TestPublishedRecoveryRediscoversAcceptedEpochWithoutLocalCommitment(t *testing.T) {
	for _, foreclosed := range []bool{false, true} {
		name := "live"
		if foreclosed {
			name = "foreclosed"
		}
		t.Run(name, func(t *testing.T) {
			f := newPublishedBondFixture(t)
			if foreclosed {
				f.app.ForecloseBlock = 90
				f.app.LastEpochCheckBlock, f.app.LastInputCheckBlock = 100, 100
			}
			// Fresh service, accepted epoch, and no local commitment or join record.
			f.expectBoundary(3)
			candidate := bondCandidate(0)
			f.expectPage(100, nil, 3, []repository.RootBondRecoveryCandidate{candidate}, nil)
			tx := f.expectBroadcast(t, candidate)
			require.Empty(t, f.s.rootBondRecoveries)
			require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 110, 120))
			require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID].TxHash)
			f.repo.AssertNotCalled(t, "GetCommitment", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
		})
	}
}

func TestPublishedRecoveryBoundsPagesAndWrapsFiniteSweep(t *testing.T) {
	f := newPublishedBondFixture(t)
	f.expectBoundary(16)
	rows := make([]repository.RootBondRecoveryCandidate, 16)
	for i := range rows {
		rows[i] = bondCandidate(uint64(i))
		f.expectPreflight(t, rows[i], canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil)
	}
	f.expectPage(100, nil, 16, rows, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Equal(t, uint64(15), *f.s.rootBondRecoveryScans[f.app.ID].AfterEpoch)
	require.Empty(t, f.s.rootBondRecoveries, "already paid rows must not build an unsent queue")

	// A newer observation must not reset page one or extend this sweep.
	f.app.LastTournamentCheckBlock = 110
	f.expectPage(110, new(uint64(15)), 16, []repository.RootBondRecoveryCandidate{bondCandidate(16)}, nil)
	f.expectPreflight(t, bondCandidate(16), canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 110, 120))
	require.Empty(t, f.s.rootBondRecoveryScans)

	// Once publication catches up, the next sweep can revisit epoch zero if
	// the published state still says recoverable (for example, after a reorg).
	f.app.LastTournamentCheckBlock = 120
	f.expectBoundary(25)
	f.expectPage(120, nil, 25, []repository.RootBondRecoveryCandidate{bondCandidate(0)}, nil)
	tx := f.expectBroadcast(t, bondCandidate(0))
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 120, 120))
	require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID].TxHash)
	require.Empty(t, f.s.failedRootBondPayments, "a latest RECOVERED view is not permanent suppression")
}

func TestPublishedRecoverySkipsSuppressedPageWithoutStarvingLaterRoot(t *testing.T) {
	f := newPublishedBondFixture(t)
	f.expectBoundary(16)
	f.s.failedRootBondPayments = make(map[rootBondRecoveryKey]struct{})
	rows := make([]repository.RootBondRecoveryCandidate, 16)
	for i := range rows {
		rows[i] = bondCandidate(uint64(i))
		f.s.failedRootBondPayments[rootBondRecoveryKey{f.app.ID, rows[i].Tournament}] = struct{}{}
	}
	f.expectPage(100, nil, 16, rows, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Empty(t, f.factory.Calls)
	require.Empty(t, f.s.rootBondRecoveries)
	last := bondCandidate(16)
	f.expectPage(100, new(uint64(15)), 16, []repository.RootBondRecoveryCandidate{last}, nil)
	tx := f.expectBroadcast(t, last)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID].TxHash)
}

func TestPublishedRecoveryTemporaryPreflightFailureDefersOnlyThatRoot(t *testing.T) {
	f := newPublishedBondFixture(t)
	first, second := bondCandidate(0), bondCandidate(1)
	f.expectBoundary(1)
	f.expectPage(100, nil, 1, []repository.RootBondRecoveryCandidate{first, second}, nil)
	rpcErr := errors.New("provider unavailable")
	f.expectPreflight(t, first, BondRecovery{}, rpcErr)
	require.ErrorIs(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120), rpcErr)
	require.Empty(t, f.s.rootBondRecoveries)
	require.Empty(t, f.s.failedRootBondPayments)
	f.expectPage(100, new(uint64(0)), 1, []repository.RootBondRecoveryCandidate{second}, nil)
	f.expectPreflight(t, second, canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Empty(t, f.s.rootBondRecoveryScans)
	f.expectBoundary(1)
	f.expectPage(100, nil, 1, []repository.RootBondRecoveryCandidate{first}, nil)
	f.expectBroadcast(t, first)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
}

func TestPublishedRecoveryUsesLowerObservationBoundAndRetriesQueryErrors(t *testing.T) {
	f := newPublishedBondFixture(t)
	f.expectBoundary(3)
	queryErr := errors.New("database unavailable")
	f.expectPage(80, nil, 3, nil, queryErr)
	require.ErrorIs(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 80, 120), queryErr)
	require.Nil(t, f.s.rootBondRecoveryScans[f.app.ID].AfterEpoch)
	// A regressed head changes the query bound, not the scan's start position.
	f.expectPage(70, nil, 3, nil, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 70, 120))
	require.Empty(t, f.s.rootBondRecoveryScans)
	f.expectBoundary(3)
	f.expectPage(100, nil, 3, []repository.RootBondRecoveryCandidate{bondCandidate(3)}, nil)
	f.expectBroadcast(t, bondCandidate(3))
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 110, 120))
}

func TestPublishedRecoveryEmptyOrUnavailableSweepBoundary(t *testing.T) {
	for _, readErr := range []error{nil, errors.New("epoch read failed")} {
		name := "no sealed epoch"
		if readErr != nil {
			name = "query failed"
		}
		t.Run(name, func(t *testing.T) {
			f := newPublishedBondFixture(t)
			f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, f.app.IApplicationAddress.Hex()).
				Return((*uint64)(nil), readErr).Once()
			err := f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120)
			if readErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, readErr)
			}
			require.Empty(t, f.s.rootBondRecoveryScans)
			require.Empty(t, f.s.rootBondRecoveries)
			require.Empty(t, f.factory.Calls)
		})
	}
}

func TestPublishedRecoverySuppressionIsApplicationScoped(t *testing.T) {
	f := newPublishedBondFixture(t)
	candidate := bondCandidate(0)
	f.s.failedRootBondPayments = map[rootBondRecoveryKey]struct{}{{f.app.ID + 1, candidate.Tournament}: {}}
	f.expectBoundary(0)
	f.expectPage(100, nil, 0, []repository.RootBondRecoveryCandidate{candidate}, nil)
	tx := f.expectBroadcast(t, candidate)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID].TxHash)
}

func TestPublishedRecoverySkipsDiscoveryWithoutPublishedEvidence(t *testing.T) {
	for _, test := range []struct {
		name     string
		update   func(*model.Application)
		observed uint64
	}{
		{name: "zero head", observed: 0},
		{name: "zero publication cursor", observed: 100, update: func(app *model.Application) { app.LastTournamentCheckBlock = 0 }},
		{name: "foreclosure ahead of head", observed: 80, update: func(app *model.Application) {
			app.ForecloseBlock, app.LastEpochCheckBlock, app.LastInputCheckBlock = 90, 100, 100
		}},
		{name: "foreclosure inputs not scanned", observed: 100, update: func(app *model.Application) {
			app.ForecloseBlock, app.LastEpochCheckBlock, app.LastInputCheckBlock = 90, 100, 89
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newPublishedBondFixture(t)
			if test.update != nil {
				test.update(f.app)
			}
			require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, test.observed, 120))
			require.Empty(t, f.repo.Calls)
			require.Empty(t, f.factory.Calls)
		})
	}
}

func TestPublishedRecoveryMaintainsPendingTransactionBeforeDiscovery(t *testing.T) {
	f := newPublishedBondFixture(t)
	tx := types.NewTx(&types.LegacyTx{Nonce: 1})
	f.s.rootBondRecoveries[f.app.ID] = &rootBondRecovery{
		EpochIndex: 0, Tournament: bondCandidate(0).Tournament, TxHash: new(tx.Hash()),
	}
	f.client.On("TransactionByHash", mock.Anything, tx.Hash()).Return(tx, true, nil).Once()
	// A zero bound models observation failure during foreclosure. No query may
	// run; an unavailable repository must not block maintenance of a sent hash.
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 0, 120))
	require.Empty(t, f.repo.Calls)
	require.Empty(t, f.factory.Calls)
	require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID].TxHash)
}

func TestPublishedRecoveryHealthAndSubmissionGates(t *testing.T) {
	for _, status := range model.ApplicationStatusAllValues {
		if status == model.ApplicationStatus_OK {
			continue
		}
		t.Run(status.String(), func(t *testing.T) {
			f := newPublishedBondFixture(t)
			f.app.Status = status
			require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
			require.Empty(t, f.repo.Calls)
			require.Empty(t, f.factory.Calls)
		})
	}
	t.Run("reader mode", func(t *testing.T) {
		f := newPublishedBondFixture(t)
		f.s.submissionEnabled = false
		require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
		require.Empty(t, f.repo.Calls)
		require.Empty(t, f.factory.Calls)
	})
}

func TestPublishedRecoveryFailedPushIsSuppressedAcrossForeclosureAndAcceptance(t *testing.T) {
	f := newPublishedBondFixture(t)
	candidate := bondCandidate(0)
	f.expectBoundary(0)
	f.expectPage(100, nil, 0, []repository.RootBondRecoveryCandidate{candidate}, nil)
	tx := f.expectBroadcast(t, candidate)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	f.client.On("TransactionByHash", mock.Anything, tx.Hash()).Return(tx, false, nil).Once()
	f.client.On("TransactionReceipt", mock.Anything, tx.Hash()).Return(&types.Receipt{
		TxHash: tx.Hash(), BlockNumber: big.NewInt(120), Status: types.ReceiptStatusSuccessful,
	}, nil).Once()
	f.expectPreflight(t, candidate, canonicalBondRecovery(model.BondDispositionRecoverable, f.owned, 1), nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Empty(t, f.s.rootBondRecoveries)
	require.Contains(t, f.s.failedRootBondPayments, rootBondRecoveryKey{f.app.ID, candidate.Tournament})

	// End the interrupted sweep, then revisit the still-recoverable row after
	// acceptance advances the epoch. Foreclosure must not bypass suppression.
	f.expectPage(100, new(uint64(0)), 0, nil, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	f.expectBoundary(1)
	f.expectPage(100, nil, 1, []repository.RootBondRecoveryCandidate{candidate}, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	f.app.ForecloseBlock, f.app.LastEpochCheckBlock, f.app.LastInputCheckBlock = 90, 100, 100
	f.expectBoundary(1)
	f.expectPage(100, nil, 1, []repository.RootBondRecoveryCandidate{candidate}, nil)
	require.NoError(t, f.s.recoverPublishedRootBonds(t.Context(), f.app, 100, 120))
	require.Empty(t, f.s.rootBondRecoveries)
}
