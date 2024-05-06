// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"fmt"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/crypto"
)

// A representation of a binary Merkle tree where all leaves are pristine nodes.
// A pristine node is an empty byte array of 32 bytes.
// It's purpose is to serve as a fixed table that [merkle.Tree] can use
// to fill incomplete levels when calculating its Merkle root
type pristineTree struct {
	// The height of the tree
	height uint
	// The nodes at each level. Since all nodes at the same level are equal,
	// only a single one needs to be stored
	nodes []Hash
}

func NewPristineTree(height uint) *pristineTree {
	leaf := Hash{}
	levels := make([]Hash, height+1)
	// the root has height 0, so we start filling the levels backwards
	levels[height] = leaf
	for index := int(height) - 1; index >= 0; index-- {
		levels[index] = crypto.Keccak256Hash(levels[index+1][:], levels[index+1][:])
	}
	return &pristineTree{height, levels}
}

// NodeAt returns the node at the specified height
func (t pristineTree) NodeAt(height uint) (Hash, error) {
	if height > t.height {
		return Hash{}, fmt.Errorf("merkle: maximum tree height is %d", t.height)
	}
	return t.nodes[height], nil
}
