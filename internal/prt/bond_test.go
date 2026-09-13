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
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

const noWinnerCase = "no winner"

func TestBondRecoveryFromBindingMapsAndOwnsPayment(t *testing.T) {
	claimer := common.HexToAddress("0x100")
	payment := big.NewInt(42)
	recovery, err := bondRecoveryFromBinding(uint8(2), claimer, payment)
	require.NoError(t, err)
	require.Equal(t, model.BondDispositionRecoverable, recovery.Disposition)
	require.Equal(t, claimer, recovery.Claimer)
	require.Equal(t, int64(42), recovery.Payment.Int64())

	recovery.Payment.SetInt64(7)
	require.Equal(t, int64(42), payment.Int64())
}

func TestBondRecoveryFromBindingValidatesCanonicalState(t *testing.T) {
	claimer := common.HexToAddress("0x100")
	tests := []struct {
		name        string
		disposition uint8
		claimer     common.Address
		payment     *big.Int
	}{
		{name: "unknown disposition", disposition: 4, payment: big.NewInt(0)},
		{name: "nil payment", disposition: uint8(3)},
		{name: "negative payment", disposition: uint8(2), claimer: claimer, payment: big.NewInt(-1)},
		{name: "running claimer", disposition: uint8(0), claimer: claimer, payment: big.NewInt(0)},
		{name: "no winner payment", disposition: uint8(1), payment: big.NewInt(1)},
		{name: "recovered claimer", disposition: uint8(3), claimer: claimer, payment: big.NewInt(0)},
		{name: "recoverable zero claimer", disposition: uint8(2), payment: big.NewInt(0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := bondRecoveryFromBinding(test.disposition, test.claimer, test.payment)
			require.ErrorIs(t, err, errInvalidBondRecovery)
		})
	}

	recovery, err := bondRecoveryFromBinding(
		uint8(2), claimer, big.NewInt(0),
	)
	require.NoError(t, err)
	require.Zero(t, recovery.Payment.Sign(), "a zero recoverable payment is valid")
}

func TestAcceptQueuesRootBondBeforeCallAndDeduplicates(t *testing.T) {
	app := prtRevertTestApp()
	epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
	snapshot := resultTestSnapshot(epoch, true)
	acceptErr := errors.New("accept failed")
	consensus := &daveConsensusAdapterMock{}
	service := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
	var output bytes.Buffer
	service.Logger = slog.New(slog.NewTextHandler(&output, nil))
	consensus.On("AcceptStagedTournamentResult", mock.Anything, epoch.Index).
		Run(func(mock.Arguments) {
			require.Len(t, service.rootBondRecoveries[app.ID], 1)
			candidate := service.rootBondRecoveries[app.ID][0]
			require.Equal(t, epoch.Index, candidate.EpochIndex)
			require.Equal(t, snapshot.sealed.Tournament, candidate.Tournament)
		}).
		Return((*types.Transaction)(nil), acceptErr).Twice()

	for range 2 {
		err := service.broadcastAcceptTournamentResult(context.Background(), app, epoch, consensus, snapshot)
		require.ErrorIs(t, err, acceptErr)
	}
	require.Len(t, service.rootBondRecoveries[app.ID], 1)
	require.Equal(t, 1, strings.Count(output.String(), "Queued root tournament bond recovery candidate"))
	require.Contains(t, output.String(), "restart clears")
	require.Contains(t, output.String(), "application_id=7")
	require.Contains(t, output.String(), "epoch_index=3")
	require.Contains(t, output.String(), snapshot.sealed.Tournament.Hex())
	require.NotContains(t, output.String(), "payment=")
	require.NotContains(t, output.String(), "claimer=")
	consensus.AssertExpectations(t)
}

