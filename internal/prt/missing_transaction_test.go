// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func expectMissingTournamentTransaction(client *ethClientMock, hash common.Hash) {
	client.On("TransactionByHash", mock.Anything, hash).
		Return((*types.Transaction)(nil), false, fmt.Errorf("lookup: %w", ethereum.NotFound)).Once()
	client.On("TransactionReceipt", mock.Anything, hash).
		Return((*types.Receipt)(nil), fmt.Errorf("receipt: %w", ethereum.NotFound)).Once()
}

func TestMissingTournamentTransactionExpiresAfterBlockBudget(t *testing.T) {
	for _, action := range []tournamentAction{tournamentActionJoin, tournamentActionStage, tournamentActionAccept} {
		for _, start := range []uint64{0, 100, math.MaxUint64 - 64} {
			t.Run(fmt.Sprintf("%s/from_%d", action, start), func(t *testing.T) {
				s, repo := newPRTServiceMock()
				app := prtRevertTestApp()
				tx := pendingTournamentTransaction{Action: action, Hash: common.HexToHash("0xbeef"), EpochIndex: 3}
				s.pendingTransactions[app.ID] = tx
				s.pendingTransactions[app.ID+1] = tx
				client := &ethClientMock{}
				s.client = client
				var logs bytes.Buffer
				s.Logger = slog.New(slog.NewTextHandler(&logs, nil))

				// Pin the 64-block policy independently of the implementation constant.
				// Neither repeated ticks nor a much older observation head may age it.
				for i, head := range []uint64{start, start, start + 63, start + 64} {
					expectMissingTournamentTransaction(client, tx.Hash)
					epoch, recovery, err := s.progressTournamentResult(t.Context(), app, 0, head)
					require.NoError(t, err)
					require.Nil(t, epoch)
					require.False(t, recovery, "do not broadcast a refund on the release tick")
					require.Equal(t, i != 3, s.hasNonRecoveryMutationInFlight(app.ID))
					require.Equal(t, tx, s.pendingTransactions[app.ID+1])
					require.Empty(t, repo.Calls, "release cannot change epoch or application state")
					if i != 3 {
						require.Empty(t, logs.String(), "a normal bounded wait is not an operational error")
					}
				}
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				require.Contains(t, logs.String(), "level=WARN")
				require.Contains(t, logs.String(), "action="+string(action))
				require.Contains(t, logs.String(), tx.Hash.Hex())
				client.AssertExpectations(t)
			})
		}
	}
}

func TestMissingTournamentTransactionWaitRestartsWhenKnown(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtRevertTestApp()
	tx := pendingTournamentTransaction{Action: tournamentActionJoin, Hash: common.HexToHash("0xbeef")}
	s.pendingTransactions[app.ID] = tx
	client := &ethClientMock{}
	s.client = client

	expectMissingTournamentTransaction(client, tx.Hash)
	_, _, err := s.progressTournamentResult(t.Context(), app, 0, 100)
	require.NoError(t, err)
	// Even a very old transaction must stay tracked while it is known pending.
	client.On("TransactionByHash", mock.Anything, tx.Hash).Return((*types.Transaction)(nil), true, nil).Once()
	_, _, err = s.progressTournamentResult(t.Context(), app, 0, 1_000)
	require.NoError(t, err)
	require.Equal(t, tx, s.pendingTransactions[app.ID])
	for i, head := range []uint64{1_001, 1_064, 1_065} {
		expectMissingTournamentTransaction(client, tx.Hash)
		_, _, err = s.progressTournamentResult(t.Context(), app, 0, head)
		require.NoError(t, err)
		require.Equal(t, i != 2, s.hasNonRecoveryMutationInFlight(app.ID))
	}
	require.Empty(t, repo.Calls)
	client.AssertExpectations(t)
}

