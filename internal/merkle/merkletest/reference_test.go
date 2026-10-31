// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkletest

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestRecursiveReferenceMatchesDenseLeaves(t *testing.T) {
	a := common.HexToHash("0x1234")
	b := common.HexToHash("0x5678")
	runs := []Run{{a, 3}, {b, 4}, {a, 1}}
	leaves := []common.Hash{a, a, a, b, b, b, b, a}
	node := leaves[len(leaves)-1]
	var siblings []common.Hash
	for len(leaves) > 1 {
		siblings = append(siblings, leaves[len(leaves)-2])
		next := make([]common.Hash, len(leaves)/2)
		for i := range next {
			next[i] = crypto.Keccak256Hash(leaves[2*i][:], leaves[2*i+1][:])
		}
		leaves = next
	}
	require.Equal(t, leaves[0], Root(runs, 3))
	actualNode, actualSiblings := LastProof(runs, 3)
	require.Equal(t, node, actualNode)
	require.Equal(t, siblings, actualSiblings)
	left, right := RootChildren(runs, 3)
	require.Equal(t, leaves[0], crypto.Keccak256Hash(left[:], right[:]))
}

func TestRecursiveReferenceUniformLargeRun(t *testing.T) {
	hash := common.HexToHash("0xabcd")
	root := hash
	for range 54 {
		root = crypto.Keccak256Hash(root[:], root[:])
	}
	require.Equal(t, root, Root([]Run{{hash, 1 << 54}}, 54))
}
