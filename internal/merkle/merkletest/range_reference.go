// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkletest

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ComputeRootHashFromProof independently opens an output leaf at its global index.
func ComputeRootHashFromProof(index uint64, hash common.Hash, siblings []common.Hash) common.Hash {
	for level, sibling := range siblings {
		if index>>level&1 == 0 {
			hash = crypto.Keccak256Hash(hash[:], sibling[:])
		} else {
			hash = crypto.Keccak256Hash(sibling[:], hash[:])
		}
	}
	return hash
}

// ComputeSiblingsMatrix recursively opens a small range with arbitrary context roots.
// Contexts avoid allocating the preceding history, including indexes near 2^63.
func ComputeSiblingsMatrix(pre, leaves, post []common.Hash, begin uint64) ([]common.Hash, error) {
	if len(leaves) == 0 || len(pre) < outputsDepth || len(post) < outputsDepth {
		return nil, fmt.Errorf("missing range leaves or contexts")
	}
	_, matrix := rangeReference(pre, leaves, post, begin, outputsDepth)
	return matrix, nil
}

// CreateProofs supplies the small dense-output oracle used by integration tests.
func CreateProofs(leaves []common.Hash, height uint) (common.Hash, []common.Hash, error) {
	if height > outputsDepth || uint64(len(leaves)) > uint64(1)<<height {
		return common.Hash{}, nil, fmt.Errorf("too many leaves or invalid height")
	}
	post := make([]common.Hash, height+1)
	for i := uint(1); i <= height; i++ {
		post[i] = crypto.Keccak256Hash(post[i-1][:], post[i-1][:])
	}
	root, matrix := rangeReference(post, leaves, post, 0, height)
	return root, matrix, nil
}

func rangeReference(pre, leaves, post []common.Hash, begin uint64, height uint) (common.Hash, []common.Hash) {
	end := begin + uint64(len(leaves))
	nodes := make(map[outputNodeKey]common.Hash)
	var node func(uint64, uint) common.Hash
	node = func(start uint64, h uint) common.Hash {
		if start >= end {
			return post[h]
		}
		if start+(uint64(1)<<h) <= begin {
			return pre[h]
		}
		if h == 0 {
			return leaves[start-begin]
		}
		key := outputNodeKey{start: start, height: h}
		if hash, ok := nodes[key]; ok {
			return hash
		}
		left, right := node(start, h-1), node(start+(uint64(1)<<(h-1)), h-1)
		hash := crypto.Keccak256Hash(left[:], right[:])
		nodes[key] = hash
		return hash
	}
	root := node(0, height)
	matrix := make([]common.Hash, len(leaves)*int(height))
	for i := range leaves {
		index := begin + uint64(i)
		for level := uint(0); level < height; level++ {
			matrix[i*int(height)+int(level)] = node(((index>>level)^1)<<level, level)
		}
	}
	return root, matrix
}