func TestRootBondRecoveryWaitsForNextEpoch(t *testing.T) {
	app := prtRevertTestApp()
	service := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
	service.queueRootBondRecovery(app.ID, 3, common.HexToAddress("0x300"))
	consensus := &daveConsensusAdapterMock{}
	consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(20))).
		Return(CurrentSealedEpoch{EpochNumber: 3}, nil).Once()
	service.adapterFactory.(*adapterFactoryMock).
		On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()

	require.NoError(t, service.recoverRootBonds(context.Background(), app, 20))
	require.Len(t, service.rootBondRecoveries[app.ID], 1)
	service.adapterFactory.(*adapterFactoryMock).AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestRootBondRecoveryHandlesEveryDispositionAndOwnership(t *testing.T) {
	owned := common.HexToAddress("0x600")
	external := common.HexToAddress("0x700")
	tests := []struct {
		name        string
		recovery    BondRecovery
		wantRetired bool
		wantTx      bool
	}{
		{name: "running", recovery: canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0)},
		{name: noWinnerCase, recovery: canonicalBondRecovery(model.BondDispositionNoWinner, common.Address{}, 0), wantRetired: true},
		{name: "recovered by other actor",
			recovery: canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), wantRetired: true},
		{name: "external claimer", recovery: canonicalBondRecovery(model.BondDispositionRecoverable, external, 8), wantRetired: true},
		{name: "owned zero payment", recovery: canonicalBondRecovery(model.BondDispositionRecoverable, owned, 0), wantTx: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := prtRevertTestApp()
			factory := &adapterFactoryMock{}
			service := newRootBondTestService(owned, factory)
			tournamentAddress := common.HexToAddress("0x300")
			service.queueRootBondRecovery(app.ID, 3, tournamentAddress)
			consensus := &daveConsensusAdapterMock{}
			consensus.On("GetCurrentSealedEpoch", mock.Anything).
				Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
			tournament := &tournamentAdapterMock{}
			tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(20))).Return(test.recovery, nil).Once()
			if test.wantTx {
				tournament.On("TryRecoveringBond", mock.Anything).
					Return(types.NewTx(&types.LegacyTx{Nonce: 1}), nil).Once()
			}
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()

			require.NoError(t, service.recoverRootBonds(context.Background(), app, 20))
			if test.wantRetired {
				require.Empty(t, service.rootBondRecoveries[app.ID])
				// A resolved candidate must not trigger another state read or send.
				require.NoError(t, service.recoverRootBonds(context.Background(), app, 21))
			} else {
				require.Len(t, service.rootBondRecoveries[app.ID], 1)
				require.Equal(t, test.wantTx, service.rootBondRecoveries[app.ID][0].TxHash != nil)
			}
			factory.AssertExpectations(t)
			consensus.AssertExpectations(t)
			tournament.AssertExpectations(t)
		})
	}
}

func TestPendingRootBondRecoveryBlocksOtherMutations(t *testing.T) {
	app := prtRevertTestApp()
	service := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
	candidate := &rootBondRecovery{EpochIndex: 3, Tournament: common.HexToAddress("0x300")}
	txHash := common.HexToHash("0x400")
	candidate.TxHash = &txHash
	service.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
	client := &ethClientMock{}
	client.On("TransactionByHash", mock.Anything, txHash).
		Return(types.NewTx(&types.LegacyTx{}), true, nil).Once()
	service.client = client

	joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.NoError(t, err)
	require.Nil(t, joinEpoch)
	require.NoError(t, service.recoverRootBonds(context.Background(), app, 20))
	require.Equal(t, &txHash, candidate.TxHash)
	client.AssertExpectations(t)
}

func TestOtherPRTMutationBlocksRootBondRecovery(t *testing.T) {
	app := prtRevertTestApp()
	for _, action := range []tournamentAction{tournamentActionJoin, tournamentActionStage, tournamentActionAccept} {
		t.Run(string(action), func(t *testing.T) {
			factory := &adapterFactoryMock{}
			service := newRootBondTestService(common.HexToAddress("0x600"), factory)
			service.queueRootBondRecovery(app.ID, 3, common.HexToAddress("0x300"))
			txHash := common.HexToHash("0x400")
			service.pendingTransactions[app.ID] = pendingTournamentTransaction{Action: action, Hash: txHash}

			require.NoError(t, service.recoverRootBonds(context.Background(), app, 20))
			require.Len(t, service.rootBondRecoveries[app.ID], 1)
			factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
			factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
		})
	}
}

