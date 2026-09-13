// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func prtForeclosedApp(id int64, block uint64) *model.Application {
	txHash := common.HexToHash("0xcafe")
	return &model.Application{
		ID:                   id,
		Name:                 "prt-app",
		IApplicationAddress:  common.BigToAddress(common.Big1),
		ConsensusType:        model.Consensus_PRT,
		Enabled:              true,
		Status:               model.ApplicationStatus_OK,
		ForecloseBlock:       block,
		ForecloseTransaction: &txHash,
		// Both DaveConsensus observation cursors default to the foreclose
		// block so callers that don't care about the bootstrap guard skip it.
		// Tests that exercise the guard override one cursor explicitly.
		LastEpochCheckBlock: block,
		LastInputCheckBlock: block,
	}
}

// An empty root query still proves that passive observation ran. It must
// include all tournament-bearing epochs, not only particular claim states.
func expectEmptyForeclosedObservation(r *prtRepositoryMock, app *model.Application) func() (uint64, error) {
	r.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{HasTournament: new(true)},
		repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil).Once()
	return func() (uint64, error) { return 120, nil }
}

// TestHandleForeclosedApp_NoOpWhenForecloseBlockZero verifies the guard at
// the top of handleForeclosedApp. The PRT Tick passes every running app
// through this function; only those with a non-zero ForecloseBlock should
// drive any work.
func TestHandleForeclosedApp_NoOpWhenForecloseBlockZero(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := &model.Application{ID: 1, ConsensusType: model.Consensus_PRT}
	require.NoError(t, s.handleForeclosedApp(context.Background(), app, nil))
}

