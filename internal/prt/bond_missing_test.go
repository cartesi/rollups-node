// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
)

func TestRootBondRecoveryMissingHashWaitsForFreshState(t *testing.T) {
	for _, recoveredElsewhere := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry owned bond", true: "another caller recovered"}[recoveredElsewhere], func(t *testing.T) {
			app := prtRevertTestApp()
			owned := common.HexToAddress("0x600")
			factory := &adapterFactoryMock{}
			s := newRootBondTestService(owned, factory)
			txHash := common.HexToHash("0x400")
			candidate := &rootBondRecovery{EpochIndex: 3, Tournament: common.HexToAddress("0x300"), TxHash: &txHash}
			s.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
			client := &ethClientMock{}
			s.client = client
			client.On("TransactionByHash", mock.Anything, txHash).
				Return((*types.Transaction)(nil), false, ethereum.NotFound).Times(5)
			client.On("TransactionReceipt", mock.Anything, txHash).
				Return((*types.Receipt)(nil), ethereum.NotFound).Times(5)
			for _, head := range []uint64{100, 100, 99, 162} {
				require.NoError(t, s.recoverRootBonds(t.Context(), app, head))
				require.Equal(t, &txHash, candidate.TxHash)
			}
			require.Equal(t, uint64(99), *candidate.FirstMissingBlock)
			require.NoError(t, s.recoverRootBonds(t.Context(), app, 163))
			require.Nil(t, candidate.TxHash)
			require.Nil(t, candidate.FirstMissingBlock)
			require.Len(t, s.rootBondRecoveries[app.ID], 1)
			require.Empty(t, factory.Calls, "expiry must end the action cycle")
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			require.Empty(t, s.repository.(*prtRepositoryMock).Calls)

			consensus := &daveConsensusAdapterMock{}
			consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(164))).
				Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			tournament := &tournamentAdapterMock{}
			factory.On("CreateTournamentAdapter", candidate.Tournament).Return(tournament, nil).Once()
			recovery := canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1)
			if recoveredElsewhere {
				recovery = canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0)
			} else {
				tournament.On("TryRecoveringBond", mock.Anything).Return(types.NewTx(&types.LegacyTx{Nonce: 1}), nil).Once()
			}
			tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(164))).Return(recovery, nil).Once()
			require.NoError(t, s.recoverRootBonds(t.Context(), app, 164))
			if recoveredElsewhere {
				require.Empty(t, s.rootBondRecoveries[app.ID])
			} else {
				require.NotNil(t, candidate.TxHash)
			}
			client.AssertExpectations(t)
			consensus.AssertExpectations(t)
			factory.AssertExpectations(t)
			tournament.AssertExpectations(t)
		})
	}
}

func TestRootBondRecoveryMissingHashRetainsUncertainTransaction(t *testing.T) {
	rpcErr := errors.New("provider unavailable")
	for _, test := range []struct {
		name       string
		lookupErr  error
		pending    bool
		receiptErr error
		receipt    *types.Receipt
		wantErr    error
		resetTimer bool
	}{
		{name: "pending resets timer", pending: true, resetTimer: true},
		{name: "transaction visible without receipt", receiptErr: ethereum.NotFound, wantErr: ethereum.NotFound, resetTimer: true},
		{name: "lookup failure", lookupErr: rpcErr, wantErr: rpcErr},
		{name: "lookup cancellation", lookupErr: context.Canceled, wantErr: context.Canceled},
		{name: "receipt failure", lookupErr: ethereum.NotFound, receiptErr: rpcErr, wantErr: rpcErr},
		{name: "receipt cancellation", lookupErr: ethereum.NotFound, receiptErr: context.Canceled, wantErr: context.Canceled},
		{name: "nil receipt", lookupErr: ethereum.NotFound},
		{name: "wrong receipt hash", lookupErr: ethereum.NotFound,
			receipt: &types.Receipt{TxHash: common.HexToHash("0xbad"), BlockNumber: big.NewInt(21)}},
		{name: "invalid receipt block", lookupErr: ethereum.NotFound,
			receipt: &types.Receipt{TxHash: common.HexToHash("0x400"), BlockNumber: big.NewInt(-1)}},
		{name: "overflow receipt block", lookupErr: ethereum.NotFound,
			receipt: &types.Receipt{TxHash: common.HexToHash("0x400"), BlockNumber: new(big.Int).Lsh(big.NewInt(1), 64)}},
		{name: "invalid receipt status", lookupErr: ethereum.NotFound,
			receipt: &types.Receipt{TxHash: common.HexToHash("0x400"), BlockNumber: big.NewInt(21), Status: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newRootBondTestService(common.HexToAddress("0x600"), &adapterFactoryMock{})
			app := prtRevertTestApp()
			txHash := common.HexToHash("0x400")
			candidate := &rootBondRecovery{EpochIndex: 3, TxHash: &txHash, FirstMissingBlock: new(uint64(1))}
			s.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
			client := &ethClientMock{}
			s.client = client
			client.On("TransactionByHash", mock.Anything, txHash).
				Return((*types.Transaction)(nil), test.pending, test.lookupErr).Once()
			if !test.pending && (test.lookupErr == nil || errors.Is(test.lookupErr, ethereum.NotFound)) {
				client.On("TransactionReceipt", mock.Anything, txHash).Return(test.receipt, test.receiptErr).Once()
			}
			err := s.recoverRootBonds(t.Context(), app, 100)
			switch {
			case test.pending:
				require.NoError(t, err)
			case test.wantErr != nil:
				require.ErrorIs(t, err, test.wantErr)
			default:
				require.Error(t, err)
			}
			require.Equal(t, &txHash, candidate.TxHash)
			require.Equal(t, test.resetTimer, candidate.FirstMissingBlock == nil)
			require.Len(t, s.rootBondRecoveries[app.ID], 1)
			require.Empty(t, s.repository.(*prtRepositoryMock).Calls)
			client.AssertExpectations(t)
		})
	}
}

func TestRootBondRecoveryReceiptOnlyResetsMissingTimer(t *testing.T) {
	app := prtRevertTestApp()
	factory := &adapterFactoryMock{}
	s := newRootBondTestService(common.HexToAddress("0x600"), factory)
	txHash := common.HexToHash("0x400")
	candidate := &rootBondRecovery{
		EpochIndex: 3, Tournament: common.HexToAddress("0x300"), TxHash: &txHash, FirstMissingBlock: new(uint64(1)),
	}
	s.rootBondRecoveries[app.ID] = []*rootBondRecovery{candidate}
	client := &ethClientMock{}
	s.client = client
	client.On("TransactionByHash", mock.Anything, txHash).
		Return((*types.Transaction)(nil), false, ethereum.NotFound).Once()
	client.On("TransactionReceipt", mock.Anything, txHash).Return(&types.Receipt{
		TxHash: txHash, BlockNumber: big.NewInt(101), Status: types.ReceiptStatusSuccessful,
	}, nil).Once()
	tournament := &tournamentAdapterMock{}
	factory.On("CreateTournamentAdapter", candidate.Tournament).Return(tournament, nil).Once()
	tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(101))).
		Return(canonicalBondRecovery(model.BondDispositionRecovered, common.Address{}, 0), nil).Once()
	require.NoError(t, s.recoverRootBonds(t.Context(), app, 100))
	require.Nil(t, candidate.FirstMissingBlock)
	require.Empty(t, s.rootBondRecoveries[app.ID])
	client.AssertExpectations(t)
	factory.AssertExpectations(t)
	tournament.AssertExpectations(t)
}
