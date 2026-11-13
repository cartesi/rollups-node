// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/test/tooling/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// These fixtures test persistence and eligibility rather than guest execution.
func setMatchingCalculatedRoot(t *testing.T, repo repository.Repository, epoch *model.Epoch) {
	t.Helper()
	var declared []byte
	r := repo.(*PostgresRepository)
	require.NoError(t, r.db.QueryRow(t.Context(),
		`SELECT tx_buffer_data_block FROM epoch WHERE application_id=$1 AND index=$2`, epoch.ApplicationID, epoch.Index).
		Scan(&declared))
	require.Len(t, declared, common.HashLength)
	root := common.BytesToHash(declared)
	epoch.CalculatedOutputsRoot = &root
}

func TestCalculatedOutputsRootRollsBackWithProofs(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch := seedEpochPublication(t, r)
	stateProof := repotest.DummyStateProof()
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, stateProof))
	output, err := r.GetOutput(t.Context(), app.Name, 0)
	require.NoError(t, err)
	pristine := merkle.CreatePostContext()
	proof := repository.OutputProof{Index: output.Index, Hash: crypto.Keccak256Hash(output.RawData),
		Siblings: pristine[:merkle.TREE_DEPTH]}
	calculated := merkletest.ComputeRootHashFromProof(proof.Index, proof.Hash, proof.Siblings)
	require.NotEqual(t, stateProof.TxBufferDataBlock, calculated)
	epoch.CalculatedOutputsRoot = &calculated
	epoch.Commitment = new(repotest.UniqueHash())
	epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
	failure := errors.New("proof callback failed after its first write")
	written := false
	err = r.StoreClaimAndProofs(t.Context(), epoch, func(ctx context.Context, writer repository.ClaimProofWriter) error {
		if writeErr := writer.StoreOutputProofs(ctx, []repository.OutputProof{proof}); writeErr != nil {
			return writeErr
		}
		written = true
		return failure
	})
	require.ErrorIs(t, err, failure)
	require.True(t, written, "the failure must occur after a real proof write")

	endpoint, err := db.GetTestDatabaseEndpoint()
	require.NoError(t, err)
	reconnected, err := NewPostgresRepository(t.Context(), endpoint, 1, 0)
	require.NoError(t, err)
	defer reconnected.Close()
	stored, err := reconnected.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_InputsProcessed, stored.Status)
	require.Nil(t, stored.CalculatedOutputsRoot)
	require.Nil(t, stored.Commitment)
	require.Empty(t, stored.CommitmentProof)
	require.False(t, claimValidated(t, reconnected.(*PostgresRepository), app.ID, epoch.Index))
	storedOutput, err := reconnected.GetOutput(t.Context(), app.Name, output.Index)
	require.NoError(t, err)
	require.Equal(t, output, storedOutput, "the failed batch must not change output proof or timestamps")

	require.NoError(t, reconnected.StoreClaimAndProofs(t.Context(), epoch,
		func(ctx context.Context, writer repository.ClaimProofWriter) error {
			return writer.StoreOutputProofs(ctx, []repository.OutputProof{proof})
		}))
	stored, err = reconnected.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, model.EpochStatus_ClaimComputed, stored.Status)
	require.Equal(t, &calculated, stored.CalculatedOutputsRoot)
	require.Equal(t, &stateProof.TxBufferDataBlock, stored.TxBufferDataBlock)
	require.Equal(t, epoch.Commitment, stored.Commitment)
	require.Equal(t, epoch.CommitmentProof, stored.CommitmentProof)
	require.True(t, claimValidated(t, reconnected.(*PostgresRepository), app.ID, epoch.Index))
	storedOutput, err = reconnected.GetOutput(t.Context(), app.Name, output.Index)
	require.NoError(t, err)
	require.Equal(t, &proof.Hash, storedOutput.Hash)
	require.Equal(t, proof.Siblings, storedOutput.OutputHashesSiblings)
	storedApp, err := reconnected.GetApplication(t.Context(), app.Name)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_OK, storedApp.Status)
	require.Nil(t, storedApp.Reason)
	state, err := reconnected.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Nil(t, state.Problem)
}

