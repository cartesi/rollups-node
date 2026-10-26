// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkletest

import (
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const outputsDepth = 63

type outputNodeKey struct {
	start  uint64
	height uint
}

// OutputHistory is a recursive reference over every preceding output leaf.
// It uses no Merkle builder, context fold, or aligned chunk decomposition.
type OutputHistory struct {
	leaves   []common.Hash
	nodes    map[outputNodeKey]common.Hash
	pristine [outputsDepth + 1]common.Hash
	root     common.Hash
}

// NewOutputHistory copies a complete small history and computes its root recursively.
func NewOutputHistory(leaves []common.Hash) *OutputHistory {
	history := &OutputHistory{leaves: slices.Clone(leaves), nodes: make(map[outputNodeKey]common.Hash)}
	for height := 1; height <= outputsDepth; height++ {
		previous := history.pristine[height-1]
		history.pristine[height] = crypto.Keccak256Hash(previous[:], previous[:])
	}
	history.root = history.node(0, outputsDepth)
	return history
}

// Root returns the cumulative depth-63 root.
func (history *OutputHistory) Root() common.Hash { return history.root }

// Proof returns the lowest sibling first for one leaf in the copied history.
func (history *OutputHistory) Proof(index uint64) []common.Hash {
	if index >= uint64(len(history.leaves)) {
		panic("output reference index is outside the history")
	}
	siblings := make([]common.Hash, outputsDepth)
	for height := uint(0); height < outputsDepth; height++ {
		start := ((index >> height) ^ 1) << height
		siblings[height] = history.node(start, height)
	}
	return siblings
}

func (history *OutputHistory) node(start uint64, height uint) common.Hash {
	if start >= uint64(len(history.leaves)) {
		return history.pristine[height]
	}
	if height == 0 {
		return history.leaves[start]
	}
	key := outputNodeKey{start: start, height: height}
	if hash, ok := history.nodes[key]; ok {
		return hash
	}
	left := history.node(start, height-1)
	right := history.node(start+(uint64(1)<<(height-1)), height-1)
	hash := crypto.Keccak256Hash(left[:], right[:])
	history.nodes[key] = hash
	return hash
}
