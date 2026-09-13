// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

func TestCheckedUint64(t *testing.T) {
	require.Equal(t, uint64(42), mustCheckedUint64(t, big.NewInt(42)))
	for _, value := range []*big.Int{
		nil,
		big.NewInt(-1),
		new(big.Int).Lsh(common.Big1, 64),
	} {
		_, err := checkedUint64(value, "test field")
		require.ErrorContains(t, err, "test field is not a uint64")
	}
}

func mustCheckedUint64(t *testing.T, value *big.Int) uint64 {
	t.Helper()
	result, err := checkedUint64(value, "test field")
	require.NoError(t, err)
	return result
}

func TestDaveMachineValidityProofUsesContractOrderAndOwnsSiblings(t *testing.T) {
	proof := model.StateProof{
		IflagsYDataBlock: common.HexToHash("0x11"), IflagsYProof: [][32]byte{{0x12}, {0x13}},
		HtifTohostDataBlock: common.HexToHash("0x21"), HtifTohostProof: [][32]byte{{0x22}, {0x23}},
		TxBufferDataBlock: common.HexToHash("0x31"), TxBufferProof: [][32]byte{{0x32}, {0x33}},
	}
	wire := daveMachineValidityProof(proof)
	require.Equal(t, [32]byte(proof.IflagsYDataBlock), wire.IflagsYProof.DataBlock)
	require.Equal(t, proof.IflagsYProof, wire.IflagsYProof.Siblings)
	require.Equal(t, [32]byte(proof.HtifTohostDataBlock), wire.HtifTohostProof.DataBlock)
	require.Equal(t, proof.HtifTohostProof, wire.HtifTohostProof.Siblings)
	require.Equal(t, [32]byte(proof.TxBufferDataBlock), wire.TxBufferProof.DataBlock)
	require.Equal(t, proof.TxBufferProof, wire.TxBufferProof.Siblings)
	wire.IflagsYProof.Siblings[0][0] ^= 0xff
	wire.HtifTohostProof.Siblings[0][0] ^= 0xff
	wire.TxBufferProof.Siblings[0][0] ^= 0xff
	require.Equal(t, byte(0x12), proof.IflagsYProof[0][0])
	require.Equal(t, byte(0x22), proof.HtifTohostProof[0][0])
	require.Equal(t, byte(0x32), proof.TxBufferProof[0][0])
}

func TestReadDaveConsensusSnapshotUsesOnePinnedCallOptions(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
	snapshot := resultTestSnapshot(epoch, true)
	consensus := &daveConsensusAdapterMock{}
	var first *bind.CallOpts
	checkOpts := func(args mock.Arguments) {
		opts := args.Get(0).(*bind.CallOpts)
		require.Equal(t, int64(20), opts.BlockNumber.Int64())
		if first == nil {
			first = opts
		} else {
			require.Same(t, first, opts)
		}
	}
	consensus.On("GetCurrentSealedEpoch", mock.Anything).Run(checkOpts).Return(snapshot.sealed, nil).Once()
	consensus.On("CanStageTournamentResult", mock.Anything).Run(checkOpts).Return(snapshot.stage, nil).Once()
	consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Run(checkOpts).Return(snapshot.accept, nil).Once()

	got, err := readDaveConsensusSnapshot(context.Background(), consensus, 20)
	require.NoError(t, err)
	require.Equal(t, snapshot, got)
	consensus.AssertExpectations(t)
}

func TestValidateDaveConsensusSnapshotRejectsInconsistentViews(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
	tests := []struct {
		name   string
		mutate func(*daveConsensusSnapshot)
		want   string
	}{
		{name: "epoch number", mutate: func(s *daveConsensusSnapshot) { s.accept.EpochNumber++ }, want: "epoch numbers"},
		{name: "staged flag", mutate: func(s *daveConsensusSnapshot) {
			s.accept.IsTournamentResultStaged = false
		}, want: "staged flags"},
		{name: "staging block", mutate: func(s *daveConsensusSnapshot) { s.sealed.StagingBlockNumber = 21 }, want: "staging block"},
		{name: "staged machine", mutate: func(s *daveConsensusSnapshot) {
			s.accept.StagedPostEpochMachineStateHash[0] ^= 0xff
		}, want: "staged machine state differs"},
		{name: "staged outputs", mutate: func(s *daveConsensusSnapshot) {
			s.accept.StagedPostEpochOutputsMerkleRoot[0] ^= 0xff
		}, want: "staged outputs root differs"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := resultTestSnapshot(epoch, true)
			test.mutate(&snapshot)
			require.ErrorContains(t, validateDaveConsensusSnapshot(snapshot, 20), test.want)
		})
	}
}

