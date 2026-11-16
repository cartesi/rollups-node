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

func TestValidatorIORSignalRequiresCommittedClaim(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored", true: "store failed"}[fail], func(t *testing.T) {
			repo, s, app, epoch := rootValidationFixture()
			epoch.TxBufferDataBlock = new(common.Hash{})
			input := &model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}
			expectRootValidationReads(repo, app, epoch, input, nil)
			var output bytes.Buffer
			s.Logger = slog.New(slog.NewTextHandler(&output, nil))
			var failure error
			if fail {
				failure = errors.New("claim transaction failed")
			}
			repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(failure).Once()
			err := s.validateApplication(t.Context(), app)
			if fail {
				require.ErrorIs(t, err, failure)
				require.NotContains(t, output.String(), "condition=ior_settlement_withheld")
			} else {
				require.NoError(t, err)
				require.Contains(t, output.String(), "condition=ior_settlement_withheld")
				require.Contains(t, output.String(), "join_state=not_checked")
				require.Contains(t, output.String(), "declared_outputs_root=")
				require.Contains(t, output.String(), "calculated_outputs_root=")
			}
			require.NotContains(t, output.String(), "PRT protection stopped")
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertExpectations(t)
		})
	}
}
