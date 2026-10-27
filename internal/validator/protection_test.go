// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func expectProtectionRoots(repo *Mockrepo, app *model.Application) {
	var epochs []*model.Epoch
	for index := uint64(0); index < 3; index++ {
		root := common.Address{byte(index + 1)}
		epochs = append(epochs, &model.Epoch{ApplicationID: app.ID, Index: index,
			LastBlock: 100 + index, TournamentAddress: &root, Status: model.EpochStatus_Closed})
		if index > 0 {
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), root.Hex()).Return(&model.Tournament{
				ApplicationID: app.ID, EpochIndex: index, Address: root, StartInstant: 100 + index, Allowance: 50,
			}, nil).Once()
		}
	}
	repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), repository.EpochFilter{
		HasTournament: new(true), Status: model.NonTerminalEpochStatuses(),
	}, repository.Pagination{}, false).Return(epochs, uint64(len(epochs)), nil).Once()
}

func TestValidatorInvalidRootProtectionStartsAtCommittedCutoff(t *testing.T) {
	for _, storeFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "write failed"}[storeFailure], func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			app.ConsensusType = model.Consensus_PRT
			epoch.Index = 1
			zero := common.Hash{}
			epoch.TxBufferDataBlock = &zero
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(),
				repository.EpochFilter{Status: []model.EpochStatus{model.EpochStatus_InputsProcessed}},
				repository.Pagination{}, false).
				Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
			expectEpochOutputs(repo, app, epoch, nil)
			repo.On("GetLastInput", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).
				Return(&model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}, nil).Once()
			var storeErr error
			if storeFailure {
				storeErr = errors.New("atomic root finding was not saved")
			} else {
				expectProtectionRoots(repo, app)
			}
			repo.On("StoreInvalidOutputsRoot", mock.Anything, app.ID, epoch.Index, mock.Anything).Return(storeErr).Once()
			err := s.validateApplication(t.Context(), app)
			require.Error(t, err)
			if storeFailure {
				require.ErrorIs(t, err, storeErr)
				require.NotContains(t, output.String(), "PRT protection stopped")
			} else {
				require.Contains(t, output.String(), "join_close_block=151")
				require.Contains(t, output.String(), "join_close_block=152")
				require.NotContains(t, output.String(), "join_close_block=150")
				repo.AssertNumberOfCalls(t, "GetTournament", 2)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestValidatorIntegrityAndProofFailureProtection(t *testing.T) {
	for _, proofFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "corruption", true: "proof failure"}[proofFailure], func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			app.ConsensusType = model.Consensus_PRT
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			root := common.HexToAddress("0x777")
			obligation := &model.Epoch{ApplicationID: app.ID, Index: 3, LastBlock: 100,
				TournamentAddress: &root, Status: model.EpochStatus_Closed}
			repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), repository.EpochFilter{
				HasTournament: new(true), Status: model.NonTerminalEpochStatuses(),
			}, repository.Pagination{}, false).Return([]*model.Epoch{obligation}, uint64(1), nil).Once()
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), root.Hex()).Return(&model.Tournament{
				ApplicationID: app.ID, EpochIndex: 3, Address: root, StartInstant: 100, Allowance: 50,
			}, nil).Once()
			status := model.ApplicationStatus_Corrupted
			if proofFailure {
				status = model.ApplicationStatus_Failed
			}
			writeErr := errors.New("status write failed")
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, status, mock.Anything).Return(writeErr).Once()
			var err error
			if proofFailure {
				_, _, err = s.proveCommitment(t.Context(), app, epoch,
					&merkle.Tree{Height: uint32(model.Log2EpochComputationHashLeafCount)})
			} else {
				err = s.setApplicationCorrupted(t.Context(), app, "stored root missing")
			}
			require.ErrorIs(t, err, writeErr)
			require.Contains(t, output.String(), "join_close_block=150")
			require.Contains(t, output.String(), "epoch_index=3")
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertExpectations(t)
		})
	}
}