func TestProgressTournamentResultReaderReconcilesStagedResult(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 20
	service, repo, consensus := resultTestService(t, epoch, resultTestSnapshot(epoch, true), false)
	repo.On("UpdateEpochReconciledStaged", mock.Anything, epoch.ApplicationID, epoch.Index, uint64(10)).
		Return(nil).Once()

	joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.NoError(t, err)
	require.Nil(t, joinEpoch)
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	require.NotNil(t, epoch.StagedAtBlock)
	require.Equal(t, uint64(10), *epoch.StagedAtBlock)
	consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestProgressTournamentResultReaderDoesNotStageResult(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	service, repo, consensus := resultTestService(t, epoch, resultTestSnapshot(epoch, false), false)
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 20

	joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.NoError(t, err)
	require.Nil(t, joinEpoch)
	consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestProgressTournamentResultUsesLatestWithoutPublishedConsensus(t *testing.T) {
	for _, publishedBlock := range []uint64{0, 99} {
		name := "no published window"
		if publishedBlock != 0 {
			name = "published window precedes deployment"
		}
		t.Run(name, func(t *testing.T) {
			f := newPRTBlockPolicyFixture(t, true)
			f.app.LastTournamentCheckBlock = publishedBlock
			if publishedBlock != 0 {
				f.consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(publishedBlock))).
					Return(CurrentSealedEpoch{}, bind.ErrNoCode).Once()
			}
			latest := resultTestSnapshot(f.epoch, false)
			latest.stage.IsFinished = false
			f.expectSnapshot(120, latest)

			epoch, _, err := f.s.progressTournamentResult(t.Context(), f.app, 100, 120)
			require.NoError(t, err)
			require.Same(t, f.epoch, epoch, "latest join selection must remain available")
			f.assertNoStageWrite(t)
			f.assertNoPermanentStatusWrite(t)
			f.consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(0)))
		})
	}
}

func TestProgressTournamentResultReaderWaitsWithoutPublishedConsensus(t *testing.T) {
	for _, publishedBlock := range []uint64{0, 99} {
		t.Run(new(big.Int).SetUint64(publishedBlock).String(), func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.LastTournamentCheckBlock = publishedBlock
			consensus := &daveConsensusAdapterMock{}
			factory := &adapterFactoryMock{}
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			s.adapterFactory = factory
			if publishedBlock != 0 {
				consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(publishedBlock))).
					Return(CurrentSealedEpoch{}, bind.ErrNoCode).Once()
			}

			epoch, _, err := s.progressTournamentResult(t.Context(), app, 100, 100)
			require.NoError(t, err)
			require.Nil(t, epoch)
			require.Empty(t, repo.Calls)
			consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(100)))
			consensus.AssertExpectations(t)
			factory.AssertExpectations(t)
		})
	}
}

func TestProgressTournamentResultReusesPublishedSnapshotAtLatest(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	f.app.LastTournamentCheckBlock = 100
	snapshot := resultTestSnapshot(f.epoch, false)
	snapshot.stage.IsFinished = false
	f.expectSnapshot(100, snapshot)

	epoch, _, err := f.s.progressTournamentResult(t.Context(), f.app, 100, 100)
	require.NoError(t, err)
	require.Same(t, f.epoch, epoch)
	f.consensus.AssertNumberOfCalls(t, "GetCurrentSealedEpoch", 1)
}

