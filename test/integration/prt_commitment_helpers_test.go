// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// sparseDisputeCommitment is a test-only tree whose leaves all contain the same
// machine hash, except for an optional different final leaf. It follows Dave's
// ConfigurableCommitmentFixture SAME and RIGHTMOST_DIFFERENT families. Machine
// hashes are leaves directly; only ordered pairs of child hashes are hashed.
//
// This proves commitment membership, not validity of a machine execution. It
// supports external dispute actors without adding node dispute automation.
// There is deliberately no endtoendtests tag: its pure tests must run without
// the integration TestMain, which starts a node and connects to the devnet.
type sparseDisputeCommitment struct {
	height   uint64
	repeated []common.Hash // Entry h is a repeated-state subtree of height h.
	final    []common.Hash // Entry h has the different state at its final leaf.
}

// The test helper uses uint64 leaf positions. All canonical heights (48, 17,
// and 27) fit this bound. Solidity supports larger coordinates independently.
const maxSparseDisputeHeight = 63

func newSparseDisputeCommitment(height uint64, repeatedState, finalState common.Hash) (*sparseDisputeCommitment, error) {
	if height == 0 || height > maxSparseDisputeHeight {
		return nil, fmt.Errorf("sparse dispute height %d is outside 1..%d", height, maxSparseDisputeHeight)
	}
	tree := &sparseDisputeCommitment{height: height,
		repeated: make([]common.Hash, height+1), final: make([]common.Hash, height+1)}
	tree.repeated[0], tree.final[0] = repeatedState, finalState
	for level := uint64(1); level <= height; level++ {
		left, right := tree.repeated[level-1], tree.final[level-1]
		tree.repeated[level] = crypto.Keccak256Hash(left[:], left[:])
		tree.final[level] = crypto.Keccak256Hash(left[:], right[:])
	}
	return tree, nil
}

func (tree *sparseDisputeCommitment) root() common.Hash {
	return tree.final[tree.height]
}

func (tree *sparseDisputeCommitment) leafCount() uint64 {
	return uint64(1) << tree.height
}

// rightmostChildren opens a height-h subtree on the rightmost branch. With
// repeated and final-leaf-different actors, every bisection selects this branch.
// Passing h and h-1 supplies the two adjacent openings for advanceMatch; h=1
// supplies the leaf pair for either seal method.
func (tree *sparseDisputeCommitment) rightmostChildren(height uint64) (common.Hash, common.Hash, error) {
	if height == 0 || height > tree.height {
		return common.Hash{}, common.Hash{}, fmt.Errorf("cannot open subtree height %d in height-%d commitment", height, tree.height)
	}
	return tree.repeated[height-1], tree.final[height-1], nil
}

// proof returns the leaf and its siblings from leaf level to root level, as
// required by Commitment.getRoot. The returned slice is owned by the caller and
// uses the generated Solidity binding's bytes32[] type. The final-leaf proof
// is used for joinTournament; the second-last proof supplies the agree state
// when sealing the final-leaf divergence.
func (tree *sparseDisputeCommitment) proof(position uint64) (common.Hash, [][32]byte, error) {
	if position >= tree.leafCount() {
		return common.Hash{}, nil, fmt.Errorf("leaf position %d is outside height-%d commitment", position, tree.height)
	}
	leaf := tree.repeated[0]
	if position == tree.leafCount()-1 {
		leaf = tree.final[0]
	}
	siblings := make([][32]byte, tree.height)
	for level := uint64(0); level < tree.height; level++ {
		// Subtrees are numbered from left to right at each height. Only
		// the last subtree can contain the different final leaf.
		siblingIndex := (position >> level) ^ 1
		lastSubtreeIndex := (uint64(1) << (tree.height - level)) - 1
		siblings[level] = tree.repeated[level]
		if siblingIndex == lastSubtreeIndex {
			siblings[level] = tree.final[level]
		}
	}
	return leaf, siblings, nil
}
