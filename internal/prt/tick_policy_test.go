// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPRTTickSharesObservationHeadAndKeepsActionHeadsFresh(t *testing.T) {
	s, repo := newPRTServiceMock()
	s.Context = t.Context()
	s.defaultBlock = model.DefaultBlock_Finalized
	s.submissionEnabled = true
	client := &ethClientMock{}
	factory := &adapterFactoryMock{}
	s.client, s.adapterFactory = client, factory
	var output bytes.Buffer
	s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	apps := []*model.Application{prtRevertTestApp(), prtRevertTestApp()}
	apps[1].ID++
	apps[1].Name = "delayed-app"
	apps[1].IApplicationAddress = common.HexToAddress("0x900")
	apps[1].IConsensusAddress = common.HexToAddress("0x901")
	apps[1].ClaimStagingPeriod = 300
	repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return(apps, uint64(2), nil).Twice()
	for i, app := range apps {
		consensus := &daveConsensusAdapterMock{}
		factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Twice()
		repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{HasTournament: new(true)}, repository.Pagination{}, false).
			Return([]*model.Epoch{}, uint64(0), nil).Twice()
		epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
		snapshot := resultTestSnapshot(epoch, false)
		// No tournament window is published while the EVM reader catches up.
		// Only the independent latest action snapshot can be read at this point.
		repo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).
			Return((*model.Epoch)(nil), nil).Twice()
		for tick := range 2 {
			opts := mock.MatchedBy(resultCallOptsAtBlock(120 + uint64(tick*2+i)))
			consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
			consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
			consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
		}
		t.Cleanup(func() { consensus.AssertExpectations(t) })
	}
	for tick := range 2 {
		client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
			Return(&types.Header{Number: big.NewInt(int64(100 + tick))}, nil).Once()
		for appIndex := range apps {
			client.On("BlockNumber", mock.Anything).Return(uint64(120+tick*2+appIndex), nil).Once()
		}
		require.Empty(t, s.Tick())
	}
	require.Equal(t, 1, strings.Count(output.String(), "Application has no claim staging delay"))
	require.Contains(t, output.String(), "without sentries")
	require.NotContains(t, output.String(), "application=delayed-app")
	require.Empty(t, s.pendingTransactions)
	client.AssertExpectations(t)
	factory.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestPRTTickWithoutApplicationsDoesNotReadHead(t *testing.T) {
	s, repo := newPRTServiceMock()
	s.Context = t.Context()
	s.client = &ethClientMock{}
	repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), nil).Once()
	require.Empty(t, s.Tick())
	require.Empty(t, s.client.(*ethClientMock).Calls)
	repo.AssertExpectations(t)
}

func TestPRTTickSuppressesHeadReadCancellationDuringShutdown(t *testing.T) {
	s, repo := newPRTServiceMock()
	s.Context = t.Context()
	s.defaultBlock = model.DefaultBlock_Finalized
	client := &ethClientMock{}
	s.client = client
	repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{prtRevertTestApp()}, uint64(1), nil).Once()
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Run(func(mock.Arguments) { s.SetStopping() }).Return((*types.Header)(nil), context.Canceled).Once()
	require.Empty(t, s.Tick())
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestPRTTickConfiguredHeadFailureDoesNotBlockForeclosedBondRecovery(t *testing.T) {
	live := prtRevertTestApp()
	foreclosed := prtForeclosedApp(live.ID+1, 100)
	foreclosed.LastEpochCheckBlock = 99 // Recovery precedes the incomplete-ingestion gate.
	foreclosed.IConsensusAddress = common.HexToAddress("0x800")
	owned := common.HexToAddress("0x600")
	factory := &adapterFactoryMock{}
	s := newRootBondTestService(owned, factory)
	s.Context = t.Context()
	s.defaultBlock = model.DefaultBlock_Finalized
	s.submissionEnabled = true
	repo := s.repository.(*prtRepositoryMock)
	repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{live, foreclosed}, uint64(2), nil).Once()
	tournamentAddress := common.HexToAddress("0x300")
	s.queueRootBondRecovery(foreclosed.ID, 3, tournamentAddress)
	client := &ethClientMock{}
	headError := errors.New("finalized head unavailable")
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return((*types.Header)(nil), headError).Once()
	client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
	s.client = client
	consensus := &daveConsensusAdapterMock{}
	consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(120))).
		Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
	factory.On("CreateDaveConsensusAdapter", foreclosed.IConsensusAddress).Return(consensus, nil).Once()
	tournament := &tournamentAdapterMock{}
	tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(120))).
		Return(canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), nil).Once()
	recoveryTx := types.NewTx(&types.LegacyTx{})
	tournament.On("TryRecoveringBond", mock.Anything).Return(recoveryTx, nil).Once()
	factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()

	errs := s.Tick()
	require.Len(t, errs, 2, "each application reports its failed passive observation")
	for _, err := range errs {
		require.ErrorIs(t, err, headError)
	}
	require.Equal(t, recoveryTx.Hash(), *s.rootBondRecoveries[foreclosed.ID][0].TxHash)
	client.AssertExpectations(t)
	consensus.AssertExpectations(t)
	tournament.AssertExpectations(t)
	factory.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestPRTTickPublishesObservationBeforePendingBondMaintenance(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.s.Context = t.Context()
	f.s.defaultBlock = model.DefaultBlock_Finalized
	f.s.submissionEnabled = true
	epoch := checkpointEpoch(3, "0x100")
	epoch.Status = model.EpochStatus_ClaimAccepted
	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
	f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{f.app}, uint64(1), nil).Once()
	f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
	f.client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
	published := false
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID,
		mock.MatchedBy(func(batches []*repository.TournamentEventBatch) bool {
			return len(batches) == 1 && batches[0].Tournament.Address == *epoch.TournamentAddress
		}), uint64(100)).Run(func(mock.Arguments) { published = true }).Return(nil).Once()
	transaction := types.NewTx(&types.LegacyTx{Nonce: 1})
	hash := transaction.Hash()
	f.s.rootBondRecoveries[f.app.ID] = []*rootBondRecovery{{
		EpochIndex: epoch.Index, Tournament: *epoch.TournamentAddress, TxHash: &hash,
	}}
	f.client.On("TransactionByHash", mock.Anything, hash).
		Run(func(mock.Arguments) {
			require.True(t, published, "a pending refund must not stop passive state publication")
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
		}).Return(transaction, true, nil).Once()

	require.Empty(t, f.s.Tick())
	require.True(t, published)
	require.Equal(t, hash, *f.s.rootBondRecoveries[f.app.ID][0].TxHash)
	require.Empty(t, f.s.pendingTransactions, "the observer must not submit tournament actions")
}