func TestProgressTournamentResultPreservesObservedReadError(t *testing.T) {
	s, repo := newPRTServiceMock()
	s.submissionEnabled = true
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 99
	consensus := &daveConsensusAdapterMock{}
	factory := &adapterFactoryMock{}
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	s.adapterFactory = factory
	consensus.On("GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(99))).
		Return(CurrentSealedEpoch{}, context.DeadlineExceeded).Once()

	epoch, _, err := s.progressTournamentResult(t.Context(), app, 100, 120)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, epoch)
	require.Empty(t, repo.Calls)
	consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.MatchedBy(resultCallOptsAtBlock(120)))
	consensus.AssertExpectations(t)
	factory.AssertExpectations(t)
}

func TestProgressTournamentResultReloadsLatestEpochAfterPublishedResult(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, true)
	f.app.LastTournamentCheckBlock = 99
	observed := resultTestSnapshot(f.epoch, false)
	observed.stage.IsFinished = false
	f.expectSnapshot(99, observed)
	latestEpoch := *f.epoch
	latestEpoch.Index++
	latestEpoch.TournamentAddress = new(common.HexToAddress("0x201"))
	latest := resultTestSnapshot(&latestEpoch, false)
	latest.stage.IsFinished = false
	f.expectSnapshot(120, latest)
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), latestEpoch.Index).Return(&latestEpoch, nil).Once()

	epoch, _, err := f.s.progressTournamentResult(t.Context(), f.app, 100, 120)
	require.NoError(t, err)
	require.Same(t, &latestEpoch, epoch)
	f.assertNoStageWrite(t)
	f.assertNoPermanentStatusWrite(t)
}

func TestProgressTournamentResultWaitsBelowStoredStagingBlock(t *testing.T) {
	f := newPRTBlockPolicyFixture(t, false)
	f.app.LastTournamentCheckBlock = 99
	f.epoch.Status = model.EpochStatus_ClaimStaged
	f.epoch.StagedAtBlock = new(uint64(100))
	older := resultTestSnapshot(f.epoch, true)
	older.sealed.StagingBlockNumber = 90
	f.expectSnapshot(99, older)

	epoch, _, err := f.s.progressTournamentResult(t.Context(), f.app, 101, 101)
	require.NoError(t, err)
	require.Nil(t, epoch)
	require.Equal(t, new(uint64(100)), f.epoch.StagedAtBlock)
	f.assertNoStageWrite(t)
	f.assertNoPermanentStatusWrite(t)
}

func TestProgressTournamentResultRejectsInputBoundsMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*daveConsensusSnapshot)
	}{
		{name: "lower bound", mutate: func(snapshot *daveConsensusSnapshot) {
			snapshot.sealed.InputIndexLowerBound++
		}},
		{name: "upper bound", mutate: func(snapshot *daveConsensusSnapshot) {
			snapshot.sealed.InputIndexUpperBound++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
			snapshot := resultTestSnapshot(epoch, false)
			test.mutate(&snapshot)
			service, repo, consensus := resultTestService(t, epoch, snapshot, true)
			app := prtRevertTestApp()
			app.LastTournamentCheckBlock = 20
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
				mock.MatchedBy(reasonContains("input bounds inconsistent"))).Return(nil).Once()

			joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
			require.Error(t, err)
			require.Nil(t, joinEpoch)
			consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
			consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
			require.Empty(t, service.rootBondRecoveries[app.ID])
			repo.AssertExpectations(t)
			consensus.AssertExpectations(t)
		})
	}
}

func TestProgressTournamentResultKeepsFailedRootObservable(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	snapshot := resultTestSnapshot(epoch, false)
	snapshot.stage.IsTournamentFailed = true
	snapshot.stage.WinnerCommitment = common.Hash{}
	snapshot.stage.WinnerPostEpochMachineStateHash = common.Hash{}
	service, repo, consensus := resultTestService(t, epoch, snapshot, true)
	app := prtRevertTestApp()
	joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	require.Nil(t, joinEpoch)
	consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestProgressTournamentResultAcceptsWhenSentriesAgreeOrPeriodIsOver(t *testing.T) {
	for _, test := range []struct {
		name    string
		agree   bool
		period  bool
		accepts bool
	}{
		{name: "sentries agree", agree: true, accepts: true},
		{name: "zero period is over", period: true, accepts: true},
		{name: "not ready", accepts: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
			snapshot := resultTestSnapshot(epoch, true)
			snapshot.accept.DoAllSentriesAgreeWithStagedTournamentResult = test.agree
			snapshot.accept.IsClaimStagingPeriodOver = test.period
			service, repo, consensus := resultTestService(t, epoch, snapshot, true)
			if test.accepts {
				tx := types.NewTx(&types.LegacyTx{Nonce: 7})
				consensus.On("AcceptStagedTournamentResult", mock.Anything, epoch.Index).Return(tx, nil).Once()
			}

			joinEpoch, _, err := service.progressTournamentResult(context.Background(), prtRevertTestApp(), 20, 20)
			require.NoError(t, err)
			require.Nil(t, joinEpoch)
			if test.accepts {
				require.Contains(t, service.pendingTransactions, epoch.ApplicationID)
			} else {
				require.NotContains(t, service.pendingTransactions, epoch.ApplicationID)
			}
			repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			consensus.AssertExpectations(t)
		})
	}
}

