// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestSparseDisputeCommitmentMatchesDenseTrees(t *testing.T) {
	repeated := common.HexToHash("0x1234")
	for height := uint64(1); height <= 8; height++ {
		for _, final := range []common.Hash{repeated, common.HexToHash("0xabcd")} {
			t.Run(strconv.FormatUint(height, 10)+"/"+final.Hex(), func(t *testing.T) {
				tree, err := newSparseDisputeCommitment(height, repeated, final)
				require.NoError(t, err)
				// This independent reference materializes only small trees.
				levels := make([][]common.Hash, height+1)
				levels[0] = make([]common.Hash, tree.leafCount())
				for position := range levels[0] {
					levels[0][position] = repeated
				}
				levels[0][len(levels[0])-1] = final
				for level := uint64(1); level <= height; level++ {
					previous := levels[level-1]
					levels[level] = make([]common.Hash, len(previous)/2)
					for index := range levels[level] {
						levels[level][index] = crypto.Keccak256Hash(previous[2*index][:], previous[2*index+1][:])
					}
				}
				require.Equal(t, levels[height][0], tree.root())
				for position := uint64(0); position < tree.leafCount(); position++ {
					leaf, siblings, err := tree.proof(position)
					require.NoError(t, err)
					require.Equal(t, levels[0][position], leaf)
					require.Equal(t, height, uint64(len(siblings)))
					for level := uint64(0); level < height; level++ {
						require.Equal(t, [32]byte(levels[level][(position>>level)^1]), siblings[level])
					}
					require.Equal(t, tree.root(), rebuildSparseDisputeProof(leaf, position, siblings))
				}
			})
		}
	}
}

func TestSparseDisputeCommitmentCanonicalGeometry(t *testing.T) {
	repeated, changed := common.HexToHash("0x1234"), common.HexToHash("0xabcd")
	// Canonical ArbitrationConstants.sol heights, in root-to-leaf order.
	for _, height := range []uint64{48, 17, 27} {
		t.Run(strconv.FormatUint(height, 10), func(t *testing.T) {
			one, err := newSparseDisputeCommitment(height, repeated, repeated)
			require.NoError(t, err)
			two, err := newSparseDisputeCommitment(height, repeated, changed)
			require.NoError(t, err)
			require.NotEqual(t, one.root(), two.root())
			for _, tree := range []*sparseDisputeCommitment{one, two} {
				require.Equal(t, height+1, uint64(len(tree.repeated)))
				require.Equal(t, height+1, uint64(len(tree.final)), "storage is linear in height, not leaf count")
				for _, position := range []uint64{0, 1, tree.leafCount() / 2, tree.leafCount() - 2, tree.leafCount() - 1} {
					leaf, siblings, err := tree.proof(position)
					require.NoError(t, err)
					require.Equal(t, tree.root(), rebuildSparseDisputeProof(leaf, position, siblings))
					left, right, err := tree.rightmostChildren(height)
					require.NoError(t, err)
					require.Equal(t, tree.root(), crypto.Keccak256Hash(left[:], right[:]))
				}
				leaf, proof, err := tree.proof(tree.leafCount() - 1)
				require.NoError(t, err)
				root := tree.root()
				proof[0][0] ^= 1
				require.NotEqual(t, root, rebuildSparseDisputeProof(leaf, tree.leafCount()-1, proof))
				_, fresh, err := tree.proof(tree.leafCount() - 1)
				require.NoError(t, err)
				require.NotEqual(t, proof[0], fresh[0], "callers cannot mutate cached subtrees through a proof")
				require.Equal(t, root, tree.root())
			}
			assertSparseDisputeBisection(t, one, two)
		})
	}
}

// Follow Match.create, advanceBisection, and sealDivergence without any RPC.
// The helper supplies openings; this independent state progression checks that
// their ordering fits the protocol, including odd/even final revealer parity.
func assertSparseDisputeBisection(t *testing.T, one, two *sparseDisputeCommitment) {
	t.Helper()
	actors := []*sparseDisputeCommitment{one, two}
	otherParent := one.root()
	waitingLeft, waitingRight, err := two.rightmostChildren(two.height)
	require.NoError(t, err)
	var revealer, position uint64
	for height := one.height; height > 1; height-- {
		left, right, err := actors[revealer].rightmostChildren(height)
		require.NoError(t, err)
		require.Equal(t, otherParent, crypto.Keccak256Hash(left[:], right[:]))
		require.Equal(t, waitingLeft, left)
		require.NotEqual(t, waitingRight, right)
		nextLeft, nextRight, err := actors[revealer].rightmostChildren(height - 1)
		require.NoError(t, err)
		require.Equal(t, right, crypto.Keccak256Hash(nextLeft[:], nextRight[:]))
		otherParent, waitingLeft, waitingRight = waitingRight, nextLeft, nextRight
		position += uint64(1) << (height - 1)
		revealer ^= 1
	}
	left, right, err := actors[revealer].rightmostChildren(1)
	require.NoError(t, err)
	require.Equal(t, otherParent, crypto.Keccak256Hash(left[:], right[:]))
	require.Equal(t, waitingLeft, left)
	require.NotEqual(t, waitingRight, right)
	position++
	require.Equal(t, one.leafCount()-1, position)
	require.Equal(t, (one.height-1)%2, revealer)
	agree, proof, err := actors[revealer].proof(position - 1)
	require.NoError(t, err)
	require.Equal(t, one.repeated[0], agree)
	require.Equal(t, actors[revealer].root(), rebuildSparseDisputeProof(agree, position-1, proof))
	if revealer == 0 {
		require.Equal(t, one.final[0], right)
		require.Equal(t, two.final[0], waitingRight)
	} else {
		require.Equal(t, one.final[0], waitingRight)
		require.Equal(t, two.final[0], right)
	}
}

func TestSparseDisputeCommitmentRejectsOutOfRangeRequests(t *testing.T) {
	for _, height := range []uint64{0, maxSparseDisputeHeight + 1, ^uint64(0)} {
		_, err := newSparseDisputeCommitment(height, common.Hash{}, common.Hash{})
		require.Error(t, err)
	}
	tree, err := newSparseDisputeCommitment(maxSparseDisputeHeight, common.Hash{}, common.HexToHash("0x1"))
	require.NoError(t, err)
	for _, height := range []uint64{0, maxSparseDisputeHeight + 1} {
		_, _, err := tree.rightmostChildren(height)
		require.Error(t, err)
	}
	for _, position := range []uint64{tree.leafCount(), ^uint64(0)} {
		_, proof, err := tree.proof(position)
		require.Error(t, err)
		require.Nil(t, proof)
	}
	leaf, proof, err := tree.proof(tree.leafCount() - 1)
	require.NoError(t, err)
	require.Equal(t, tree.root(), rebuildSparseDisputeProof(leaf, tree.leafCount()-1, proof))
}

func rebuildSparseDisputeProof(leaf common.Hash, position uint64, siblings [][32]byte) common.Hash {
	proof := merkle.Leaf(leaf, new(big.Int).SetUint64(position))
	for _, sibling := range siblings {
		proof.PushHash(sibling)
	}
	return proof.BuildRoot()
}