func TestCalculatedOutputsRootRequiredForLocalComputation(t *testing.T) {
	for _, method := range []string{"claim writer", "status update"} {
		t.Run(method, func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch, proof := machinePublicationEpoch(t, r, false)
			require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
			epoch.CalculatedOutputsRoot = nil
			called := false
			var err error
			if method == "claim writer" {
				err = r.StoreClaimAndProofs(t.Context(), epoch, func(context.Context, repository.ClaimProofWriter) error {
					called = true
					return nil
				})
			} else {
				_, err = r.db.Exec(t.Context(), `UPDATE epoch SET status='CLAIM_COMPUTED' WHERE application_id=$1 AND index=$2`,
					app.ID, epoch.Index)
			}
			require.ErrorContains(t, err, "calculated_outputs_root")
			require.False(t, called)
			stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, model.EpochStatus_InputsProcessed, stored.Status)
			require.Nil(t, stored.CalculatedOutputsRoot)
			require.False(t, claimValidated(t, r, app.ID, epoch.Index))
		})
	}
}

func TestCalculatedOutputsRootRequiresValidatedEpoch(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Open, model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).Create(t.Context(), t, r)
			initial := status
			if status == model.EpochStatus_InputsProcessed {
				initial = model.EpochStatus_Closed
			}
			epoch := repotest.NewEpochBuilder(app.ID).WithStatus(initial).WithBlocks(0, 0).WithInputBounds(0, 0).Build()
			require.NoError(t, r.CreateEpochsAndInputs(t.Context(), app.Name, map[*model.Epoch][]*model.Input{epoch: nil}, 0))
			if status == model.EpochStatus_InputsProcessed {
				require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
			}
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			root := repotest.UniqueHash()
			_, err = r.db.Exec(t.Context(), `UPDATE epoch SET calculated_outputs_root=$3 WHERE application_id=$1 AND index=$2`,
				app.ID, epoch.Index, root.Bytes())
			require.ErrorContains(t, err, "epoch_calculated_outputs_root_check")
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, before, after, "root evidence cannot appear before local validation")
			require.False(t, claimValidated(t, r, app.ID, epoch.Index))
		})
	}
}

func TestCalculatedOutputsRootValidationFlagCannotClear(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app, epoch, proof := machinePublicationEpoch(t, r, false)
	epoch.CalculatedOutputsRoot = new(common.Hash{})
	require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
	require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
	before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.True(t, claimValidated(t, r, app.ID, epoch.Index), "zero is a present computed root")
	// This direct flag update does not fire the status/root UPDATE OF trigger.
	_, err = r.db.Exec(t.Context(), `UPDATE epoch SET claim_validated=false WHERE application_id=$1 AND index=$2`,
		app.ID, epoch.Index)
	require.ErrorContains(t, err, "epoch_calculated_outputs_root_check")
	after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.True(t, claimValidated(t, r, app.ID, epoch.Index))
}

func TestCalculatedOutputsRootLoadedWithoutReplacingDeclaredRoot(t *testing.T) {
	for _, calculated := range []common.Hash{{}, common.HexToHash("0xcafe")} {
		t.Run(calculated.Hex(), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch, proof := machinePublicationEpoch(t, r, false)
			require.NotEqual(t, proof.TxBufferDataBlock, calculated)
			epoch.CalculatedOutputsRoot = &calculated
			require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
			require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
			check := func(t *testing.T, got *model.Epoch, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, &calculated, got.CalculatedOutputsRoot)
				require.Equal(t, &proof.TxBufferDataBlock, got.TxBufferDataBlock)
			}
			got, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			check(t, got, err)
			virtualIndex := got.VirtualIndex
			got, err = r.GetLastNonOpenEpoch(t.Context(), app.Name)
			check(t, got, err)
			got, err = r.GetEpochByVirtualIndex(t.Context(), app.Name, virtualIndex)
			check(t, got, err)
			epochs, total, err := r.ListEpochs(t.Context(), app.Name, repository.EpochFilter{}, repository.Pagination{}, false)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, epochs, 1)
			check(t, epochs[0], nil)
			page, err := r.SelectClaimsToSubmitPage(t.Context(), repository.ClaimCursor{}, 2)
			require.NoError(t, err)
			require.Len(t, page.Work, 1)
			check(t, page.Work[0].Epoch, nil)
			state, err := r.GetEpochCompletionState(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			check(t, state.Epoch, nil)
			roots, err := r.ReadClaimRoots(t.Context(), app.ID, epoch.LastBlock)
			require.NoError(t, err)
			require.True(t, roots.Ready)
			require.Equal(t, proof.MachineHash, roots.MachineHash)
			require.Equal(t, proof.TxBufferDataBlock, roots.OutputsRoot, "chain comparison uses the declared root")
		})
	}
}

