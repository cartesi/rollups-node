// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestValidateInvalidOutputsRoot(t *testing.T) {
	for _, consensus := range model.ConsensusAllValues {
		t.Run(consensus.String(), func(t *testing.T) {
			for _, changedOutputs := range []bool{false, true} {
				name := "declared root differs"
				if changedOutputs {
					name = "stored output bytes differ"
				}
				t.Run(name, func(t *testing.T) {
					repo, validator, app, epoch := rootValidationFixture()
					app.ConsensusType = consensus
					declared := common.Hash{}
					var outputs []*model.Output
					if changedOutputs {
						declared = validator.pristineRootHash
						outputs = []*model.Output{{Index: 0, RawData: []byte("stored bytes")}}
					}
					epoch.TxBufferDataBlock = &declared
					input := &model.Input{
						Index: 0, Status: model.InputCompletionStatus_Accepted,
						MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock,
					}
					expectRootValidationReads(repo, app, epoch, input, outputs)
					repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_InvalidOutputsRoot,
						mock.MatchedBy(func(reason *string) bool {
							return reason != nil && strings.Contains(*reason, "declared="+declared.Hex()) &&
								strings.Contains(*reason, "calculated=")
						})).Return(nil).Once()

					err := validator.validateApplication(t.Context(), app)

					require.ErrorContains(t, err, "epoch 0: declared outputs root does not match the root calculated from stored outputs")
					require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, app.Status)
					require.Equal(t, err.Error(), *app.Reason)
					require.NotContains(t, *app.Reason, "guest")
					require.NotContains(t, *app.Reason, "corrupt")
					require.Equal(t, model.InputCompletionStatus_Accepted, input.Status)
					require.Equal(t, model.EpochStatus_InputsProcessed, epoch.Status)
					repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
					repo.AssertNotCalled(t, "ListStateHashes", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false)
					repo.AssertExpectations(t)
				})
			}
		})
	}
}

func TestValidateEmptyFirstEpochInvalidOutputsRoot(t *testing.T) {
	repo, validator, app, epoch := rootValidationFixture()
	app.ConsensusType = model.Consensus_PRT
	zero := common.Hash{}
	epoch.TxBufferDataBlock = &zero
	expectRootValidationReads(repo, app, epoch, nil, nil)
	reason := "epoch 0: declared outputs root does not match the root calculated from stored outputs; declared=" +
		zero.Hex() + "; calculated=" + validator.pristineRootHash.Hex()
	repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_InvalidOutputsRoot,
		mock.MatchedBy(func(value *string) bool { return value != nil && *value == reason })).Return(nil).Once()

	err := validator.validateApplication(t.Context(), app)

	require.EqualError(t, err, reason)
	require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, app.Status)
	require.Equal(t, reason, *app.Reason)
	repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestValidateStoredIdentityBeforeOutputsRootRule(t *testing.T) {
	for _, tc := range []struct {
		name         string
		empty        bool
		machineError bool
		wantReason   string
	}{
		{name: "epoch word differs from input", wantReason: "outputs merkle root does not match last input"},
		{name: "epoch machine differs from input", machineError: true, wantReason: "machine hash does not match epoch last input"},
		{name: "empty epoch machine differs from template", empty: true, machineError: true,
			wantReason: "machine hash does not match for application template hash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, validator, app, epoch := rootValidationFixture()
			app.ConsensusType = model.Consensus_PRT
			zero := common.Hash{}
			epoch.TxBufferDataBlock = &zero // Also fails the calculated-root rule.
			input := &model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: &validator.pristineRootHash}
			if tc.machineError {
				input.TxBufferDataBlock = epoch.TxBufferDataBlock
				epoch.MachineHash = new(common.HexToHash("0xbad"))
			}
			if tc.empty {
				input = nil
			}
			expectRootValidationReads(repo, app, epoch, input, nil)
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, model.ApplicationStatus_Corrupted,
				mock.MatchedBy(func(reason *string) bool {
					return reason != nil && strings.Contains(*reason, tc.wantReason)
				})).Return(nil).Once()

			err := validator.validateApplication(t.Context(), app)

			require.ErrorContains(t, err, tc.wantReason)
			require.Equal(t, model.ApplicationStatus_Corrupted, app.Status)
			repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
			repo.AssertExpectations(t)
		})
	}
}

func rootValidationFixture() (*Mockrepo, *Service, *model.Application, *model.Epoch) {
	repo := newMockrepo()
	postContext := merkle.CreatePostContext()
	validator := &Service{
		repository: repo, pristinePostContext: postContext, pristineRootHash: postContext[merkle.TREE_DEPTH],
	}
	validator.Logger = slog.Default()
	app := &model.Application{
		ID: 73, Name: "root-validation", Enabled: true, ConsensusType: model.Consensus_Authority,
		Status: model.ApplicationStatus_OK, TemplateHash: common.HexToHash("0xcafe"),
	}
	epoch := &model.Epoch{
		ApplicationID: app.ID, Index: 0, VirtualIndex: 0, FirstBlock: 0, LastBlock: 9,
		Status: model.EpochStatus_InputsProcessed, MachineHash: &app.TemplateHash,
		TxBufferDataBlock: &validator.pristineRootHash,
	}
	return repo, validator, app, epoch
}

func expectRootValidationReads(
	repo *Mockrepo, app *model.Application, epoch *model.Epoch, input *model.Input, outputs []*model.Output,
) {
	appAddress := app.IApplicationAddress.String()
	// A later epoch must remain unprocessed after the first finding.
	epochs := []*model.Epoch{epoch, {ApplicationID: app.ID, Index: 1, VirtualIndex: 1}}
	repo.On("ListEpochs", mock.Anything, appAddress, mock.Anything, repository.Pagination{}, false).
		Return(epochs, uint64(len(epochs)), nil).Once()
	repo.On("ListOutputs", mock.Anything, appAddress, repository.OutputFilter{EpochIndex: &epoch.Index},
		repository.Pagination{}, false).Return(outputs, uint64(len(outputs)), nil).Once()
	repo.On("GetLastInput", mock.Anything, appAddress, epoch.Index).Return(input, nil).Once()
}
