// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
)

func TestCommitmentProofFailureClassification(t *testing.T) {
	machineHash := common.HexToHash("0x1234")
	dbErr := errors.New("status write failed")
	for _, tc := range []struct {
		name       string
		tree       *merkle.Tree
		status     model.ApplicationStatus
		errorText  string
		cause      error
		writeError error
	}{
		{
			name: "internal tree invariant", tree: &merkle.Tree{Height: uint32(model.Log2EpochComputationHashLeafCount)},
			status: model.ApplicationStatus_Failed, errorText: "internal invariant violated", cause: merkle.ErrInvariant,
		},
		{
			name: "invariant with status write failure", tree: &merkle.Tree{Height: uint32(model.Log2EpochComputationHashLeafCount)},
			status: model.ApplicationStatus_Failed, errorText: "internal invariant violated", cause: merkle.ErrInvariant, writeError: dbErr,
		},
		{
			name: "internal child height mismatch",
			tree: &merkle.Tree{
				Height: uint32(model.Log2EpochComputationHashLeafCount),
				Subtrees: &merkle.InnerNode{
					LHS: merkle.TreeLeaf(machineHash), RHS: merkle.TreeLeaf(machineHash),
				},
			},
			status: model.ApplicationStatus_Failed, errorText: "index out of bounds", cause: merkle.ErrBadInput,
		},
		{
			name: "stored height mismatch", tree: merkle.TreeLeaf(machineHash).Iterated(model.Log2EpochComputationHashLeafCount - 1),
			status: model.ApplicationStatus_Corrupted, errorText: "commitment tree height",
		},
		{
			name: "stored final hash mismatch", tree: merkle.TreeLeaf(common.Hash{}).Iterated(model.Log2EpochComputationHashLeafCount),
			status: model.ApplicationStatus_Corrupted, errorText: "does not match machine hash",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockrepo()
			app := &model.Application{ID: 42, Name: "proof-app", Status: model.ApplicationStatus_OK}
			epoch := &model.Epoch{Index: 3, MachineHash: &machineHash}
			s := &Service{repository: repo}
			s.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			repo.On("UpdateApplicationStatus", mock.Anything, app.ID, tc.status, mock.Anything).
				Return(tc.writeError).Once()

			commitment, proof, err := s.proveCommitment(t.Context(), app, epoch, tc.tree)

			require.ErrorContains(t, err, tc.errorText)
			require.Nil(t, commitment)
			require.Nil(t, proof)
			if tc.cause != nil {
				require.ErrorIs(t, err, tc.cause)
			}
			if tc.writeError != nil {
				require.ErrorIs(t, err, tc.writeError)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
			} else {
				require.Equal(t, tc.status, app.Status)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestCommitmentProofMatchesEpoch(t *testing.T) {
	repo := newMockrepo()
	s := &Service{repository: repo}
	s.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	machineHash := common.HexToHash("0x1234")
	epoch := &model.Epoch{Index: 3, MachineHash: &machineHash}
	tree := merkle.TreeLeaf(machineHash).Iterated(model.Log2EpochComputationHashLeafCount)
	app := &model.Application{Status: model.ApplicationStatus_OK}

	commitment, proof, err := s.proveCommitment(t.Context(), app, epoch, tree)

	require.NoError(t, err)
	require.Equal(t, tree.GetRootHash(), *commitment)
	require.Equal(t, machineHash, proof.Node)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
