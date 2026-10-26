// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestOpaqueTreeHeightZeroIsLeaf(t *testing.T) {
	hash := common.HexToHash("0x1234")
	require.Equal(t, TreeLeaf(hash), OpaqueTree(hash, 0))
	proof, err := OpaqueTree(hash, 0).ProveLast()
	require.NoError(t, err)
	require.Equal(t, hash, proof.Node)
	require.Empty(t, proof.Siblings)
	require.True(t, proof.VerifyRoot(hash))
}

func TestOpaqueTreeProofClassification(t *testing.T) {
	hash := common.HexToHash("0x1234")
	for _, index := range []int64{0, 1} {
		_, err := OpaqueTree(hash, 1).ProveLeaf(big.NewInt(index))
		require.ErrorIs(t, err, ErrOpaqueTree)
		require.NotErrorIs(t, err, ErrInvariant)
		require.ErrorContains(t, err, "proof enters subtree of height 1")
		require.ErrorContains(t, err, hash.Hex())
		_, err = (&Tree{RootHash: hash, Height: 1}).ProveLeaf(big.NewInt(index))
		require.ErrorIs(t, err, ErrInvariant)
		require.NotErrorIs(t, err, ErrOpaqueTree)
	}
	_, err := OpaqueTree(hash, 1).ProveLast()
	require.ErrorIs(t, err, ErrOpaqueTree)
	_, err = OpaqueTree(hash, 1).ProveLeaf(big.NewInt(2))
	require.ErrorIs(t, err, ErrBadInput)
}

func TestOpaqueTreeCanBeAProofSibling(t *testing.T) {
	left := OpaqueTree(common.HexToHash("0x1234"), 1)
	right := TreeLeaf(common.HexToHash("0x5678")).Iterated(1)
	tree := left.Join(right)
	proof, err := tree.ProveLast()
	require.NoError(t, err)
	require.True(t, proof.VerifyRoot(tree.RootHash))
	require.Equal(t, left.RootHash, proof.Siblings[1])
	_, err = tree.ProveLeaf(big.NewInt(0))
	require.ErrorIs(t, err, ErrOpaqueTree)
}
