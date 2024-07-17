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

	currentLevel := leaves
	nextLevel := make([]model.Hash, (len(leaves)+1)/2)
	siblings := make([][]model.Hash, len(leaves))

	// for each level in the tree, starting from the leaves
	var levelIdx uint
	for levelIdx = 0; levelIdx < height; levelIdx++ {
		calculateSiblings(levelIdx, height, currentLevel, siblings, &pristineNode)
		calculateParents(currentLevel, nextLevel, &pristineNode)

		aux := currentLevel
		currentLevel = nextLevel
		// re-slices current level to avoid allocating multiple arrays
		nextLevel = aux[:(len(currentLevel)+1)/2]
		pristineNode = crypto.Keccak256Hash(pristineNode[:], pristineNode[:])
	}

	// in the end, current level is the root level
	if len(currentLevel) > 1 {
		return model.Hash{}, nil, errors.New("too many leaves for height")
	}

	return *at(currentLevel, 0, &pristineNode), siblings, nil
}

// calculateSiblings iterates over each leaf and populates the siblings matrix
// with the appropriate sibling nodes at the current level.
//
// If the current level has an odd number of nodes a pristine node will be used.
func calculateSiblings(
	levelIdx, height uint,
	currentLevel []model.Hash,
	siblings [][]model.Hash,
	pristineNode *model.Hash,
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
		if levelIdx == 0 {
			// in the leaf level, use the leaf index directly
			siblingIdx = idx ^ 1
		}

		siblings[idx][levelIdx] = *at(currentLevel, uint(siblingIdx), pristineNode)
	}
}

// calculateParents computes the parent nodes for the next level in the Merkle
// tree by hashing their children with the Keccak-256 hash function.
//
// If the current level has an odd number of nodes a pristine node will be used.
func calculateParents(currentLevel, nextLevel []model.Hash, pristineNode *model.Hash) {
	// for each parent node
	for idx := range nextLevel {
		leftChild := &currentLevel[2*idx]
		rightChild := at(currentLevel, uint(2*idx+1), pristineNode)
		nextLevel[idx] = crypto.Keccak256Hash(leftChild[:], rightChild[:])
	}
}

// at returns the item at index in the array or the provided default value.
func at(array []model.Hash, index uint, defaultValue *model.Hash) *model.Hash {
	if index < uint(len(array)) {
		return &array[index]
	} else {
		return defaultValue
	}
}