func TestConcurrentRootRecoveryAndResultMutationIsRejected(t *testing.T) {
	app := prtRevertTestApp()
	service := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
	recoveryTx := common.HexToHash("0x400")
	stageTx := common.HexToHash("0x500")
	service.rootBondRecoveries[app.ID] = []*rootBondRecovery{{
		EpochIndex: 3,
		Tournament: common.HexToAddress("0x300"),
		TxHash:     &recoveryTx,
	}}
	service.pendingTransactions[app.ID] = pendingTournamentTransaction{Action: tournamentActionStage, Hash: stageTx}

	joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.Nil(t, joinEpoch)
	require.ErrorContains(t, err, "bond recovery and another PRT transaction in flight")
}

func TestRootBondRecoveryKnownRevertsWaitForFreshView(t *testing.T) {
	app := prtRevertTestApp()
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "tournament not finished", err: tournamentRevertError("TournamentNotFinished")},
		{name: noWinnerCase, err: tournamentRevertError("NoWinner")},
		{name: "nonce too low", err: errors.New("nonce too low")},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
			candidate := &rootBondRecovery{EpochIndex: 3, Tournament: common.HexToAddress("0x300")}
			tournament := &tournamentAdapterMock{}
			tournament.On("TryRecoveringBond", mock.Anything).
				Return((*types.Transaction)(nil), test.err).Once()

			err := service.broadcastRootBondRecovery(
				context.Background(), app, candidate, tournament,
				canonicalBondRecovery(model.BondDispositionRecoverable, common.HexToAddress("0x600"), 1),
			)
			require.NoError(t, err)
			require.Nil(t, candidate.TxHash)
			tournament.AssertExpectations(t)
		})
	}
}

func TestMinedRootBondRecoveryUsesPostTransactionState(t *testing.T) {
	owned := common.HexToAddress("0x600")
	external := common.HexToAddress("0x700")
	for _, test := range []struct {
		name           string
		status         uint64
		recovery       BondRecovery
		wantRetired    bool
		wantTxCleared  bool
		wantError      string
		wantFailedPush bool
	}{
		{name: "recovered", status: types.ReceiptStatusSuccessful,
			recovery: canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), wantRetired: true},
		{name: "successful receipt with failed push", status: types.ReceiptStatusSuccessful,
			recovery:    canonicalBondRecovery(model.BondDispositionRecoverable, owned, 7),
			wantRetired: true, wantFailedPush: true},
		{name: "recoverable bond now belongs to another claimer", status: types.ReceiptStatusSuccessful,
			recovery: canonicalBondRecovery(model.BondDispositionRecoverable, external, 7), wantRetired: true},
		{name: "failed receipt with recoverable state", status: types.ReceiptStatusFailed,
			recovery: canonicalBondRecovery(model.BondDispositionRecoverable, owned, 7), wantTxCleared: true},
		{name: noWinnerCase, status: types.ReceiptStatusSuccessful,
			recovery: canonicalBondRecovery(model.BondDispositionNoWinner, common.Address{}, 0), wantRetired: true},
		{name: "running", status: types.ReceiptStatusFailed,
			recovery:      canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0),
			wantTxCleared: true, wantError: "running bond"},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := prtRevertTestApp()
			factory := &adapterFactoryMock{}
			service := newRootBondTestService(owned, factory)
			var logs bytes.Buffer
			service.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			tournamentAddress := common.HexToAddress("0x300")
			txHash := common.HexToHash("0x400")
			candidate := &rootBondRecovery{EpochIndex: 3, Tournament: tournamentAddress, TxHash: &txHash}
			service.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
			client := &ethClientMock{}
			client.On("TransactionByHash", mock.Anything, txHash).
				Return(types.NewTx(&types.LegacyTx{}), false, nil).Once()
			client.On("TransactionReceipt", mock.Anything, txHash).Return(&types.Receipt{
				Status:      test.status,
				TxHash:      txHash,
				BlockNumber: big.NewInt(21),
			}, nil).Once()
			service.client = client
			tournament := &tournamentAdapterMock{}
			tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(21))).Return(test.recovery, nil).Once()
			factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()

			err := service.recoverRootBonds(context.Background(), app, 20)
			if test.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantError)
			}
			require.Equal(t, test.wantRetired, len(service.rootBondRecoveries[app.ID]) == 0)
			if test.wantTxCleared {
				require.Nil(t, candidate.TxHash)
			}
			if test.wantFailedPush {
				require.Contains(t, logs.String(), "level=ERROR")
				require.Contains(t, logs.String(), "outcome=failed_push")
			} else {
				require.NotContains(t, logs.String(), "outcome=failed_push")
			}
			client.AssertExpectations(t)
			factory.AssertExpectations(t)
			tournament.AssertExpectations(t)
		})
	}
}

