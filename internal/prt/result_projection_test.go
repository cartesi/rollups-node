// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFailedRootDoesNotBlockOlderOwnedBondRecovery(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	snapshot := resultTestSnapshot(f.epoch, false)
	snapshot.stage.IsTournamentFailed = true
	snapshot.stage.WinnerCommitment = common.Hash{}
	snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
	f.expectTick(100, 100, snapshot, snapshot)
	olderTournament := common.HexToAddress("0x900")
	f.s.queueRootBondRecovery(f.app.ID, f.epoch.Index-1, olderTournament)
	f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(snapshot.sealed, nil).Once()
	tournament := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", olderTournament).Return(tournament, nil).Once()
	tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(100))).
		Return(canonicalBondRecovery(model.BondDispositionRecoverable, f.s.txOptsFactory.From(), 1), nil).Once()
	tx := types.NewTx(&types.LegacyTx{Nonce: 1})
	tournament.On("TryRecoveringBond", mock.Anything).Return(tx, nil).Once()

	require.NoError(t, f.tick(100))
	require.Equal(t, tx.Hash(), *f.s.rootBondRecoveries[f.app.ID][0].TxHash)
	f.assertNoPermanentStatusWrite(t)
	f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
	tournament.AssertExpectations(t)
}

func TestResultSnapshotAllowsZeroFinalState(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	epoch.MachineHash = new(common.Hash{})
	snapshot := resultTestSnapshot(epoch, false)
	require.NoError(t, validateDaveConsensusSnapshot(snapshot, 20))
	require.NoError(t, matchConsensusSnapshotToEpoch(epoch, snapshot))
}

func TestFailedRootRepeatedObservationAndForeclosure(t *testing.T) {
	app := prtRevertTestApp()
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	snapshot := resultTestSnapshot(epoch, false)
	snapshot.stage.IsTournamentFailed = true
	snapshot.stage.WinnerCommitment = common.Hash{}
	snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
	service, repo := newPRTServiceMock()
	for range 2 {
		require.NoError(t, service.recordTournamentResult(t.Context(), app, epoch, snapshot))
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
	}

	app.ForecloseBlock = 100
	app.LastEpochCheckBlock = 100
	app.LastInputCheckBlock = 100
	app.LastTournamentCheckBlock = 100
	service.defaultBlock = model.DefaultBlock_Finalized
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
	service.client = client
	factory := &adapterFactoryMock{}
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(&daveConsensusAdapterMock{}, nil).Once()
	service.adapterFactory = factory
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(false, nil).Once()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock).Return(true, nil).Once()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{HasTournament: new(true)}, repository.Pagination{}, false).
		Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(
		&model.Tournament{Address: *epoch.TournamentAddress, Snapshot: model.TournamentSnapshot{
			AsOfBlock: 100, FinishedAtBlock: 90, Standing: model.TournamentStandingRootFailed,
		}}, nil).Once()
	repo.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, epoch.Index).Return(nil).Once()

	require.NoError(t, service.handleForeclosedApp(t.Context(), app, func() (uint64, error) {
		return service.getDefaultBlockNumber(t.Context())
	}))
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	client.AssertExpectations(t)
	factory.AssertExpectations(t)
}
