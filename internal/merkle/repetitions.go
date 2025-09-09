// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Node struct {
	hash  common.Hash
	until uint64 // (count of) elements to the left
}

func nodeCmp(x Node, until uint64) int {
	return cmp.Compare(x.until, until)
}

func Append(xs []Node, hash common.Hash, reps uint64) []Node {
	if len(xs) == 0 {
		return []Node{{hash, reps}}
	}

	latest := &xs[len(xs)-1]
	if latest.hash == hash { // reuse previous Node when repeating the hash
		xs[len(xs)-1].until += reps
		return xs
	}
	return append(xs, Node{hash, latest.until + reps})
}

// Merkle with repetitions receives an ORDERED array of nodes, log2size and a
// stride to compute the root hash of subtree in the range `a` - `b`.
//
//	      .       |
//	     / \      |
//	    /   \     |
//	   /     \    |
//	  /       \   |
//	 /  /\     \  |
//	+--/--\-----+ v  log2size
//	0  1  2  3  4 <- stride
//	   a--b
func GetRootHash(nodes []Node, log2size uint64, stride uint64) (common.Hash, error) {
	zero := common.Hash{}

	aIndex := (1 << log2size) * stride
	bIndex := (1 << log2size) * (stride + 1)

	limit := nodes[len(nodes)-1].until
	if limit < bIndex {
		return zero, fmt.Errorf("Index out of bounds: %v out of %v.", bIndex, limit)
	}

	aCell, _ := slices.BinarySearchFunc(nodes, aIndex, nodeCmp)
	bCell, _ := slices.BinarySearchFunc(nodes, bIndex, nodeCmp)
	if aCell == bCell {
		return repeatedMerkleRoot(nodes[aCell].hash, log2size)
	}

	lhs, _ := GetRootHash(nodes[aCell:bCell], log2size-1, 2*stride)
	rhs, _ := GetRootHash(nodes[aCell:bCell], log2size-1, 2*stride+1)
	return crypto.Keccak256Hash(lhs[:], rhs[:]), nil
}

// level is a power of 2
func repeatedMerkleRoot(hash common.Hash, log2level uint64) (common.Hash, error) {
	for range log2level {
		hash = crypto.Keccak256Hash(hash[:], hash[:])
	}
	return hash, nil
}
