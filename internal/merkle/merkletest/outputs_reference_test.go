// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkletest

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestOutputHistoryProofsAndCopies(t *testing.T) {
	leaves := []common.Hash{{1}, {2}, {3}}
	history := NewOutputHistory(leaves)
	root := history.Root()
	for index, leaf := range leaves {
		proof := history.Proof(uint64(index))
		require.Len(t, proof, outputsDepth)
		got := leaf
		for height, sibling := range proof {
			if (index>>height)&1 == 0 {
				got = crypto.Keccak256Hash(got[:], sibling[:])
			} else {
				got = crypto.Keccak256Hash(sibling[:], got[:])
			}
		}
		require.Equal(t, root, got)
	}
	leaves[0] = common.Hash{99}
	require.Equal(t, root, history.Root())
	require.Panics(t, func() { history.Proof(uint64(len(leaves))) })
	empty := NewOutputHistory(nil)
	var pristine common.Hash
	for range outputsDepth {
		pristine = crypto.Keccak256Hash(pristine[:], pristine[:])
	}
	require.Equal(t, pristine, empty.Root())
}
