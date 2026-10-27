// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
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
	consensus := newCompatibleDaveConsensusAdapterMock()
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

type inconsistentConsensusView struct {
	name   string
	mutate func(*daveConsensusSnapshot)
	want   string
}

func inconsistentConsensusViews() []inconsistentConsensusView {
	return []inconsistentConsensusView{
		{name: "epoch number", mutate: func(s *daveConsensusSnapshot) { s.accept.EpochNumber++ }, want: "epoch numbers"},
		{name: "staged flag", mutate: func(s *daveConsensusSnapshot) {
			s.accept.IsTournamentResultStaged = false
		}, want: "staged flags"},
		{name: "staged machine", mutate: func(s *daveConsensusSnapshot) {
			s.accept.StagedPostEpochMachineStateHash[0] ^= 0xff
		}, want: "staged machine state differs"},
		{name: "staged outputs", mutate: func(s *daveConsensusSnapshot) {
			s.accept.StagedPostEpochOutputsMerkleRoot[0] ^= 0xff
		}, want: "staged outputs root differs"},
		{name: "winner machine", mutate: func(s *daveConsensusSnapshot) {
			s.stage.WinnerPostEpochMachineStateHash[0] ^= 0xff
		}, want: "differs from the tournament winner"},
	}
}

func TestValidateDaveConsensusSnapshotRejectsInconsistentViews(t *testing.T) {
	epoch := resultTestEpoch(model.EpochStatus_ClaimStaged)
	for _, test := range inconsistentConsensusViews() {
		t.Run(test.name, func(t *testing.T) {
			snapshot := resultTestSnapshot(epoch, true)
			test.mutate(&snapshot)
			err := validateDaveConsensusSnapshot(snapshot, 20)
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.ErrorContains(t, err, test.want)
		})
	}
	t.Run("invalid staging block", func(t *testing.T) {
		snapshot := resultTestSnapshot(epoch, true)
		snapshot.sealed.StagingBlockNumber = 21
		err := validateDaveConsensusSnapshot(snapshot, 20)
		require.ErrorContains(t, err, "staging block")
		require.NotErrorIs(t, err, txsender.ErrInconclusive, "structurally invalid data is distinct from disagreeing views")
	})
}

