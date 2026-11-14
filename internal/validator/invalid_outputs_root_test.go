// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestValidateInvalidOutputsRoot(t *testing.T) {
	for _, consensus := range model.ConsensusAllValues {
		t.Run(consensus.String(), func(t *testing.T) {
			for _, withOutputs := range []bool{false, true} {
				name := "no outputs"
				if withOutputs {
					name = "one output"
				}
				t.Run(name, func(t *testing.T) {
					repo, validator, app, epoch := rootValidationFixture()
					app.ConsensusType = consensus
					epoch.InputIndexUpperBound = 1
					declared := common.HexToHash("0xbad")
					epoch.TxBufferDataBlock = &declared
					input := &model.Input{Index: 0, Status: model.InputCompletionStatus_Accepted,
						MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}
					var outputs []*model.Output
					calculated := validator.pristineRootHash
					if withOutputs {
						outputs = []*model.Output{{Index: 0, RawData: []byte("stored bytes")}}
						calculated = merkletest.ComputeRootHashFromProof(0, crypto.Keccak256Hash(outputs[0].RawData),
							validator.pristinePostContext[:merkle.TREE_DEPTH])
					}
					expectRootValidationReads(repo, app, epoch, input, outputs)
					var expectedCommitment common.Hash
					var expectedProof *merkle.Proof
					if consensus == model.Consensus_PRT {
						first := common.HexToHash("0x123")
						rows := []*model.StateHash{{MachineHash: first, Repetitions: 1},
							{MachineHash: model.FinalStateEntry(*epoch.MachineHash), Repetitions: model.InputHashCollectionCapacity - 1}}
						expectStateHashRows(repo, app, epoch, rows, uint64(len(rows)))
						expectedCommitment, expectedProof = invalidRootTraceOracle(first, *epoch.MachineHash)
					}
					writer := &rootProofWriter{outputs: outputs}
					repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once().
						Run(func(args mock.Arguments) {
							require.Equal(t, &calculated, epoch.CalculatedOutputsRoot)
							write, ok := args.Get(2).(repository.ClaimProofWriteFunc)
							if withOutputs {
								require.True(t, ok)
								require.NoError(t, write(t.Context(), writer))
							} else {
								require.Nil(t, args.Get(2))
							}
						})
					err := validator.validateApplication(t.Context(), app)
					require.NoError(t, err)
					require.Equal(t, model.ApplicationStatus_OK, app.Status)
					require.Nil(t, repo.problem)
					require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
					require.Equal(t, &declared, epoch.TxBufferDataBlock)
					require.Equal(t, &calculated, epoch.CalculatedOutputsRoot)
					require.True(t, epoch.HasInvalidOutputsRoot())
					require.Equal(t, model.InputCompletionStatus_Accepted, input.Status)
					if consensus == model.Consensus_PRT {
						require.Equal(t, &expectedCommitment, epoch.Commitment)
						require.Equal(t, expectedProof.Siblings, epoch.CommitmentProof)
						require.True(t, expectedProof.VerifyRoot(*epoch.Commitment))
					} else {
						require.Nil(t, epoch.Commitment)
						repo.AssertNotCalled(t, "CountEpochStateHashes", mock.Anything, mock.Anything, mock.Anything)
					}
					if withOutputs {
						require.Len(t, writer.proofs, 1)
						require.Equal(t, crypto.Keccak256Hash(outputs[0].RawData), writer.proofs[0].Hash)
						require.Equal(t, validator.pristinePostContext[:merkle.TREE_DEPTH], writer.proofs[0].Siblings)
					}
					repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
					repo.AssertExpectations(t)
				})
			}
		})
	}
}

// The reference run-tree oracle is independent of the streaming commitment builder.
func invalidRootTraceOracle(first, final common.Hash) (common.Hash, *merkle.Proof) {
	runs := []merkletest.Run{{Hash: first, Count: 1},
		{Hash: model.FinalStateEntry(final), Count: model.InputHashCollectionCapacity - 1},
		{Hash: model.FinalStateEntry(final),
			Count: (model.MaxAdvanceStatesPerEpoch - 1) << model.Log2InputHashCollectionCapacity}}
	const entryHeight = uint32(48)
	root := merkletest.Root(runs, entryHeight)
	_, upper := merkletest.LastProof(runs, entryHeight)
	siblings := make([]common.Hash, 0, model.Log2EpochComputationHashLeafCount)
	hash := final
	for range 6 {
		siblings = append(siblings, hash)
		hash = crypto.Keccak256Hash(hash[:], hash[:])
	}
	siblings = append(siblings, upper...)
	return root, &merkle.Proof{Pos: new(big.Int).SetUint64((uint64(1) << model.Log2EpochComputationHashLeafCount) - 1),
		Node: final, Siblings: siblings}
}

type rootProofWriter struct {
	outputs []*model.Output
	proofs  []repository.OutputProof
}

