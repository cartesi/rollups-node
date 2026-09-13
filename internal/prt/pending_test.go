// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPendingTournamentTransactionRetainsOrReleasesSlot(t *testing.T) {
	for _, action := range []tournamentAction{tournamentActionJoin, tournamentActionStage, tournamentActionAccept} {
		for _, test := range []struct {
			name         string
			pending      bool
			lookupError  error
			receiptError error
			mutate       func(*types.Receipt) *types.Receipt
			wantError    bool
			wantRelease  bool
			wantRevert   bool
		}{
			{name: "pending", pending: true},
			{name: "missing transaction with receipt", lookupError: ethereum.NotFound, wantRelease: true},
			{name: "missing transaction with reverted receipt", lookupError: ethereum.NotFound,
				mutate:      func(r *types.Receipt) *types.Receipt { r.Status = types.ReceiptStatusFailed; return r },
				wantRelease: true, wantRevert: true},
			{name: "missing transaction with nil receipt", lookupError: ethereum.NotFound,
				mutate: func(*types.Receipt) *types.Receipt { return nil }, wantError: true},
			{name: "missing transaction with wrong receipt hash", lookupError: ethereum.NotFound,
				mutate: func(r *types.Receipt) *types.Receipt { r.TxHash = common.Hash{}; return r }, wantError: true},
			{name: "missing receipt", receiptError: ethereum.NotFound, wantError: true},
			{name: "nil receipt", mutate: func(*types.Receipt) *types.Receipt { return nil }, wantError: true},
			{name: "wrong hash", mutate: func(r *types.Receipt) *types.Receipt { r.TxHash = common.Hash{}; return r }, wantError: true},
			{name: "missing block", mutate: func(r *types.Receipt) *types.Receipt { r.BlockNumber = nil; return r }, wantError: true},
			{name: "invalid status", mutate: func(r *types.Receipt) *types.Receipt { r.Status = 2; return r }, wantError: true},
			{name: "mined success", wantRelease: true},
			{name: "mined revert", mutate: func(r *types.Receipt) *types.Receipt { r.Status = 0; return r },
				wantRelease: true, wantRevert: true},
		} {
			t.Run(string(action)+"/"+test.name, func(t *testing.T) {
				app := prtRevertTestApp()
				s, repo := newPRTServiceMock()
				var output bytes.Buffer
				s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				tx := pendingTournamentTransaction{Action: action, Hash: common.HexToHash("0xbeef"), EpochIndex: 3}
				s.pendingTransactions[app.ID] = tx
				s.pendingTransactions[app.ID+1] = tx
				client := &ethClientMock{}
				client.On("TransactionByHash", mock.Anything, tx.Hash).
					Return((*types.Transaction)(nil), test.pending, test.lookupError).Once()
				if !test.pending {
					receipt := &types.Receipt{TxHash: tx.Hash, BlockNumber: big.NewInt(21), Status: types.ReceiptStatusSuccessful}
					if test.mutate != nil {
						receipt = test.mutate(receipt)
					}
					client.On("TransactionReceipt", mock.Anything, tx.Hash).Return(receipt, test.receiptError).Once()
				}
				s.client = client
				// A receipt newer than the observed head does not need finality to
				// release the local slot. It cannot mark an epoch or app as terminal.
				joinEpoch, _, err := s.progressTournamentResult(t.Context(), app, 20, 20)
				require.Equal(t, test.wantError, err != nil, "%v", err)
				require.Nil(t, joinEpoch)
				if test.wantRelease {
					require.NotContains(t, s.pendingTransactions, app.ID)
				} else {
					require.Equal(t, tx, s.pendingTransactions[app.ID])
				}
				require.Equal(t, tx, s.pendingTransactions[app.ID+1], "another application's slot must not change")
				require.Equal(t, ApplicationStatus_OK, app.Status)
				require.Empty(t, repo.Calls)
				if test.wantRevert {
					require.Contains(t, output.String(), "level=ERROR")
					require.Contains(t, output.String(), "action="+string(action))
					require.Contains(t, output.String(), "epoch_index=3")
					require.Contains(t, output.String(), tx.Hash.Hex())
				} else {
					require.NotContains(t, output.String(), "level=ERROR")
				}
				client.AssertExpectations(t)
			})
		}
	}
}

func TestPendingJoinReadsCurrentEpochOnlyAfterMining(t *testing.T) {
	epoch := resultTestEpoch(EpochStatus_ClaimComputed)
	snapshot := resultTestSnapshot(epoch, false)
	snapshot.stage = CanStageTournamentResult{EpochNumber: epoch.Index}
	s, repo, consensus := resultTestService(t, epoch, snapshot, true)
	app := prtRevertTestApp()
	tx := types.NewTx(&types.LegacyTx{})
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionJoin, Hash: tx.Hash(), EpochIndex: epoch.Index - 1,
	}
	client := &ethClientMock{}
	client.On("TransactionByHash", mock.Anything, tx.Hash()).Return(tx, true, nil).Once()
	client.On("TransactionByHash", mock.Anything, tx.Hash()).Return(tx, false, nil).Once()
	client.On("TransactionReceipt", mock.Anything, tx.Hash()).Return(&types.Receipt{
		TxHash: tx.Hash(), BlockNumber: big.NewInt(20), Status: types.ReceiptStatusSuccessful,
	}, nil).Once()
	s.client = client
	for range 2 {
		joinEpoch, _, err := s.progressTournamentResult(t.Context(), app, 20, 20)
		require.NoError(t, err)
		require.Nil(t, joinEpoch)
		require.Empty(t, repo.Calls)
		require.Empty(t, consensus.Calls)
	}
	// The chain advanced while the old join was pending. The next snapshot,
	// not the old transaction's epoch, supplies the new join decision.
	joinEpoch, _, err := s.progressTournamentResult(t.Context(), app, 20, 20)
	require.NoError(t, err)
	require.Same(t, epoch, joinEpoch)
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}
