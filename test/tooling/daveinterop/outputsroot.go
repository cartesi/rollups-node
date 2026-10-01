// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// outputsTreeHeight is the height of the global outputs Merkle tree
// (CM_ROLLUP_LOG2_MAX_OUTPUT_COUNT). The tree does not restart per epoch.
const outputsTreeHeight = 63

// outputsMerkleRoot computes the outputs root from output payloads with a
// plain keccak tree, independently of the node's merkle package. Each leaf is
// keccak256 of one output; missing leaves are zero.
func outputsMerkleRoot(outputs [][]byte) common.Hash {
	level := make([]common.Hash, len(outputs))
	for i, output := range outputs {
		level[i] = crypto.Keccak256Hash(output)
	}
	zero := common.Hash{}
	for range outputsTreeHeight {
		if len(level)%2 == 1 {
			level = append(level, zero)
		}
		next := make([]common.Hash, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			next = append(next, crypto.Keccak256Hash(level[i][:], level[i+1][:]))
		}
		level = next
		zero = crypto.Keccak256Hash(zero[:], zero[:])
	}
	if len(level) == 0 {
		return zero
	}
	return level[0]
}
