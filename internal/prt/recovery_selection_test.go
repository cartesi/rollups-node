// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func queueRecoverableOlderBond(t *testing.T, f *prtBlockPolicyFixture) *tournamentAdapterMock {
	t.Helper()
	address := common.HexToAddress("0x900")
	f.s.queueRootBondRecovery(f.app.ID, f.epoch.Index-1, address)
	f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(100))).
		Return(CurrentSealedEpoch{EpochNumber: f.epoch.Index}, nil).Maybe()
	tournament := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", address).Return(tournament, nil).Maybe()
	tournament.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(100))).
		Return(canonicalBondRecovery(BondDispositionRecoverable, f.s.txOptsFactory.From(), 1), nil).Maybe()
	tournament.On("TryRecoveringBond", mock.Anything).
		Return(types.NewTx(&types.LegacyTx{Nonce: 9}), nil).Maybe()
	return tournament
}

func TestRecoveryDefersUntilCurrentClaimIsReady(t *testing.T) {
	for _, status := range []EpochStatus{EpochStatus_Open, EpochStatus_Closed, EpochStatus_InputsProcessed} {
		t.Run(status.String(), func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			f.epoch.Status = status
			snapshot := resultTestSnapshot(f.epoch, false)
			snapshot.stage.IsFinished = false
			f.expectTick(100, 100, snapshot, snapshot)
			bond := queueRecoverableOlderBond(t, f)

			require.NoError(t, f.tick(100))
			bond.AssertNotCalled(t, "TryRecoveringBond", mock.Anything)
			require.Nil(t, f.s.rootBondRecoveries[f.app.ID][0].TxHash)
		})
	}
}

func TestRecoveryDefersAfterMinedTournamentTransaction(t *testing.T) {
	for _, action := range []tournamentAction{tournamentActionJoin, tournamentActionStage, tournamentActionAccept} {
		t.Run(string(action), func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			f.epoch.Status = EpochStatus_ClaimStaged
			f.epoch.StagedAtBlock = new(uint64(10))
			tx := types.NewTx(&types.LegacyTx{Nonce: 1})
			f.s.pendingTransactions[f.app.ID] = pendingTournamentTransaction{
				Action: action, Hash: tx.Hash(), EpochIndex: f.epoch.Index,
			}
			f.client.On("TransactionByHash", mock.Anything, tx.Hash()).Return(tx, false, nil).Once()
			f.client.On("TransactionReceipt", mock.Anything, tx.Hash()).Return(&types.Receipt{
				TxHash: tx.Hash(), BlockNumber: big.NewInt(99), Status: types.ReceiptStatusSuccessful,
			}, nil).Once()
			f.expectHead(100)
			f.client.On("BlockNumber", mock.Anything).Return(uint64(100), nil).Once()
			// The next tick must read fresh state and send the eligible accept.
			snapshot := resultTestSnapshot(f.epoch, true)
			snapshot.accept.IsClaimStagingPeriodOver = true
			f.expectTick(100, 100, snapshot, snapshot)
			bond := queueRecoverableOlderBond(t, f)
			f.consensus.On("AcceptStagedTournamentResult", mock.Anything, f.epoch.Index).
				Return(types.NewTx(&types.LegacyTx{Nonce: 2}), nil).Once()

			require.NoError(t, f.tick(100))
			bond.AssertNotCalled(t, "TryRecoveringBond", mock.Anything)
			require.Empty(t, f.s.pendingTransactions)
			require.NoError(t, f.tick(100))
			require.Equal(t, tournamentActionAccept, f.s.pendingTransactions[f.app.ID].Action)
			bond.AssertNotCalled(t, "TryRecoveringBond", mock.Anything)
		})
	}
}

func TestFailedRootAllowsRecoveryWithoutPreparedClaim(t *testing.T) {
	for _, name := range []string{"missing", "open", "closed", "inputs processed"} {
		t.Run(name, func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			snapshot := resultTestSnapshot(f.epoch, false)
			snapshot.stage.IsTournamentFailed = true
			snapshot.stage.WinnerCommitment = common.Hash{}
			snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
			switch name {
			case "missing":
				f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index).Unset()
				f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index).
					Return((*Epoch)(nil), nil)
			case "open":
				f.epoch.Status = EpochStatus_Open
			case "closed":
				f.epoch.Status = EpochStatus_Closed
			case "inputs processed":
				f.epoch.Status = EpochStatus_InputsProcessed
			}
			f.expectTick(100, 100, snapshot, snapshot)
			bond := queueRecoverableOlderBond(t, f)

			require.NoError(t, f.tick(100))
			bond.AssertNumberOfCalls(t, "TryRecoveringBond", 1)
			f.assertNoPermanentStatusWrite(t)
			f.consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
			f.consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
		})
	}
}