func TestHandleForeclosedAppRecoversQueuedRootBond(t *testing.T) {
	app := prtForeclosedApp(1, 100)
	app.IConsensusAddress = common.HexToAddress("0x200")
	owned := common.HexToAddress("0x600")
	tournamentAddress := common.HexToAddress("0x300")
	factory := &adapterFactoryMock{}
	service := newRootBondTestService(owned, factory)
	service.submissionEnabled = true
	service.queueRootBondRecovery(app.ID, 3, tournamentAddress)
	acceptTx := common.HexToHash("0x400")
	service.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionAccept, Hash: acceptTx, EpochIndex: 3,
	}
	client := &ethClientMock{}
	client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
	client.On("TransactionByHash", mock.Anything, acceptTx).
		Return(types.NewTx(&types.LegacyTx{}), false, nil).Once()
	client.On("TransactionReceipt", mock.Anything, acceptTx).Return(&types.Receipt{
		Status: types.ReceiptStatusSuccessful, TxHash: acceptTx, BlockNumber: big.NewInt(119),
	}, nil).Once()
	service.client = client
	consensus := &daveConsensusAdapterMock{}
	consensus.On("GetCurrentSealedEpoch", mock.Anything).
		Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
	tournament := &tournamentAdapterMock{}
	tournament.On("BondRecovery", mock.Anything).
		Return(canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), nil).Once()
	tournament.On("TryRecoveringBond", mock.Anything).
		Return(types.NewTx(&types.LegacyTx{Nonce: 1}), nil).Once()
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()
	service.repository.(*prtRepositoryMock).On(
		"HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()

	require.NoError(t, service.handleForeclosedApp(context.Background(), app,
		expectEmptyForeclosedObservation(service.repository.(*prtRepositoryMock), app)))
	require.NotNil(t, service.rootBondRecoveries[app.ID][0].TxHash)
	factory.AssertExpectations(t)
	consensus.AssertExpectations(t)
	tournament.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestHandleForeclosedAppReconcilesInFlightRootBondRecovery(t *testing.T) {
	app := prtForeclosedApp(1, 100)
	owned := common.HexToAddress("0x600")
	tournamentAddress := common.HexToAddress("0x300")
	recoveryTx := common.HexToHash("0x400")
	factory := &adapterFactoryMock{}
	service := newRootBondTestService(owned, factory)
	service.submissionEnabled = true
	service.rootBondRecoveries[app.ID] = []*rootBondRecovery{{
		EpochIndex: 3,
		Tournament: tournamentAddress,
		TxHash:     &recoveryTx,
	}}
	client := &ethClientMock{}
	client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
	client.On("TransactionByHash", mock.Anything, recoveryTx).
		Return(types.NewTx(&types.LegacyTx{}), false, nil).Once()
	client.On("TransactionReceipt", mock.Anything, recoveryTx).Return(&types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      recoveryTx,
		BlockNumber: big.NewInt(119),
	}, nil).Once()
	service.client = client
	tournament := &tournamentAdapterMock{}
	tournament.On("BondRecovery", mock.Anything).
		Return(canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil).Once()
	factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()
	service.repository.(*prtRepositoryMock).On(
		"HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()

	require.NoError(t, service.handleForeclosedApp(context.Background(), app,
		expectEmptyForeclosedObservation(service.repository.(*prtRepositoryMock), app)))
	require.Empty(t, service.rootBondRecoveries[app.ID])
	factory.AssertExpectations(t)
	tournament.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestHandleForeclosedAppReconcilesJoinBeforeQueuedRootBond(t *testing.T) {
	for _, pending := range []bool{true, false} {
		name := "mined"
		if pending {
			name = "pending"
		}
		t.Run(name, func(t *testing.T) {
			app := prtForeclosedApp(1, 100)
			app.IConsensusAddress = common.HexToAddress("0x200")
			owned := common.HexToAddress("0x600")
			tournamentAddress := common.HexToAddress("0x300")
			joinTx := common.HexToHash("0x400")
			factory := &adapterFactoryMock{}
			service := newRootBondTestService(owned, factory)
			service.submissionEnabled = true
			service.queueRootBondRecovery(app.ID, 3, tournamentAddress)
			service.pendingTransactions[app.ID] = pendingTournamentTransaction{
				Action: tournamentActionJoin, Hash: joinTx, EpochIndex: 3,
			}
			client := &ethClientMock{}
			client.On("BlockNumber", mock.Anything).Return(uint64(120), nil).Once()
			client.On("TransactionByHash", mock.Anything, joinTx).
				Return(types.NewTx(&types.LegacyTx{}), pending, nil).Once()
			service.client = client

			if pending {
				require.NoError(t, service.handleForeclosedApp(context.Background(), app,
					expectEmptyForeclosedObservation(service.repository.(*prtRepositoryMock), app)))
				require.Contains(t, service.pendingTransactions, app.ID)
				require.Nil(t, service.rootBondRecoveries[app.ID][0].TxHash)
				factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
			} else {
				client.On("TransactionReceipt", mock.Anything, joinTx).Return(&types.Receipt{
					Status: types.ReceiptStatusSuccessful, TxHash: joinTx, BlockNumber: big.NewInt(119),
				}, nil).Once()
				consensus := &daveConsensusAdapterMock{}
				consensus.On("GetCurrentSealedEpoch", mock.Anything).
					Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
				tournament := &tournamentAdapterMock{}
				tournament.On("BondRecovery", mock.Anything).
					Return(canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), nil).Once()
				tournament.On("TryRecoveringBond", mock.Anything).
					Return(types.NewTx(&types.LegacyTx{Nonce: 1}), nil).Once()
				factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
				factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()
				service.repository.(*prtRepositoryMock).On(
					"HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, app.ForecloseBlock,
				).Return(true, nil).Once()

				require.NoError(t, service.handleForeclosedApp(context.Background(), app,
					expectEmptyForeclosedObservation(service.repository.(*prtRepositoryMock), app)))
				require.NotContains(t, service.pendingTransactions, app.ID)
				require.NotNil(t, service.rootBondRecoveries[app.ID][0].TxHash)
				consensus.AssertExpectations(t)
				tournament.AssertExpectations(t)
			}
			service.repository.(*prtRepositoryMock).AssertExpectations(t)
			factory.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestGetObservableApplications_IncludesUnhealthyApps(t *testing.T) {
	r := &prtRepositoryMock{}
	r.On("ListApplications",
		mock.Anything,
		mock.MatchedBy(func(f repository.ApplicationFilter) bool {
			return f.Enabled != nil && *f.Enabled &&
				f.ConsensusType != nil && *f.ConsensusType == model.Consensus_PRT &&
				len(f.Statuses) == 0
		}),
		repository.Pagination{},
		false,
	).Return([]*model.Application{}, uint64(0), nil).Once()

	_, _, err := getObservableApplications(context.Background(), r)
	require.NoError(t, err)
	r.AssertExpectations(t)
}

// TestHandleForeclosedApp_DefersWhenUndrained verifies the
// pre-foreclosure-work guard. While the advancer/validator have epochs to
// process before the foreclose block, the PRT app must keep its current
// status. Marking it terminal early would lose the last machine state needed
// to process its last pre-foreclosure epoch.
func TestHandleForeclosedApp_DefersWhenUndrained(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()
	// No UpdateApplicationStatus expectation — see TestProcessForeclosedApps_DefersWhenUndrained
	// in the claimer suite for the equivalent reasoning.

	require.NoError(t, s.handleForeclosedApp(context.Background(), app, expectEmptyForeclosedObservation(r, app)))
}

// A completed local drain must not stop passive tournament observation. With
// no roots, this pass only queries eligibility and checks the existing gates.
func TestHandleForeclosedApp_ObservesAfterLocalDrain(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()

	require.NoError(t, s.handleForeclosedApp(context.Background(), app, expectEmptyForeclosedObservation(r, app)))
}

// TestHandleForeclosedApp_SurfacesDrainCheckError verifies the surrounding
// behavior on transient repository failures: the error must propagate so
// the Tick's err slice marks the app as in trouble; the app keeps its current
// status for retry on the next tick.
func TestHandleForeclosedApp_SurfacesDrainCheckError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("connection refused")
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, dbErr).Once()

	err := s.handleForeclosedApp(context.Background(), app, expectEmptyForeclosedObservation(r, app))
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

// TestHandleForeclosedApp_DefersWhenStillBackfilling verifies the
// bootstrap-readiness guard. When a freshly registered PRT app encounters
// an already-foreclosed contract, evmreader sets ForecloseBlock before
// checkForEpochsAndInputs has ingested any historical sealed epochs. The
// drain gate would then see an empty input table and incorrectly return
// false, making the app look drained before any pre-foreclosure epoch is
// observed locally. The guard must defer the drain check until
// both DaveConsensus observation cursors reach ForecloseBlock.
//
// The mock has no HasUndrainedEpochsBeforeBlock or UpdateApplicationStatus
// expectation registered; testify/mock panics on an unexpected call, so
// either reach attempt fails the test loudly.
func TestHandleForeclosedApp_DefersWhenStillBackfilling(t *testing.T) {
	for _, test := range []struct {
		name        string
		epochCursor uint64
		inputCursor uint64
	}{
		{name: "sealed epochs still landing", epochCursor: 50, inputCursor: 100},
		{name: "same-block open inputs still landing", epochCursor: 100, inputCursor: 99},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, r := newPRTServiceMock()
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			app := prtForeclosedApp(1, 100)
			app.LastEpochCheckBlock = test.epochCursor
			app.LastInputCheckBlock = test.inputCursor

			require.NoError(t, s.handleForeclosedApp(t.Context(), app, expectEmptyForeclosedObservation(r, app)))
			require.Contains(t, output.String(), "sealed epochs and inputs")
			require.Contains(t, output.String(), "last_epoch_check_block="+new(big.Int).SetUint64(test.epochCursor).String())
			require.Contains(t, output.String(), "last_input_check_block="+new(big.Int).SetUint64(test.inputCursor).String())
			require.Contains(t, output.String(), "foreclose_block=100")
			r.AssertNotCalled(t, "HasUndrainedEpochsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
			r.AssertNotCalled(t, "HasUnreconciledClaimsBeforeBlock", mock.Anything, mock.Anything, mock.Anything)
			r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, mock.Anything, mock.Anything)
			r.AssertExpectations(t)
		})
	}
}

// TestHandleForeclosedApp_SurfacesReconciliationCheckError verifies the
// epoch-level completion gate's error propagates so the Tick retries on the
// next pass and the app keeps its status meanwhile.
func TestHandleForeclosedApp_SurfacesReconciliationCheckError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("connection refused")
	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, dbErr).Once()

	err := s.handleForeclosedApp(context.Background(), app, expectEmptyForeclosedObservation(r, app))
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

// TestHandleForeclosedApp_LeavesClaimedEpochForNextReconciliationPass models
// an epoch that appears between the observation query and the local drain
// query. An on-chain EpochSealed transaction hash requires reconciliation on
// the next pass; the local drain must not mark that epoch CLAIM_FORECLOSED.
func TestHandleForeclosedApp_LeavesClaimedEpochForNextReconciliationPass(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(120)}, nil).Once()
	s.client = client
	s.defaultBlock = model.DefaultBlock_Finalized
	claimTx := common.HexToHash("0xbeef")

	r.On("HasUndrainedEpochsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(false, nil).Once()
	r.On("HasUnreconciledClaimsBeforeBlock",
		mock.Anything, app.ID, app.ForecloseBlock,
	).Return(true, nil).Once()
	r.On("ListEpochs",
		mock.Anything, app.Name, repository.EpochFilter{HasTournament: new(true)}, repository.Pagination{}, false,
	).Return([]*model.Epoch{}, uint64(0), nil).Once()
	r.On("ListEpochs",
		mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()}, repository.Pagination{}, false,
	).Return([]*model.Epoch{
		{Index: 1, ClaimTransactionHash: &claimTx},
	}, uint64(1), nil).Once()

	require.NoError(t, s.handleForeclosedApp(context.Background(), app, func() (uint64, error) {
		return s.getDefaultBlockNumber(context.Background())
	}))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(1))
	client.AssertExpectations(t)
}