func TestMissingTournamentTransactionWaitRestartsOnHeadRegression(t *testing.T) {
	s, _ := newPRTServiceMock()
	app := prtRevertTestApp()
	hash := common.HexToHash("0xbeef")
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{Action: tournamentActionStage, Hash: hash}
	client := &ethClientMock{}
	s.client = client
	for i, head := range []uint64{100, 99, 100, 162, 163} {
		expectMissingTournamentTransaction(client, hash)
		_, _, err := s.progressTournamentResult(t.Context(), app, 0, head)
		require.NoError(t, err)
		require.Equal(t, i != 4, s.hasNonRecoveryMutationInFlight(app.ID))
	}
	client.AssertExpectations(t)
}

func TestMissingTournamentTransactionErrorsDoNotReleaseSlot(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		lookup bool
	}{
		{"lookup failure", errors.New("provider unavailable"), true},
		{"receipt failure", errors.New("provider unavailable"), false},
		{"lookup canceled", context.Canceled, true},
		{"receipt deadline", context.DeadlineExceeded, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := newPRTServiceMock()
			app := prtRevertTestApp()
			hash := common.HexToHash("0xbeef")
			s.pendingTransactions[app.ID] = pendingTournamentTransaction{Action: tournamentActionAccept, Hash: hash}
			client := &ethClientMock{}
			s.client = client
			expectMissingTournamentTransaction(client, hash)
			_, _, err := s.progressTournamentResult(t.Context(), app, 0, 100)
			require.NoError(t, err)
			tracked := s.pendingTransactions[app.ID]
			lookupErr := test.err
			if !test.lookup {
				lookupErr = ethereum.NotFound
				client.On("TransactionReceipt", mock.Anything, hash).Return((*types.Receipt)(nil), test.err).Once()
			}
			client.On("TransactionByHash", mock.Anything, hash).Return((*types.Transaction)(nil), false, lookupErr).Once()
			_, _, err = s.progressTournamentResult(t.Context(), app, 0, 164)
			require.ErrorIs(t, err, test.err)
			require.Equal(t, tracked, s.pendingTransactions[app.ID])
			client.AssertExpectations(t)
		})
	}
}

func TestMissingTournamentTransactionUsesFreshEpochAfterRelease(t *testing.T) {
	for _, action := range []tournamentAction{tournamentActionJoin, tournamentActionStage, tournamentActionAccept} {
		t.Run(string(action), func(t *testing.T) {
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			snapshot := resultTestSnapshot(epoch, false)
			snapshot.stage.IsFinished = false
			s, repo, consensus := resultTestService(t, epoch, snapshot, true)
			app := prtRevertTestApp()
			hash := common.HexToHash("0xbeef")
			s.pendingTransactions[app.ID] = pendingTournamentTransaction{Action: action, Hash: hash, EpochIndex: epoch.Index - 1}
			client := &ethClientMock{}
			s.client = client
			for _, head := range []uint64{100, 164} {
				expectMissingTournamentTransaction(client, hash)
				join, recovery, err := s.progressTournamentResult(t.Context(), app, 0, head)
				require.NoError(t, err)
				require.Nil(t, join)
				require.False(t, recovery)
				require.Empty(t, repo.Calls)
				require.Empty(t, consensus.Calls)
			}
			// Another participant accepted the old epoch. Select the new epoch;
			// do not replay the expired action from the old in-memory record.
			join, _, err := s.progressTournamentResult(t.Context(), app, 0, 165)
			require.NoError(t, err)
			require.Same(t, epoch, join)
			require.Empty(t, s.pendingTransactions)
			client.AssertExpectations(t)
			repo.AssertExpectations(t)
			consensus.AssertExpectations(t)
		})
	}
}

