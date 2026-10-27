// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	TREE_DEPTH = 63
)

func dupArray(in []common.Hash) []common.Hash {
	out := make([]common.Hash, len(in))
	copy(out, in)
	return out
}

// CreatePreContextFromProof creates the pre context for cumulative output proofs.
func CreatePreContextFromProof(
	index uint64,
	hash common.Hash,
	siblings []common.Hash,
) []common.Hash {
	preContext := dupArray(siblings) // preserve argument

	level := 0
	for ; level < TREE_DEPTH; level++ {
		from := index >> level & 1
		to := (index + 1) >> level & 1
		if from == 0 && to == 1 {
			break
		}
		hash = crypto.Keccak256Hash(siblings[level][:], hash[:])
	}
	preContext[level] = hash
	return preContext
}

// CreatePostContext creates the post context for cumulative output proofs.
func CreatePostContext() []common.Hash {
	pristine := make([]common.Hash, TREE_DEPTH+1)
	for i := 1; i <= TREE_DEPTH; i++ {
		pristine[i] = crypto.Keccak256Hash(pristine[i-1][:], pristine[i-1][:])
	}
	return pristine
}