func TestRootBondRecoveryKeepsCandidateOnErrors(t *testing.T) {
	owned := common.HexToAddress("0x600")
	app := prtRevertTestApp()
	tournamentAddress := common.HexToAddress("0x300")

	t.Run("pre-broadcast view", func(t *testing.T) {
		factory := &adapterFactoryMock{}
		service := newRootBondTestService(owned, factory)
		service.queueRootBondRecovery(app.ID, 3, tournamentAddress)
		consensus := &daveConsensusAdapterMock{}
		consensus.On("GetCurrentSealedEpoch", mock.Anything).
			Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
		tournament := &tournamentAdapterMock{}
		tournament.On("BondRecovery", mock.Anything).Return(BondRecovery{}, errors.New("view failed")).Once()
		factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
		factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()

		require.ErrorContains(t, service.recoverRootBonds(context.Background(), app, 20), "view failed")
		require.Len(t, service.rootBondRecoveries[app.ID], 1)
		require.Nil(t, service.rootBondRecoveries[app.ID][0].TxHash)
	})

	t.Run("nil transaction", func(t *testing.T) {
		factory := &adapterFactoryMock{}
		service := newRootBondTestService(owned, factory)
		service.queueRootBondRecovery(app.ID, 3, tournamentAddress)
		consensus := &daveConsensusAdapterMock{}
		consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
		tournament := &tournamentAdapterMock{}
		tournament.On("BondRecovery", mock.Anything).
			Return(canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), nil).Once()
		tournament.On("TryRecoveringBond", mock.Anything).Return((*types.Transaction)(nil), nil).Once()
		factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
		factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()

		require.ErrorContains(t, service.recoverRootBonds(context.Background(), app, 20), "nil transaction")
		require.Len(t, service.rootBondRecoveries[app.ID], 1)
		require.Nil(t, service.rootBondRecoveries[app.ID][0].TxHash)
	})

	t.Run("transaction lookup", func(t *testing.T) {
		service := newRootBondTestService(owned, &adapterFactoryMock{})
		txHash := common.HexToHash("0x400")
		candidate := &rootBondRecovery{EpochIndex: 3, Tournament: tournamentAddress, TxHash: &txHash}
		service.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
		client := &ethClientMock{}
		client.On("TransactionByHash", mock.Anything, txHash).
			Return((*types.Transaction)(nil), false, errors.New("lookup failed")).Once()
		service.client = client

		require.ErrorContains(t, service.recoverRootBonds(context.Background(), app, 20), "lookup failed")
		require.Equal(t, &txHash, candidate.TxHash)
	})

	for _, test := range []struct {
		name    string
		viewErr error
		clearTx bool
	}{
		{name: "post-transaction view", viewErr: errors.New("view failed")},
		{name: "invalid post-transaction view", viewErr: errInvalidBondRecovery, clearTx: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory := &adapterFactoryMock{}
			service := newRootBondTestService(owned, factory)
			txHash := common.HexToHash("0x400")
			candidate := &rootBondRecovery{EpochIndex: 3, Tournament: tournamentAddress, TxHash: &txHash}
			service.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
			client := &ethClientMock{}
			client.On("TransactionByHash", mock.Anything, txHash).
				Return(types.NewTx(&types.LegacyTx{}), false, nil).Once()
			client.On("TransactionReceipt", mock.Anything, txHash).
				Return(&types.Receipt{TxHash: txHash, BlockNumber: big.NewInt(21)}, nil).Once()
			service.client = client
			tournament := &tournamentAdapterMock{}
			tournament.On("BondRecovery", mock.Anything).Return(BondRecovery{}, test.viewErr).Once()
			factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()

			require.ErrorIs(t, service.recoverRootBonds(context.Background(), app, 20), test.viewErr)
			if test.clearTx {
				require.Nil(t, candidate.TxHash)
			} else {
				require.Equal(t, &txHash, candidate.TxHash)
			}
			require.Len(t, service.rootBondRecoveries[app.ID], 1)
			require.Same(t, candidate, service.rootBondRecoveries[app.ID][0])
			client.AssertExpectations(t)
			tournament.AssertExpectations(t)
			factory.AssertExpectations(t)
		})
	}

	for _, test := range []struct {
		name    string
		receipt *types.Receipt
		want    string
	}{
		{name: "receipt hash", receipt: &types.Receipt{
			TxHash: common.HexToHash("0xbad"), BlockNumber: big.NewInt(21),
		}, want: "differs from transaction"},
		{name: "receipt status", receipt: &types.Receipt{
			TxHash: common.HexToHash("0x400"), BlockNumber: big.NewInt(21), Status: 2,
		}, want: "invalid receipt status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newRootBondTestService(owned, &adapterFactoryMock{})
			txHash := common.HexToHash("0x400")
			candidate := &rootBondRecovery{EpochIndex: 3, Tournament: tournamentAddress, TxHash: &txHash}
			service.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
			client := &ethClientMock{}
			client.On("TransactionByHash", mock.Anything, txHash).
				Return(types.NewTx(&types.LegacyTx{}), false, nil).Once()
			client.On("TransactionReceipt", mock.Anything, txHash).Return(test.receipt, nil).Once()
			service.client = client

			require.ErrorContains(t, service.recoverRootBonds(context.Background(), app, 20), test.want)
			require.Equal(t, &txHash, candidate.TxHash)
		})
	}
}