func TestProgressTournamentResultRejectsIncompleteProofBeforeBroadcast(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	service, repo, consensus := resultTestService(t, epoch, resultTestSnapshot(epoch, false), true)
	app := prtRevertTestApp()
	repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
		mock.MatchedBy(reasonContains("cannot stage", "proof is incomplete"))).Return(nil).Once()

	_, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.Error(t, err)
	consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestProgressTournamentResultDefersCompleteProofValidationToContract(t *testing.T) {
	proof := repotest.KeccakStateProof(common.HexToHash("0x1234"))
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	applyPRTStateProof(epoch, proof)
	epoch.TxBufferProof[0][0] ^= 0xff
	corruptSibling := [32]byte(epoch.TxBufferProof[0])
	snapshot := resultTestSnapshot(epoch, false)
	service, repo, consensus := resultTestService(t, epoch, snapshot, true)
	app := prtRevertTestApp()
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(20)}, nil).Once()
	service.client = client
	service.defaultBlock = model.DefaultBlock_Finalized
	opts := mock.MatchedBy(resultCallOptsAtBlock(20))
	consensus.On("GetCurrentSealedEpoch", opts).Return(snapshot.sealed, nil).Once()
	consensus.On("CanStageTournamentResult", opts).Return(snapshot.stage, nil).Once()
	consensus.On("CanAcceptStagedTournamentResult", opts).Return(snapshot.accept, nil).Once()
	factory := service.adapterFactory.(*adapterFactoryMock)
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	consensus.On("StageTournamentResult", mock.Anything, epoch.Index,
		mock.MatchedBy(func(got model.StateProof) bool {
			return got.TxBufferProof[0] == corruptSibling
		})).Return((*types.Transaction)(nil), daveConsensusRevertError("InvalidMachineMerkleProof")).Once()
	repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Failed,
		mock.MatchedBy(reasonContains("Check proof serialization", "stored proof data"))).Return(nil).Once()

	joinEpoch, _, err := service.progressTournamentResult(context.Background(), app, 20, 20)
	require.NoError(t, err)
	require.Nil(t, joinEpoch)
	require.Empty(t, service.pendingTransactions)
	require.Equal(t, model.ApplicationStatus_Failed, app.Status)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
	client.AssertExpectations(t)
	factory.AssertExpectations(t)
}

func TestProgressTournamentResultStagesCompleteProof(t *testing.T) {
	proof := repotest.KeccakStateProof(common.HexToHash("0x1234"))
	epoch := resultTestEpoch(model.EpochStatus_ClaimComputed)
	applyPRTStateProof(epoch, proof)
	snapshot := resultTestSnapshot(epoch, false)
	service, repo, consensus := resultTestService(t, epoch, snapshot, true)
	tx := types.NewTx(&types.LegacyTx{Nonce: 8})
	consensus.On("StageTournamentResult", mock.Anything, epoch.Index,
		mock.MatchedBy(func(got model.StateProof) bool {
			return got.IflagsYDataBlock == proof.IflagsYDataBlock &&
				got.HtifTohostDataBlock == proof.HtifTohostDataBlock &&
				got.TxBufferDataBlock == proof.TxBufferDataBlock &&
				len(got.IflagsYProof) == model.StateProofSiblingCount &&
				len(got.HtifTohostProof) == model.StateProofSiblingCount &&
				len(got.TxBufferProof) == model.StateProofSiblingCount
		})).Return(tx, nil).Once()

	joinEpoch, _, err := service.progressTournamentResult(context.Background(), prtRevertTestApp(), 20, 20)
	require.NoError(t, err)
	require.Nil(t, joinEpoch)
	require.Contains(t, service.pendingTransactions, epoch.ApplicationID)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
}

