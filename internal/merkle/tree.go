// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"errors"
	"fmt"
	"math"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/crypto"
)

// MAX_TREE_HEIGHT defines the maximum allowable height for a Merkle tree.
const MAX_TREE_HEIGHT = 64

// Tree represents a complete Merkle tree structure used to generate
// the cryptographic proofs for the Cartesi Rollup Outputs.
// It can hold up to 2^64 leaves, which are keccak256 hashes of 32 bytes.
type Tree struct {
	// The height of the tree, determining the number of levels
	height uint
	// A slice of slices storing the hash values for
	// each node in the tree at each level
	nodes [][]Hash
	// A pointer to another Merkle tree consisting of pristine nodes. It is used
	// as a strategy to optimize memory usage
	pristine *pristineTree
}

// NewTree creates a new complete binary tree with the specified height.
//
// The height parameter specifies the number of levels in the tree. A height of 0
// creates a tree with only a root node, while a higher number adds more levels.
//
// The function returns a pointer to the newly created Tree or an error
// if the height exceeds MAX_TREE_HEIGHT.
func NewTree(height uint) (*Tree, error) {
	if height > MAX_TREE_HEIGHT {
		return nil, fmt.Errorf("merkle: maximum tree height is %d", MAX_TREE_HEIGHT)
	}
	tree := new(Tree)
	tree.height = height
	tree.pristine = NewPristineTree(height)
	tree.nodes = make([][]Hash, height+1) // the root is at height 0
	tree.nodes[0] = make([]Hash, 1)       // sets the pristine node as the root
	return tree, nil
}

// NewTreeFromLeaves creates a new Merkle tree with the specified height and
// initializes it with the provided leaves.
//
// The height parameter specifies the number of levels in the tree.
// The leaves parameter is a slice of Hash values to be stored as the leaf nodes
// of the tree.
//
// If the number of leaves exceeds the capacity for the specified
// height, the function returns an error.
func NewTreeFromLeaves(height uint, leaves []Hash) (*Tree, error) {
	if uint64(len(leaves)) > maxLeaves(height) {
		return nil, errors.New("merkle: too many leaves")
	}

	tree, err := NewTree(height)
	if err != nil {
		return nil, err
	}
	if len(leaves) > 0 {
		tree.nodes[height] = leaves
	}
	tree.calculateRootHash()
	return tree, nil
}

// Push adds a new leaf node to the Merkle tree.
//
// It places the new leaf in the first available position from left to right
// to maintain the tree's completeness.
//
// If the tree is already full it returns an error.
func (t *Tree) Push(leaf Hash) error {
	leafCount := len(t.nodes[t.height])
	if t.height == 0 || uint64(leafCount) == maxLeaves(t.height) {
		return errors.New("merkle: reached maximum capacity")
	}

	t.nodes[t.height] = append(t.nodes[t.height], leaf)
	t.calculateRootHash()
	return nil
}

// PushData adds a new leaf node to the Merkle tree.
//
// It creates the leaf by hashing the received data with keccak256 and then
// places it in the first available position from left to right to maintain
// the tree's completeness.
//
// If the tree is already full it returns an error.
func (t *Tree) PushData(data []byte) error {
	leaf := crypto.Keccak256Hash(data)
	return t.Push(leaf)
}

// RootHash returns the root hash of the Merkle tree.
//
// The root hash is a cryptographic hash representing the entire tree.
// It is calculated by hashing together the hashes of the nodes at each level,
// starting from the leaves.
func (t *Tree) RootHash() Hash {
	return t.nodes[0][0]
}

// calculateRootHash recalculates the root hash of the tree.
//
// This method updates the hash values at each level of the tree
// starting from the leaves up to the root. It uses the pristine tree nodes
// to complete any levels that are not yet full.
func (t *Tree) calculateRootHash() {
	// for each level, starting from the leaves
	for currentLevel := t.height; currentLevel > 0; currentLevel-- {
		nodeCount := len(t.nodes[currentLevel])
		nextLevelCount := (nodeCount + 1) / 2
		nextLevel := make([]Hash, 0, nextLevelCount)
		// for each pair of nodes, starting from the left
		for i := 0; i < nodeCount; i += 2 {
			leftNode := t.nodes[currentLevel][i]
			rightNode := t.node(currentLevel, uint(i+1))
			// add parent node to the next level
			nextLevel = append(
				nextLevel,
				crypto.Keccak256Hash(leftNode[:], rightNode[:]),
			)
		}
		t.nodes[currentLevel-1] = nextLevel
	}
}

// SiblingsOfLeaf returns the sibling nodes of a leaf in bottom-up order.
//
// The siblings are the nodes that are paired with the specified leaf
// at each level of the tree. These nodes are required to compute
// the Merkle proof for the leaf.
//
// If the index is out of bounds, the method returns an error.
func (t *Tree) SiblingsOfLeaf(index uint) ([]Hash, error) {
	if uint64(index) >= maxLeaves(t.height) {
		return nil, errors.New("merkle: index out of bounds")
	}
	siblings := make([]Hash, 0, t.height)
	for level := t.height; level > 0; level-- {
		siblingIndex := index - 1
		if index%2 == 0 {
			siblingIndex = index + 1
		}
		siblings = append(siblings, t.node(level, siblingIndex))
	}
	return siblings, nil
}

// node returns the node at the specified level and index.
//
// If the specified node does not exist in the current tree,
// it returns a default value from the pristine tree.
func (t *Tree) node(level, index uint) Hash {
	if level <= t.height && int(index) < len(t.nodes[level]) {
		return t.nodes[level][index]
	}
	return t.pristine.nodes[level]
}

// maxLeaves calculates the maximum number of leaves for a given tree height.
func maxLeaves(height uint) uint64 {
	if height >= MAX_TREE_HEIGHT {
		return math.MaxUint64
	}
	return 1 << height
}
