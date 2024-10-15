// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/crypto"
)

// CreateProofs creates proofs for all the leaves of a binary Merkle tree
// with the given height. It returns the root hash and the siblings matrix
// encoded as an array, in bottom-up order.
//
// If the number of leaves exceeds the capacity for the given height,
// an error is returned.
func CreateProofs(leaves []model.Hash, height uint) (model.Hash, []model.Hash, error) {
	pristineNode := model.Hash{}

	currentLevel := leaves
	leafCount := uint(len(leaves))
	siblings := make([]model.Hash, leafCount*height)

	// for each level in the tree, starting from the leaves
	for levelIdx := uint(0); levelIdx < height; levelIdx++ {
		// for each leaf
		for leafIdx := uint(0); leafIdx < leafCount; leafIdx++ {
			// calculate its sibling at the current level

			// Each pair of siblings shares the same parent. For any given parent with index i,
			// the index of its left node is 2*i, and the index of its right node is 2*i+1.
			// Since their indices differ only by the LSB, by flipping the LSB with ^1,
			// we calculate the index of one another. In order to get the index of a parent,
			// you remove the LSB with >>1. For an ancestor of height difference h,
			// you remove the h LSBs with >>h.
			siblingIdx := (leafIdx >> levelIdx) ^ 1
			siblings[leafIdx*height+levelIdx] = *at(currentLevel, siblingIdx, &pristineNode)
		}
		// calculate the next level
		currentLevel = parentLevel(currentLevel, &pristineNode)
		// update the pristine node for the next level
		pristineNode = crypto.Keccak256Hash(pristineNode[:], pristineNode[:])
	}

	// in the end, current level is the root level
	if len(currentLevel) > 1 {
		err := fmt.Errorf("too many leaves [%d] for height [%d]", leafCount, height)
		return model.Hash{}, nil, err
	}

	return *at(currentLevel, 0, &pristineNode), siblings, nil
}

// parentLevel calculates the next level in a binary Merkle tree.
//
// For each pair of nodes in the current level, it computes their parent node
// using the Keccak256 hashing algorithm. If level has an odd number of nodes,
// a pristine node will be used to complete the pair.
//
// The parent nodes are stored in the first half of the original level slice.
//
// The function returns the parent level by re-slicing the original level slice.
func parentLevel(level []model.Hash, pristineNode *model.Hash) []model.Hash {
	// for each pair of nodes in level
	for idx := 0; idx < len(level); idx += 2 {
		leftChild := level[idx][:]
		rightChild := at(level, uint(idx+1), pristineNode)[:]
		// compute the parent node in-place
		level[idx/2] = crypto.Keccak256Hash(leftChild, rightChild)
	}
	// return the parent level by re-slicing level
	return level[:(len(level)+1)/2]
}

// at returns a pointer to the item located at index or the default value.
func at(array []model.Hash, index uint, defaultValue *model.Hash) *model.Hash {
	if index < uint(len(array)) {
		return &array[index]
	} else {
		return defaultValue
	}
}