func TestCheckEpochsAcceptsComputedOrStagedEpochFromExactEpochSealedEvent(t *testing.T) {
	for _, status := range []model.EpochStatus{
		model.EpochStatus_ClaimComputed,
		model.EpochStatus_ClaimStaged,
	} {
		t.Run(status.String(), func(t *testing.T) {
			app := prtRevertTestApp()
			app.LastTournamentCheckBlock = 20
			epoch := resultTestEpoch(status)
			epoch.LastBlock = 10
			acceptTx := common.HexToHash("0x700")
			epoch.ClaimTransactionHash = &acceptTx
			wrongEmitterLog := &types.Log{
				Address: common.HexToAddress("0xbad"), TxHash: acceptTx, BlockNumber: 12, Index: 1,
			}
			wrongHashLog := &types.Log{
				Address: app.IConsensusAddress, TxHash: common.HexToHash("0xbad"), BlockNumber: 12, Index: 2,
			}
			staleLog := &types.Log{Address: app.IConsensusAddress, TxHash: acceptTx, BlockNumber: 12, Index: 3}
			exactLog := &types.Log{Address: app.IConsensusAddress, TxHash: acceptTx, BlockNumber: 12, Index: 4}
			receipt := &types.Receipt{
				Status:      types.ReceiptStatusSuccessful,
				TxHash:      acceptTx,
				BlockNumber: big.NewInt(12),
				Logs:        []*types.Log{wrongEmitterLog, wrongHashLog, staleLog, exactLog},
			}
			staleEvent := &idaveconsensus.IDaveConsensusEpochSealed{
				EpochNumber: big.NewInt(99),
				Raw:         *staleLog,
			}
			exactEvent := &idaveconsensus.IDaveConsensusEpochSealed{
				EpochNumber:             new(big.Int).SetUint64(epoch.Index + 1),
				InitialMachineStateHash: *epoch.MachineHash,
				OutputsMerkleRoot:       *epoch.TxBufferDataBlock,
				Raw:                     *exactLog,
			}

			repo := &prtRepositoryMock{}
			repo.On("ListEpochs", mock.Anything, app.Name,
				mock.MatchedBy(func(filter repository.EpochFilter) bool {
					return filter.HasTournament != nil && *filter.HasTournament && len(filter.Status) == 0
				}), repository.Pagination{}, false).
				Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
				Return(&model.Tournament{
					ApplicationID: app.ID,
					EpochIndex:    epoch.Index,
					Address:       *epoch.TournamentAddress,
					MaxLevel:      3,
					Level:         0,
					Snapshot:      model.TournamentSnapshot{FinishedAtBlock: 9},
				}, nil).Once()
			repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index,
				mock.MatchedBy(func(hash *common.Hash) bool { return hash != nil && *hash == acceptTx })).
				Return(nil).Once()

			consensus := &daveConsensusAdapterMock{}
			consensus.On("ParseEpochSealed", *staleLog).Return(staleEvent, nil).Once()
			consensus.On("ParseEpochSealed", *exactLog).Return(exactEvent, nil).Once()
			factory := &adapterFactoryMock{}
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			client := &ethClientMock{}
			client.On("TransactionReceipt", mock.Anything, acceptTx).Return(receipt, nil).Once()

			service, _ := newPRTServiceMock()
			service.repository = repo
			service.adapterFactory = factory
			service.client = client
			service.submissionEnabled = true
			deferActions, err := service.checkEpochs(context.Background(), app, 20)
			require.NoError(t, err)
			require.False(t, deferActions)
			require.Equal(t, []*rootBondRecovery{{
				EpochIndex: epoch.Index,
				Tournament: *epoch.TournamentAddress,
			}}, service.rootBondRecoveries[app.ID])
			repo.AssertExpectations(t)
			consensus.AssertExpectations(t)
			factory.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestCheckEpochsValidatesAcceptanceReceiptAgainstPinnedHead(t *testing.T) {
	for _, test := range []struct {
		name    string
		receipt *types.Receipt
		wantErr string
	}{
		{
			name: "wrong transaction hash",
			receipt: &types.Receipt{
				Status:      types.ReceiptStatusSuccessful,
				TxHash:      common.HexToHash("0xbad"),
				BlockNumber: big.NewInt(12),
			},
			wantErr: "differs from observed hash",
		},
		{
			name: "receipt after pinned head",
			receipt: &types.Receipt{
				Status:      types.ReceiptStatusSuccessful,
				TxHash:      common.HexToHash("0x700"),
				BlockNumber: big.NewInt(21),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := prtRevertTestApp()
			app.LastTournamentCheckBlock = 20
			epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
			acceptTx := common.HexToHash("0x700")
			epoch.ClaimTransactionHash = &acceptTx
			repo := &prtRepositoryMock{}
			repo.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
				Return(&model.Tournament{Snapshot: model.TournamentSnapshot{FinishedAtBlock: 9}}, nil).Once()
			consensus := &daveConsensusAdapterMock{}
			factory := &adapterFactoryMock{}
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			client := &ethClientMock{}
			client.On("TransactionReceipt", mock.Anything, acceptTx).Return(test.receipt, nil).Once()
			service, _ := newPRTServiceMock()
			service.repository = repo
			service.adapterFactory = factory
			service.client = client

			deferActions, err := service.checkEpochs(context.Background(), app, 20)
			if test.wantErr == "" {
				require.NoError(t, err)
				require.True(t, deferActions)
			} else {
				require.ErrorContains(t, err, test.wantErr)
			}
			repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			consensus.AssertNotCalled(t, "ParseEpochSealed", mock.Anything)
			repo.AssertExpectations(t)
			consensus.AssertExpectations(t)
			factory.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestValidateApplicationDefersActionsForAcceptanceAfterPinnedHead(t *testing.T) {
	app := prtRevertTestApp()
	app.LastTournamentCheckBlock = 20
	epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
	acceptTx := common.HexToHash("0x700")
	epoch.ClaimTransactionHash = &acceptTx
	repo := &prtRepositoryMock{}
	repo.On("ListEpochs", mock.Anything, app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
		Return(&model.Tournament{Snapshot: model.TournamentSnapshot{FinishedAtBlock: 9}}, nil).Once()
	consensus := &daveConsensusAdapterMock{}
	factory := &adapterFactoryMock{}
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	client := &ethClientMock{}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
		Return(&types.Header{Number: big.NewInt(20)}, nil).Once()
	client.On("TransactionReceipt", mock.Anything, acceptTx).Return(&types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		TxHash:      acceptTx,
		BlockNumber: big.NewInt(21),
	}, nil).Once()
	service, _ := newPRTServiceMock()
	service.repository = repo
	service.adapterFactory = factory
	service.client = client
	service.defaultBlock = model.DefaultBlock_Finalized

	require.NoError(t, runPRTApplicationTick(service, app))
	consensus.AssertNotCalled(t, "GetCurrentSealedEpoch", mock.Anything)
	consensus.AssertNotCalled(t, "CanStageTournamentResult", mock.Anything)
	consensus.AssertNotCalled(t, "CanAcceptStagedTournamentResult", mock.Anything)
	consensus.AssertNotCalled(t, "StageTournamentResult", mock.Anything, mock.Anything, mock.Anything)
	consensus.AssertNotCalled(t, "AcceptStagedTournamentResult", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
	factory.AssertExpectations(t)
	client.AssertExpectations(t)
}

func resultCallOptsAtBlock(block uint64) func(*bind.CallOpts) bool {
	return func(opts *bind.CallOpts) bool {
		return opts != nil && opts.BlockNumber != nil && opts.BlockNumber.IsUint64() && opts.BlockNumber.Uint64() == block
	}
}

func resultTestEpoch(status model.EpochStatus) *model.Epoch {
	tournament := common.HexToAddress("0x200")
	commitment := common.HexToHash("0x300")
	machineHash := common.HexToHash("0x400")
	outputsRoot := common.HexToHash("0x500")
	epoch := &model.Epoch{
		ApplicationID:        7,
		Index:                3,
		InputIndexLowerBound: 4,
		InputIndexUpperBound: 9,
		Status:               status,
		TournamentAddress:    &tournament,
		Commitment:           &commitment,
		MachineHash:          &machineHash,
		TxBufferDataBlock:    &outputsRoot,
	}
	if status == model.EpochStatus_ClaimStaged {
		epoch.StagedAtBlock = new(uint64)
		*epoch.StagedAtBlock = 10
	}
	return epoch
}

func resultTestSnapshot(epoch *model.Epoch, staged bool) daveConsensusSnapshot {
	snapshot := daveConsensusSnapshot{
		sealed: CurrentSealedEpoch{
			EpochNumber:              epoch.Index,
			InputIndexLowerBound:     epoch.InputIndexLowerBound,
			InputIndexUpperBound:     epoch.InputIndexUpperBound,
			Tournament:               *epoch.TournamentAddress,
			IsTournamentResultStaged: staged,
		},
		stage: CanStageTournamentResult{
			IsFinished:                      true,
			IsTournamentResultStaged:        staged,
			EpochNumber:                     epoch.Index,
			WinnerCommitment:                *epoch.Commitment,
			WinnerPostEpochMachineStateHash: *epoch.MachineHash,
		},
		accept: CanAcceptStagedTournamentResult{
			IsTournamentResultStaged: staged,
			EpochNumber:              epoch.Index,
		},
	}
	if staged {
		snapshot.sealed.StagingBlockNumber = 10
		snapshot.sealed.StagedPostEpochMachineStateHash = *epoch.MachineHash
		snapshot.sealed.StagedPostEpochOutputsMerkleRoot = *epoch.TxBufferDataBlock
		snapshot.accept.StagedPostEpochMachineStateHash = *epoch.MachineHash
		snapshot.accept.StagedPostEpochOutputsMerkleRoot = *epoch.TxBufferDataBlock
	}
	return snapshot
}

func resultTestService(
	t *testing.T,
	epoch *model.Epoch,
	snapshot daveConsensusSnapshot,
	submissionEnabled bool,
) (*Service, *prtRepositoryMock, *daveConsensusAdapterMock) {
	t.Helper()
	app := prtRevertTestApp()
	app.IConsensusAddress = common.HexToAddress("0x100")
	repo := &prtRepositoryMock{}
	repo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).Return(epoch, nil).Once()
	consensus := &daveConsensusAdapterMock{}
	consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(snapshot.sealed, nil).Once()
	consensus.On("CanStageTournamentResult", mock.Anything).Return(snapshot.stage, nil).Once()
	consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(snapshot.accept, nil).Once()
	factory := &adapterFactoryMock{}
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	service, _ := newPRTServiceMock()
	service.repository = repo
	service.adapterFactory = factory
	service.submissionEnabled = submissionEnabled
	service.submissionTimeout = time.Second
	service.txOptsFactory = ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x600")})
	return service, repo, consensus
}

func applyPRTStateProof(epoch *model.Epoch, proof model.StateProof) {
	epoch.MachineHash = new(common.Hash)
	*epoch.MachineHash = proof.MachineHash
	epoch.TxBufferDataBlock = new(common.Hash)
	*epoch.TxBufferDataBlock = proof.TxBufferDataBlock
	epoch.TxBufferProof = stateProofHashes(proof.TxBufferProof)
	epoch.IflagsYDataBlock = new(common.Hash)
	*epoch.IflagsYDataBlock = proof.IflagsYDataBlock
	epoch.IflagsYProof = stateProofHashes(proof.IflagsYProof)
	epoch.HtifTohostDataBlock = new(common.Hash)
	*epoch.HtifTohostDataBlock = proof.HtifTohostDataBlock
	epoch.HtifTohostProof = stateProofHashes(proof.HtifTohostProof)
}

func stateProofHashes(proof [][32]byte) []common.Hash {
	result := make([]common.Hash, len(proof))
	for i := range proof {
		result[i] = proof[i]
	}
	return result
}