// These cases exercise the separated observer and planner. Latest-view
// failures never publish a durable application diagnosis.
func TestTournamentResultPlansPreserveConsensusReadiness(t *testing.T) {
	for _, test := range []struct {
		name                                    string
		kind                                    actionKind
		staged, agree, period, finished, failed bool
		want                                    bool
	}{
		{name: "stage complete result", kind: stageTournament, finished: true, want: true},
		{name: "unfinished result cannot stage", kind: stageTournament},
		{name: "failed root remains observable", kind: stageTournament, finished: true, failed: true},
		{name: "already staged cannot stage again", kind: stageTournament, staged: true, finished: true},
		{name: "sentries agree", kind: acceptTournament, staged: true, finished: true, agree: true, want: true},
		{name: "staging period over", kind: acceptTournament, staged: true, finished: true, period: true, want: true},
		{name: "staging period still running", kind: acceptTournament, staged: true, finished: true},
		{name: "accept requires staged result", kind: acceptTournament, finished: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, test.staged)
			view.stage.IsFinished, view.stage.IsTournamentFailed = test.finished, test.failed
			view.accept.DoAllSentriesAgreeWithStagedTournamentResult, view.accept.IsClaimStagingPeriodOver = test.agree, test.period
			f.snapshot(view)
			call, err := f.d.planAction(t.Context(), f.app, test.kind, f.block, observerFacts(100))
			require.NoError(t, err)
			require.Equal(t, test.want, call != nil)
			if test.want {
				require.Equal(t, test.kind, call.Key.Kind)
				require.Equal(t, f.epoch.Index, call.Key.Epoch)
				require.Equal(t, f.app.IConsensusAddress, call.To)
				abi, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
				require.NoError(t, err)
				method, err := abi.MethodById(call.Data[:4])
				require.NoError(t, err)
				if test.kind == stageTournament {
					require.Equal(t, "stageTournamentResult", method.Name)
				} else {
					require.Equal(t, "acceptStagedTournamentResult", method.Name)
				}
				values, err := method.Inputs.Unpack(call.Data[4:])
				require.NoError(t, err)
				require.Equal(t, f.epoch.Index, values[0].(*big.Int).Uint64())
			}
			require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestTournamentResultLatestMismatchIsPure(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*daveConsensusSnapshot)
	}{
		{"lower input bound", func(v *daveConsensusSnapshot) { v.sealed.InputIndexLowerBound++ }},
		{"upper input bound", func(v *daveConsensusSnapshot) { v.sealed.InputIndexUpperBound++ }},
		{"staged winner state", func(v *daveConsensusSnapshot) { v.stage.WinnerPostEpochMachineStateHash[0] ^= 1 }},
		{"staged output root", func(v *daveConsensusSnapshot) {
			v.sealed.StagedPostEpochOutputsMerkleRoot[0] ^= 1
			v.accept.StagedPostEpochOutputsMerkleRoot = v.sealed.StagedPostEpochOutputsMerkleRoot
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, true)
			test.mutate(&view)
			f.snapshot(view)
			call, err := f.d.planAction(t.Context(), f.app, acceptTournament, f.block, observerFacts(100))
			require.Error(t, err)
			require.Nil(t, call)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestTournamentResultObservationRecordsOnlyPublishedStage(t *testing.T) {
	for _, test := range []struct {
		name   string
		staged bool
		stored uint64
	}{
		{"matching published stage", true, 0}, {"reader remains passive", false, 0}, {"stored stage is newer", true, 101},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDeliveryFixture(t)
			f.d.submissionEnabled = false
			if test.stored != 0 {
				f.epoch.Status, f.epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(test.stored)
			}
			view := resultTestSnapshot(f.epoch, test.staged)
			opts := mock.MatchedBy(resultCallOptsAtBlock(100))
			f.consensus.On("GetCurrentSealedEpoch", opts).Return(view.sealed, nil).Once()
			f.consensus.On("CanStageTournamentResult", opts).Return(view.stage, nil).Once()
			f.consensus.On("CanAcceptStagedTournamentResult", opts).Return(view.accept, nil).Once()
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
				repository.Pagination{}, false).Return([]*model.Epoch{f.epoch}, uint64(1), nil).Once()
			if test.staged && test.stored == 0 {
				f.repo.On("UpdateEpochReconciledStaged", mock.Anything, f.app.ID, f.epoch.Index, uint64(10)).Return(nil).Once()
			}
			expectProtectionReads(t, f.factory, f.consensus, observerFacts(100), f.epoch)
			require.NoError(t, f.d.observeApplication(t.Context(), f.app, observerFacts(100)))
			switch {
			case test.staged && test.stored == 0:
				require.Equal(t, model.EpochStatus_ClaimStaged, f.epoch.Status)
				require.Equal(t, new(uint64(10)), f.epoch.StagedAtBlock)
			case test.stored != 0:
				require.Equal(t, new(test.stored), f.epoch.StagedAtBlock)
			default:
				require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
			}
			require.Empty(t, f.backend.published)
			f.repo.AssertExpectations(t)
			f.consensus.AssertExpectations(t)
		})
	}
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
			header := &types.Header{Number: big.NewInt(12)}
			wrongEmitterLog := &types.Log{
				Address: common.HexToAddress("0xbad"), TxHash: acceptTx, BlockNumber: 12, Index: 1,
			}
			wrongHashLog := &types.Log{
				Address: app.IConsensusAddress, TxHash: common.HexToHash("0xbad"), BlockNumber: 12, Index: 2,
			}
			staleLog := &types.Log{Address: app.IConsensusAddress, TxHash: acceptTx, BlockNumber: 12, BlockHash: header.Hash(), Index: 3}
			exactLog := &types.Log{Address: app.IConsensusAddress, TxHash: acceptTx, BlockNumber: 12, BlockHash: header.Hash(), Index: 4}
			receipt := &types.Receipt{
				Status:      types.ReceiptStatusSuccessful,
				TxHash:      acceptTx,
				BlockNumber: big.NewInt(12),
				BlockHash:   header.Hash(),
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
					Snapshot:      model.TournamentSnapshot{FinishedAtBlock: 9, AsOfBlock: 20},
				}, nil).Once()
			repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index,
				mock.MatchedBy(func(hash *common.Hash) bool { return hash != nil && *hash == acceptTx })).
				Return(nil).Once()

			consensus := newCompatibleDaveConsensusAdapterMock()
			consensus.On("ParseEpochSealed", *staleLog).Return(staleEvent, nil).Once()
			consensus.On("ParseEpochSealed", *exactLog).Return(exactEvent, nil).Once()
			factory := &adapterFactoryMock{}
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			client := &ethClientMock{}
			client.On("TransactionReceipt", mock.Anything, acceptTx).Return(receipt, nil).Once()
			client.On("HeaderByNumber", mock.Anything, receipt.BlockNumber).Return(header, nil).Once()

			service, _ := newPRTServiceMock()
			service.repository = repo
			service.adapterFactory = factory
			service.client = client
			service.submissionEnabled = true
			deferActions, err := observeEpochsForTest(context.Background(), service, app, 20)
			require.NoError(t, err)
			require.False(t, deferActions)
			require.Empty(t, service.delivery.rootBondRecoveryScans, "acceptance is not a recovery discovery source")
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
				Return(&model.Tournament{ApplicationID: app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
					Snapshot: model.TournamentSnapshot{FinishedAtBlock: 9, AsOfBlock: 20}}, nil).Once()
			consensus := newCompatibleDaveConsensusAdapterMock()
			factory := &adapterFactoryMock{}
			factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
			client := &ethClientMock{}
			client.On("TransactionReceipt", mock.Anything, acceptTx).Return(test.receipt, nil).Once()
			service, _ := newPRTServiceMock()
			service.repository = repo
			service.adapterFactory = factory
			service.client = client

			deferActions, err := observeEpochsForTest(context.Background(), service, app, 20)
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
		Return(&model.Tournament{ApplicationID: app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
			Snapshot: model.TournamentSnapshot{FinishedAtBlock: 9, AsOfBlock: 20}}, nil).Once()
	consensus := newCompatibleDaveConsensusAdapterMock()
	view := resultTestSnapshot(epoch, true)
	consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil).Once()
	consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil).Once()
	consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil).Once()
	factory := &adapterFactoryMock{}
	factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil).Once()
	client := &ethClientMock{}
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

	expectProtectionReads(t, factory, consensus, observerFacts(20), epoch)
	require.NoError(t, service.delivery.observeApplication(t.Context(), app, observerFacts(20)))
	consensus.AssertNumberOfCalls(t, "GetCurrentSealedEpoch", 2)
	consensus.AssertNumberOfCalls(t, "CanStageTournamentResult", 1)
	consensus.AssertNumberOfCalls(t, "CanAcceptStagedTournamentResult", 1)
	repo.AssertExpectations(t)
	consensus.AssertExpectations(t)
	factory.AssertExpectations(t)
	client.AssertExpectations(t)
}

