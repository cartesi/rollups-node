// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"errors"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/crypto"
)

// CreateProofs creates proofs for all the leaves of the binary Merkle tree
// with the given height. It returns the root hash and the siblings matrix,
// in bottom-up order.
//
// If the number of leaves exceeds the capacity for the given height,
// an error is returned.
func CreateProofs(leaves []model.Hash, height uint) (model.Hash, [][]model.Hash, error) {
	pristineNode := model.Hash{}

	if len(leaves) == 0 {
		leaves = append(leaves, pristineNode)
	}

	currentLevel := leaves
	nextLevel := make([]model.Hash, (len(leaves)+1)/2)
	siblings := make([][]model.Hash, len(leaves))

	// for each level in the tree, starting from the leaves
	for levelIdx := 0; levelIdx < int(height); levelIdx++ {
		calculateSiblings(levelIdx, int(height), currentLevel, siblings, pristineNode)
		calculateParents(currentLevel, nextLevel, pristineNode)

		aux := currentLevel
		currentLevel = nextLevel
		// re-slices current level to avoid allocating multiple arrays
		nextLevel = aux[:(len(currentLevel)+1)/2]
		pristineNode = crypto.Keccak256Hash(pristineNode[:], pristineNode[:])
	}

	// in the end, current level is the root level
	if len(currentLevel) != 1 {
		return model.Hash{}, nil, errors.New("too many leaves for height")
	}

	return currentLevel[0], siblings, nil
}

// calculateSiblings iterates over each leaf and populates the siblings matrix
// with the appropriate sibling nodes at the current level.
//
// If the level is not full, a pristine node is used instead.
func calculateSiblings(
	level, height int,
	currentLevel []model.Hash,
	siblings [][]model.Hash,
	pristineNode model.Hash,
) {
	// for each leaf
	for idx := range siblings {
		// creates the sibling slice if needed
		if siblings[idx] == nil {
			siblings[idx] = make([]model.Hash, height)
		}

		// the sibling index is to the left of the parent if leaf index is odd
		// or to the right if leaf index is even
		siblingIdx := (idx / 2) ^ 1
		if level == 0 {
			// in the leaf level, use the leaf index directly
			siblingIdx = idx ^ 1
		}

		sibling := pristineNode
		// if current level is full
		if siblingIdx < len(currentLevel) {
			sibling = currentLevel[siblingIdx]
		}
		siblings[idx][level] = sibling
	}
}

// calculateParents computes the parent nodes for the next level in the Merkle
// tree by hashing their children with the Keccak-256 hash function.
//
// If the level is not full, a pristine node is used instead.
func calculateParents(currentLevel, nextLevel []model.Hash, pristineNode model.Hash) {
	// for each parent node
	for idx := range nextLevel {
		leftChild := currentLevel[2*idx]
		rightChild := pristineNode
		// if current level is full
		if 2*idx+1 < len(currentLevel) {
			rightChild = currentLevel[2*idx+1]
		}

		nextLevel[idx] = crypto.Keccak256Hash(leftChild[:], rightChild[:])
	}
}