// TestForeclosePendingClaimEpochs_TerminalizesEachPendingEpoch verifies the
// foreclose step: every pending claim epoch without an on-chain sealed-event
// transaction is transitioned to CLAIM_FORECLOSED.
func TestForeclosePendingClaimEpochs_TerminalizesEachPendingEpoch(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	claimTx := common.HexToHash("0xbeef")
	r.On("ListEpochs",
		mock.Anything, app.Name,
		repository.EpochFilter{Status: []model.EpochStatus{
			model.EpochStatus_Open, model.EpochStatus_Closed, model.EpochStatus_InputsProcessed,
			model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimSubmitted, model.EpochStatus_ClaimStaged,
		}},
		repository.Pagination{}, false,
	).Return([]*model.Epoch{
		{Index: 0, Status: model.EpochStatus_Open, FirstBlock: 100, LastBlock: 150},
		{Index: 1, Status: model.EpochStatus_Closed},
		{Index: 2, Status: model.EpochStatus_InputsProcessed},
		{Index: 3},
		{Index: 4, Status: model.EpochStatus_ClaimStaged},
		{Index: 5, ClaimTransactionHash: &claimTx},
		{Index: 6, Status: model.EpochStatus_ClaimSubmitted},
		{Index: 7, Status: model.EpochStatus_Open, FirstBlock: 101},
	}, uint64(8), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(0)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(1)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(2)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(3)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(4)).Return(nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(6)).Return(nil).Once()

	require.NoError(t, s.foreclosePendingClaimEpochs(context.Background(), app))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(5))
	r.AssertNotCalled(t, "UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(7))
}

// TestForeclosePendingClaimEpochs_PropagatesUpdateError verifies a failed
// terminalization surfaces so the Tick retries rather than silently dropping
// a still-non-terminal epoch.
func TestForeclosePendingClaimEpochs_PropagatesUpdateError(t *testing.T) {
	s, r := newPRTServiceMock()
	defer r.AssertExpectations(t)

	app := prtForeclosedApp(1, 100)
	dbErr := errors.New("write failed")
	r.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{{Index: 7}}, uint64(1), nil).Once()
	r.On("UpdateEpochWithForeclosedClaim", mock.Anything, app.ID, uint64(7)).Return(dbErr).Once()

	err := s.foreclosePendingClaimEpochs(context.Background(), app)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}
