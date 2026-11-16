// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// These SQL-valid snapshots test the repository boundary. The canonical writer
// cannot make its terminal epoch INPUTS_PROCESSED: input result and stop commit
// together. Do not treat these fixtures as a normal machine execution trace.
func setSyntheticExecutionBoundary(t *testing.T, r *PostgresRepository, appID int64, epoch uint64) {
	t.Helper()
	_, err := r.db.Exec(t.Context(), `UPDATE application SET status='MACHINE_HALTED',
 reason='synthetic repository cutoff',problem_epoch_index=$2,problem_kind='MACHINE_HALTED',problem_input_index=$2
 WHERE id=$1`, appID, epoch)
	require.NoError(t, err)
}

func TestSyntheticClaimWriterBoundary(t *testing.T) {
	for _, prefix := range []bool{false, true} {
		name := "cutoff epoch refuses result"
		if prefix {
			name = "earlier prefix stores result"
		}
		t.Run(name, func(t *testing.T) {
			r := newEpochPublicationRepository(t).(*PostgresRepository)
			app, epoch := seedEpochPublication(t, r)
			require.NoError(t, r.UpdateEpochInputsProcessed(t.Context(), app.Name, epoch.Index, repotest.DummyStateProof()))
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			outputBefore, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)
			pristine := merkle.CreatePostContext()
			proof := repository.OutputProof{Index: outputBefore.Index, Hash: crypto.Keccak256Hash(outputBefore.RawData),
				Siblings: pristine[:merkle.TREE_DEPTH]}
			calculated := merkletest.ComputeRootHashFromProof(proof.Index, proof.Hash, proof.Siblings)
			candidate := *before
			candidate.Status, candidate.CalculatedOutputsRoot = model.EpochStatus_ClaimComputed, &calculated
			candidate.Commitment = new(repotest.UniqueHash())
			candidate.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
			cutoff := epoch.Index
			if prefix {
				cutoff++
			}
			setSyntheticExecutionBoundary(t, r, app.ID, cutoff)
			called := false
			err = r.StoreClaimAndProofs(t.Context(), &candidate, func(ctx context.Context, writer repository.ClaimProofWriter) error {
				called = true
				return writer.StoreOutputProofs(ctx, []repository.OutputProof{proof})
			})
			if prefix {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, repository.ErrApplicationNotRunnable)
			}
			require.Equal(t, prefix, called)
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			outputAfter, err := r.GetOutput(t.Context(), app.Name, 0)
			require.NoError(t, err)
			if prefix {
				require.Equal(t, model.EpochStatus_ClaimComputed, after.Status)
				require.Equal(t, &calculated, after.CalculatedOutputsRoot)
				require.Equal(t, candidate.Commitment, after.Commitment)
				require.Equal(t, candidate.CommitmentProof, after.CommitmentProof)
				require.Equal(t, &proof.Hash, outputAfter.Hash)
				require.Equal(t, proof.Siblings, outputAfter.OutputHashesSiblings)
			} else {
				require.Equal(t, before, after, "a refused result writes no root, commitment or proof state")
				require.Equal(t, outputBefore, outputAfter, "the proof callback must not run at the cutoff")
			}
		})
	}
}

func TestSyntheticDeploymentDrainTerminalSuffix(t *testing.T) {
	for _, suffix := range []bool{false, true} {
		name := "earlier prefix stays protected"
		if suffix {
			name = "terminal cutoff can drain"
		}
		t.Run(name, func(t *testing.T) {
			r, app, epoch, request := deploymentDrainFixture(t, false)
			cutoff := epoch.Index
			if !suffix {
				cutoff++
			}
			setSyntheticExecutionBoundary(t, r, app.ID, cutoff)
			before, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			err = r.ForecloseDeploymentEpoch(t.Context(), request)
			if suffix {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, repository.ErrNoUpdate)
			}
			after, err := r.GetEpoch(t.Context(), app.Name, epoch.Index)
			require.NoError(t, err)
			if suffix {
				expected := *before
				expected.Status, expected.UpdatedAt = model.EpochStatus_ClaimForeclosed, after.UpdatedAt
				require.Equal(t, expected, *after, "drain changes status without inventing a calculated root")
				require.False(t, claimValidated(t, r, app.ID, epoch.Index))
			} else {
				require.Equal(t, before, after)
			}
		})
	}
}