func TestCalculatedOutputsRootImmutableAcrossLifecycle(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_ClaimComputed, model.EpochStatus_ClaimStaged,
		model.EpochStatus_ClaimAccepted, model.EpochStatus_ClaimForeclosed} {
		t.Run(status.String(), func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch, proof := machinePublicationEpoch(t, r, false)
			calculated := common.HexToHash("0xcafe")
			epoch.CalculatedOutputsRoot = &calculated
			require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
			require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
			if status == model.EpochStatus_ClaimStaged || status == model.EpochStatus_ClaimAccepted {
				require.NoError(t, r.UpdateEpochReconciledStaged(t.Context(), app.ID, epoch.Index, 60))
			}
			if status == model.EpochStatus_ClaimAccepted {
				require.NoError(t, r.UpdateEpochWithAcceptedClaim(t.Context(), app.ID, epoch.Index, new(repotest.UniqueHash())))
			}
			if status == model.EpochStatus_ClaimForeclosed {
				require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 49, repotest.UniqueHash(), 80))
				require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index))
			}
			for _, query := range []string{
				`UPDATE epoch SET calculated_outputs_root=$3 WHERE application_id=$1 AND index=$2`,
				`UPDATE epoch SET calculated_outputs_root=$3,status=status WHERE application_id=$1 AND index=$2`,
			} {
				for _, replacement := range [][]byte{nil, common.HexToHash("0xbeef").Bytes()} {
					_, err := r.db.Exec(t.Context(), query, app.ID, epoch.Index, replacement)
					require.ErrorContains(t, err, "calculated outputs root cannot change")
				}
			}
			stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			require.Equal(t, status, stored.Status)
			require.Equal(t, &calculated, stored.CalculatedOutputsRoot)
			if status == model.EpochStatus_ClaimComputed {
				_, err := r.db.Exec(t.Context(), `UPDATE epoch SET calculated_outputs_root=NULL,status='CLAIM_ACCEPTED'
					WHERE application_id=$1 AND index=$2`, app.ID, epoch.Index)
				require.ErrorContains(t, err, "calculated outputs root cannot change")
			}
		})
	}
}

func TestCalculatedOutputsRootSurvivesBothForeclosureWriters(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		writer := "individual"
		if bulk {
			writer = "all epochs"
		}
		for _, computed := range []bool{false, true} {
			name := writer + "/uncomputed"
			if computed {
				name = writer + "/computed"
			}
			t.Run(name, func(t *testing.T) {
				r := newEpochPublicationRepository(t).(*PostgresRepository)
				app, epoch, proof := machinePublicationEpoch(t, r, false)
				require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, proof))
				var expected *common.Hash
				if computed {
					expected = new(common.HexToHash("0xcafe"))
					epoch.CalculatedOutputsRoot = expected
					require.NoError(t, r.StoreClaimAndProofs(t.Context(), epoch, nil))
				}
				require.NoError(t, r.UpdateApplicationForeclosure(t.Context(), app.ID, 49, repotest.UniqueHash(), 80))
				if !bulk {
					require.NoError(t, r.UpdateEpochWithForeclosedClaim(t.Context(), app.ID, epoch.Index))
				} else {
					count, err := r.ForecloseUnacceptedEpochsAtOrAfterBlock(t.Context(), app.ID, 49)
					require.NoError(t, err)
					require.EqualValues(t, 1, count)
				}
				stored, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
				require.NoError(t, err)
				require.Equal(t, model.EpochStatus_ClaimForeclosed, stored.Status)
				require.Equal(t, expected, stored.CalculatedOutputsRoot)
				require.Equal(t, computed, claimValidated(t, r, app.ID, epoch.Index))
			})
		}
	}
}
