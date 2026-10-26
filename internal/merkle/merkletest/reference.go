// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package merkletest supplies a recursive Merkle reference over protocol-leaf runs.
// It does not use the Merkle builders or their aligned-block decomposition.
package merkletest

import (
	"math"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Run repeats Hash at the protocol-leaf level Count times.
type Run struct {
	Hash  common.Hash
	Count uint64
}

// Root returns the root of exactly 2^height protocol leaves.
// It panics for an invalid run count or height outside [0, 63].
func Root(runs []Run, height uint32) common.Hash {
	ends := endpoints(runs, height)
	return root(runs, ends, 0, height)
}

// LastProof returns the final protocol leaf and its siblings, lowest level first.
// It uses the same input requirements as Root.
func LastProof(runs []Run, height uint32) (common.Hash, []common.Hash) {
	ends := endpoints(runs, height)
	total := uint64(1) << height
	var siblings []common.Hash
	if height > 0 {
		siblings = make([]common.Hash, height)
	}
	for level := range height {
		siblings[level] = root(runs, ends, total-(uint64(1)<<(level+1)), level)
	}
	return runs[len(runs)-1].Hash, siblings
}

// RootChildren returns both halves of a non-leaf tree with Root's input requirements.
func RootChildren(runs []Run, height uint32) (common.Hash, common.Hash) {
	ends := endpoints(runs, height)
	if height == 0 {
		panic("root children require positive height")
	}
	return root(runs, ends, 0, height-1), root(runs, ends, uint64(1)<<(height-1), height-1)
}

func endpoints(runs []Run, height uint32) []uint64 {
	const maxHeight = 63
	if height > maxHeight {
		panic("reference height must be below 64")
	}
	ends := make([]uint64, len(runs))
	var count uint64
	for i, run := range runs {
		if run.Count == 0 || run.Count > math.MaxUint64-count {
			panic("invalid reference run count")
		}
		count += run.Count
		ends[i] = count
	}
	if count != uint64(1)<<height {
		panic("reference run total does not match height")
	}
	return ends
}

func root(runs []Run, ends []uint64, start uint64, height uint32) common.Hash {
	index := sort.Search(len(ends), func(i int) bool { return ends[i] > start })
	if (uint64(1) << height) <= ends[index]-start {
		hash := runs[index].Hash
		for range height {
			hash = crypto.Keccak256Hash(hash[:], hash[:])
		}
		return hash
	}
	left := root(runs, ends, start, height-1)
	right := root(runs, ends, start+(uint64(1)<<(height-1)), height-1)
	return crypto.Keccak256Hash(left[:], right[:])
}