func TestMissingStageObservesExternalStageAfterRelease(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	s, repo, consensus := resultTestService(t, epoch, resultTestSnapshot(epoch, true), true)
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 165
	repo.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, epoch.Index, uint64(10)).Return(nil).Once()
	hash := common.HexToHash("0xbeef")
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionStage, Hash: hash, EpochIndex: epoch.Index,
	}
	client := &ethClientMock{}
	s.client = client
	for _, head := range []uint64{100, 164} {
		expectMissingTournamentTransaction(client, hash)
		_, _, err := s.progressTournamentResult(t.Context(), app, head, head)
		require.NoError(t, err)
		require.Empty(t, repo.Calls)
		require.Empty(t, consensus.Calls)
	}
	_, _, err := s.progressTournamentResult(t.Context(), app, 165, 165)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	require.Empty(t, s.pendingTransactions, "the external stage must not trigger another stage broadcast")
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestMissingStageRetriesAfterFreshSnapshotWithEstimationEnabled(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	applyPRTStateProof(epoch, repotest.KeccakStateProof(common.HexToHash("0x1234")))
	s, repo, consensus := resultTestService(t, epoch, resultTestSnapshot(epoch, false), true)
	app := prtRevertTestApp()
	oldHash := common.HexToHash("0xbeef")
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionStage, Hash: oldHash, EpochIndex: epoch.Index,
	}
	client := &ethClientMock{}
	s.client = client
	for _, head := range []uint64{100, 164} {
		expectMissingTournamentTransaction(client, oldHash)
		_, _, err := s.progressTournamentResult(t.Context(), app, 0, head)
		require.NoError(t, err)
		require.Empty(t, repo.Calls)
		require.Empty(t, consensus.Calls)
	}
	newTx := types.NewTx(&types.LegacyTx{Nonce: 8})
	consensus.On("StageTournamentResult", mock.Anything, epoch.Index, mock.Anything).
		Run(func(args mock.Arguments) {
			require.Zero(t, args.Get(0).(*bind.TransactOpts).GasLimit, "a retry must keep gas estimation enabled")
		}).Return(newTx, nil).Once()
	_, _, err := s.progressTournamentResult(t.Context(), app, 0, 165)
	require.NoError(t, err)
	require.Equal(t, pendingTournamentTransaction{
		Action: tournamentActionStage, Hash: newTx.Hash(), EpochIndex: epoch.Index,
	}, s.pendingTransactions[app.ID])
	require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status, "broadcast is not stage confirmation")
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestMissingTournamentTransactionDefersForeclosedBondRecovery(t *testing.T) {
	app := prtForeclosedApp(1, 100)
	app.IConsensusAddress = common.HexToAddress("0x200")
	owned := common.HexToAddress("0x600")
	factory := &adapterFactoryMock{}
	s := newRootBondTestService(owned, factory)
	s.submissionEnabled = true
	tournamentAddress := common.HexToAddress("0x300")
	s.queueRootBondRecovery(app.ID, 3, tournamentAddress)
	hash := common.HexToHash("0xbeef")
	s.pendingTransactions[app.ID] = pendingTournamentTransaction{
		Action: tournamentActionAccept, Hash: hash, EpochIndex: 3,
	}
	client := &ethClientMock{}
	s.client = client
	for _, head := range []uint64{100, 164} {
		client.On("BlockNumber", mock.Anything).Return(head, nil).Once()
		expectMissingTournamentTransaction(client, hash)
		require.NoError(t, s.recoverForeclosedRootBonds(t.Context(), app))
		require.Empty(t, factory.Calls)
		require.Nil(t, s.rootBondRecoveries[app.ID][0].TxHash)
	}
	require.Empty(t, s.pendingTransactions)
	// Recovery starts only after a new tick reads current bond ownership.
	client.On("BlockNumber", mock.Anything).Return(uint64(165), nil).Once()
	consensus := &daveConsensusAdapterMock{}
	consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(165))).
		Return(CurrentSealedEpoch{EpochNumber: 4}, nil).Once()
	tournament := &tournamentAdapterMock{}
	tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(165))).
		Return(canonicalBondRecovery(model.BondDispositionRecoverable, owned, 1), nil).Once()
	refund := types.NewTx(&types.LegacyTx{Nonce: 1})
	tournament.On("TryRecoveringBond", mock.Anything).Return(refund, nil).Once()
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	factory.On("CreateTournamentAdapter", tournamentAddress).Return(tournament, nil).Once()
	require.NoError(t, s.recoverForeclosedRootBonds(t.Context(), app))
	require.Equal(t, refund.Hash(), *s.rootBondRecoveries[app.ID][0].TxHash)
	client.AssertExpectations(t)
	consensus.AssertExpectations(t)
	tournament.AssertExpectations(t)
	factory.AssertExpectations(t)
}