func TestRootBondRecoveryIsOldestFirstAndIsolatedPerApplication(t *testing.T) {
	owned := common.HexToAddress("0x600")
	factory := &adapterFactoryMock{}
	service := newRootBondTestService(owned, factory)
	appOne := prtRevertTestApp()
	appTwo := prtRevertTestApp()
	appTwo.ID = 8
	appTwo.Name = "prt-app-two"
	appTwo.IConsensusAddress = common.HexToAddress("0x108")
	rootOne := common.HexToAddress("0x301")
	rootTwo := common.HexToAddress("0x302")
	rootThree := common.HexToAddress("0x303")
	service.queueRootBondRecovery(appOne.ID, 1, rootOne)
	service.queueRootBondRecovery(appOne.ID, 2, rootTwo)
	service.queueRootBondRecovery(appTwo.ID, 1, rootThree)

	for _, app := range []*model.Application{appOne, appTwo} {
		consensus := &daveConsensusAdapterMock{}
		consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(CurrentSealedEpoch{EpochNumber: 3}, nil).Once()
		factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	}
	for _, address := range []common.Address{rootOne, rootThree} {
		tournament := &tournamentAdapterMock{}
		tournament.On("BondRecovery", mock.Anything).
			Return(canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), nil).Once()
		tournament.On("TryRecoveringBond", mock.Anything).
			Return(types.NewTx(&types.LegacyTx{Nonce: uint64(address[19])}), nil).Once()
		factory.On("CreateTournamentAdapter", address).Return(tournament, nil).Once()
	}

	require.NoError(t, service.recoverRootBonds(context.Background(), appOne, 20))
	require.NotNil(t, service.rootBondRecoveries[appOne.ID][0].TxHash)
	require.Nil(t, service.rootBondRecoveries[appOne.ID][1].TxHash)
	require.NoError(t, service.recoverRootBonds(context.Background(), appTwo, 20))
	require.NotNil(t, service.rootBondRecoveries[appTwo.ID][0].TxHash)
	factory.AssertNotCalled(t, "CreateTournamentAdapter", rootTwo)
	factory.AssertExpectations(t)
}

func newRootBondTestService(from common.Address, factory AdapterFactory) *Service {
	service, _ := newPRTServiceMock()
	service.adapterFactory = factory
	service.submissionTimeout = time.Second
	service.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: from})
	return service
}

func canonicalBondRecovery(disposition model.BondDisposition, claimer common.Address, payment int64) BondRecovery {
	return BondRecovery{Disposition: disposition, Claimer: claimer, Payment: big.NewInt(payment)}
}