func (w *rootProofWriter) ScanOutputs(_ context.Context, first, last uint64, fn repository.OutputScanFunc) error {
	for _, output := range w.outputs {
		if output.Index >= first && output.Index <= last {
			if err := fn(output.Index, output.RawData); err != nil {
				return err
			}
		}
	}
	return nil
}
func (w *rootProofWriter) StoreOutputProofs(_ context.Context, proofs []repository.OutputProof) error {
	w.proofs = append(w.proofs, proofs...)
	return nil
}

func TestValidateEmptyFirstEpochInvalidOutputsRoot(t *testing.T) {
	repo, validator, app, epoch := rootValidationFixture()
	app.ConsensusType = model.Consensus_PRT
	zero := common.Hash{}
	epoch.TxBufferDataBlock = &zero
	expectRootValidationReads(repo, app, epoch, nil, nil)
	repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()
	require.NoError(t, validator.validateApplication(t.Context(), app))
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
	require.Equal(t, &validator.pristineRootHash, epoch.CalculatedOutputsRoot)
	require.True(t, epoch.HasInvalidOutputsRoot())
	expected := merkle.TreeLeaf(*epoch.MachineHash).Iterated(model.Log2EpochComputationHashLeafCount)
	require.Equal(t, &expected.RootHash, epoch.Commitment)
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
			expectNoStoredProtectionRoots(repo, app)
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
	repo.application = app
	return repo, validator, app, epoch
}

func expectRootValidationReads(
	repo *Mockrepo, app *model.Application, epoch *model.Epoch, input *model.Input, outputs []*model.Output,
) {
	appAddress := app.IApplicationAddress.String()
	epochs := []*model.Epoch{epoch}
	repo.On("ListEpochs", mock.Anything, appAddress, mock.Anything, repository.Pagination{}, false).
		Return(epochs, uint64(len(epochs)), nil).Once()
	expectEpochOutputs(repo, app, epoch, outputs)
	repo.On("GetLastInput", mock.Anything, appAddress, epoch.Index).Return(input, nil).Once()
}

func TestValidateNoOutputSuccessorCarriesCalculatedRoot(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_PRT, model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			repo, validator, app, epoch := rootValidationFixture()
			app.ConsensusType = consensus
			epoch.Index, epoch.VirtualIndex = 1, 1
			epoch.InputIndexLowerBound = 1
			bad := common.HexToHash("0xbad")
			previous := &model.Epoch{ApplicationID: app.ID, Index: 0, VirtualIndex: 0,
				MachineHash: epoch.MachineHash, TxBufferDataBlock: &bad, CalculatedOutputsRoot: &validator.pristineRootHash}
			require.True(t, previous.HasInvalidOutputsRoot())
			reads := 1
			var input *model.Input
			if consensus == model.Consensus_PRT {
				epoch.InputIndexUpperBound = epoch.InputIndexLowerBound
				epoch.TxBufferDataBlock = &bad
				reads = 2
			} else {
				epoch.InputIndexUpperBound = epoch.InputIndexLowerBound + 1
				epoch.TxBufferDataBlock = &validator.pristineRootHash
				input = &model.Input{Index: 1, Status: model.InputCompletionStatus_Accepted,
					MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}
			}
			expectRootValidationReads(repo, app, epoch, input, nil)
			repo.On("GetEpochByVirtualIndex", mock.Anything, app.IApplicationAddress.Hex(), uint64(0)).
				Return(previous, nil).Times(reads)
			repo.On("StoreClaimAndProofs", mock.Anything, epoch, mock.Anything).Return(nil).Once()
			require.NoError(t, validator.validateApplication(t.Context(), app))
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
			require.Equal(t, &validator.pristineRootHash, epoch.CalculatedOutputsRoot)
			require.Equal(t, consensus == model.Consensus_PRT, epoch.HasInvalidOutputsRoot(),
				"empty Dave inherits the bad declaration; a nonempty A/Q epoch can correct it")
			require.True(t, previous.HasInvalidOutputsRoot())
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertExpectations(t)
		})
	}
}

func TestValidateMissingPreviousCalculatedRootRetriesWithoutVerdict(t *testing.T) {
	repo, validator, app, epoch := rootValidationFixture()
	epoch.Index, epoch.VirtualIndex = 1, 1
	previous := &model.Epoch{ApplicationID: app.ID, Index: 0, VirtualIndex: 0,
		MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}
	repo.On("ListEpochs", mock.Anything, app.IApplicationAddress.Hex(), mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{epoch}, uint64(1), nil).Once()
	expectEpochOutputs(repo, app, epoch, nil)
	repo.On("GetEpochByVirtualIndex", mock.Anything, app.IApplicationAddress.Hex(), uint64(0)).Return(previous, nil).Once()
	err := validator.validateApplication(t.Context(), app)
	require.ErrorContains(t, err, "has no calculated outputs root")
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	require.Nil(t, repo.problem)
	require.Equal(t, model.EpochStatus_InputsProcessed, epoch.Status)
	require.Nil(t, epoch.CalculatedOutputsRoot)
	require.Contains(t, validator.claimRetries, claimRetryKey{applicationID: app.ID, epochIndex: epoch.Index})
	repo.AssertNotCalled(t, "StoreClaimAndProofs", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}