func TestRecoveryDefersWhenJoinBroadcastIsDeferred(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	snapshot := resultTestSnapshot(f.epoch, false)
	snapshot.stage.IsFinished = false
	f.expectTick(100, 100, snapshot, snapshot)
	bond := queueRecoverableOlderBond(t, f)
	f.repo.On("GetCommitment", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index,
		f.epoch.TournamentAddress.Hex(), f.epoch.Commitment.Hex()).Return((*Commitment)(nil), nil).Once()
	tournament := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(tournament, nil).Once()
	tournament.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).
		Return(CommitmentStanding{}, nil).Once()
	tournament.On("Descriptor", mock.Anything).
		Return(TournamentDescriptor{Height: Log2EpochComputationHashLeafCount}, nil).Once()
	tournament.On("BondValue", mock.Anything).Return(big.NewInt(1), nil).Once()
	tournament.On("JoinTournament", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((*types.Transaction)(nil), errors.New("nonce too low")).Once()

	require.NoError(t, f.tick(100))
	require.Empty(t, f.s.pendingTransactions)
	bond.AssertNotCalled(t, "TryRecoveringBond", mock.Anything)
	tournament.AssertExpectations(t)
}

func TestRecoveryDefersWhenResultBroadcastIsDeferred(t *testing.T) {
	for _, action := range []tournamentAction{tournamentActionStage, tournamentActionAccept} {
		t.Run(string(action), func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			staged := action == tournamentActionAccept
			if staged {
				f.epoch.Status = EpochStatus_ClaimStaged
				f.epoch.StagedAtBlock = new(uint64(10))
			}
			snapshot := resultTestSnapshot(f.epoch, staged)
			snapshot.accept.IsClaimStagingPeriodOver = staged
			f.expectTick(100, 100, snapshot, snapshot)
			bond := queueRecoverableOlderBond(t, f)
			if staged {
				f.consensus.On("AcceptStagedTournamentResult", mock.Anything, f.epoch.Index).
					Return((*types.Transaction)(nil), errors.New("nonce too low")).Once()
			} else {
				f.consensus.On("StageTournamentResult", mock.Anything, f.epoch.Index, mock.Anything).
					Return((*types.Transaction)(nil), errors.New("nonce too low")).Once()
			}

			require.NoError(t, f.tick(100))
			require.Empty(t, f.s.pendingTransactions)
			bond.AssertNotCalled(t, "TryRecoveringBond", mock.Anything)
			candidates := f.s.rootBondRecoveries[f.app.ID]
			wantCandidates := 1
			if staged {
				// Acceptance queues this root before broadcast. A soft error must
				// retain it without allowing a refund to take the next nonce.
				wantCandidates++
			}
			require.Len(t, candidates, wantCandidates)
			require.Equal(t, f.epoch.Index-1, candidates[0].EpochIndex)
			for _, candidate := range candidates {
				require.Nil(t, candidate.TxHash)
			}
			f.assertNoPermanentStatusWrite(t)
		})
	}
}

func TestRecoveryRunsDuringKnownSafeTournamentWait(t *testing.T) {
	for _, staged := range []bool{false, true} {
		name := "joined and waiting for tournament"
		if staged {
			name = "waiting for staging period"
		}
		t.Run(name, func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			snapshot := resultTestSnapshot(f.epoch, staged)
			if staged {
				f.epoch.Status = EpochStatus_ClaimStaged
				f.epoch.StagedAtBlock = new(uint64(10))
			} else {
				snapshot.stage.IsFinished = false
				f.repo.On("GetCommitment", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index,
					f.epoch.TournamentAddress.Hex(), f.epoch.Commitment.Hex()).Return(&Commitment{}, nil).Once()
			}
			f.expectTick(100, 100, snapshot, snapshot)
			bond := queueRecoverableOlderBond(t, f)

			require.NoError(t, f.tick(100))
			bond.AssertNumberOfCalls(t, "TryRecoveringBond", 1)
		})
	}
}

func TestPendingRecoveryIsPolledAndReportsTournamentYield(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	// No result snapshot is required while the already-sent refund is pending.
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index).Unset()
	f.factory.On("CreateDaveConsensusAdapter", f.app.IConsensusAddress).Unset()
	tx := types.NewTx(&types.LegacyTx{Nonce: 1})
	hash := tx.Hash()
	f.s.rootBondRecoveries = map[int64][]*rootBondRecovery{f.app.ID: {{
		EpochIndex: f.epoch.Index - 1, Tournament: common.HexToAddress("0x900"), TxHash: &hash,
	}}}
	f.expectHead(100)
	f.client.On("BlockNumber", mock.Anything).Return(uint64(100), nil).Once()
	f.client.On("TransactionByHash", mock.Anything, hash).Return(tx, true, nil).Once()
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))

	require.NoError(t, f.tick(100))
	require.Contains(t, output.String(), "Tournament actions wait for pending root bond recovery")
	require.Contains(t, output.String(), "level=INFO")
	require.Contains(t, output.String(), hash.Hex())
	f.consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.Anything)
}

func TestRecoveryRunsWhenJoinIsAwaitingEventSync(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	snapshot := resultTestSnapshot(f.epoch, false)
	snapshot.stage.IsFinished = false
	f.expectTick(100, 100, snapshot, snapshot)
	bond := queueRecoverableOlderBond(t, f)
	f.repo.On("GetCommitment", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index,
		f.epoch.TournamentAddress.Hex(), f.epoch.Commitment.Hex()).Return((*Commitment)(nil), nil).Once()
	tournament := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(tournament, nil).Once()
	tournament.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).
		Return(CommitmentStanding{Joined: true, FinalState: *f.epoch.MachineHash}, nil).Once()

	require.NoError(t, f.tick(100))
	bond.AssertNumberOfCalls(t, "TryRecoveringBond", 1)
	tournament.AssertNotCalled(t, "JoinTournament", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	tournament.AssertExpectations(t)
}