func resultCallOptsAtBlock(block uint64) func(*bind.CallOpts) bool {
	return func(opts *bind.CallOpts) bool {
		if opts == nil {
			return false
		}
		if opts.BlockHash != (common.Hash{}) {
			pinned, ok := opts.Context.Value(liveBlockContextKey{}).(liveBlock)
			return ok && pinned.number == block && pinned.hash == opts.BlockHash && opts.BlockNumber == nil
		}
		return opts.BlockNumber != nil && opts.BlockNumber.IsUint64() && opts.BlockNumber.Uint64() == block
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
		IflagsYDataBlock:     new(common.Hash),
		HtifTohostDataBlock:  new(common.Hash),
		TxBufferProof:        make([]common.Hash, model.StateProofSiblingCount),
		IflagsYProof:         make([]common.Hash, model.StateProofSiblingCount),
		HtifTohostProof:      make([]common.Hash, model.StateProofSiblingCount),
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

func TestTournamentResultStageProofPreservesContractValidation(t *testing.T) {
	for _, incomplete := range []bool{true, false} {
		t.Run(fmt.Sprint(incomplete), func(t *testing.T) {
			f := newDeliveryFixture(t)
			if incomplete {
				f.epoch.TxBufferProof = nil
			} else {
				f.epoch.TxBufferProof[0][0] ^= 1
			}
			f.snapshot(resultTestSnapshot(f.epoch, false))
			call, err := f.d.planAction(t.Context(), f.app, stageTournament, f.block, observerFacts(100))
			if incomplete {
				require.Error(t, err)
				require.Nil(t, call)
			} else {
				require.NoError(t, err)
				require.NotNil(t, call)
				proof, err := f.epoch.StateProof()
				require.NoError(t, err)
				abi, err := idaveconsensus.IDaveConsensusMetaData.GetAbi()
				require.NoError(t, err)
				expected, err := abi.Pack("stageTournamentResult", new(big.Int).SetUint64(f.epoch.Index), daveMachineValidityProof(proof))
				require.NoError(t, err)
				require.Equal(t, expected, call.Data)
				failure := daveConsensusRevertError("InvalidMachineMerkleProof")
				f.backend.preflightError = failure
				report, err := f.d.sender.PublishNext(t.Context(), singleAction{f.d, *call})
				require.ErrorIs(t, err, failure)
				require.False(t, report.Attempted)
				require.Empty(t, f.backend.published)
			}
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
