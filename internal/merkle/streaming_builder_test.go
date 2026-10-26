// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/rand/v2"
	"runtime"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestStreamingBuilderLargeAlignedBoundaries(t *testing.T) {
	for _, height := range []uint32{0, 6} {
		one, two := common.HexToHash("0x1234"), common.HexToHash("0x5678")
		first := TreeLeaf(one).Iterated(uint64(height))
		second := OpaqueTree(TreeLeaf(two).Iterated(uint64(height)).RootHash, height)
		var streaming StreamingBuilder
		repetitions := []uint64{1, (uint64(1) << 54) - 1, uint64(1) << 54}
		runs := make([]merkletest.Run, 0, len(repetitions))
		for i, reps := range repetitions {
			entry := []*Tree{first, second, first}[i]
			require.NoError(t, streaming.AppendRepeated(entry, reps))
			runs = append(runs, merkletest.Run{Hash: []common.Hash{one, two, one}[i], Count: reps << height})
		}
		assertStreamingReference(t, &streaming, runs, 55+height)
	}
}

func TestStreamingBuilderNonuniformStructuredSubtrees(t *testing.T) {
	one, two := common.Hash{1}, common.Hash{2}
	three, four := common.Hash{3}, common.Hash{4}
	prefix := TreeLeaf(one).Join(TreeLeaf(two))
	last := TreeLeaf(three).Join(TreeLeaf(four))
	var streaming StreamingBuilder
	require.NoError(t, streaming.Append(OpaqueTree(prefix.RootHash, 1)))
	require.NoError(t, streaming.AppendRepeated(last, 3))
	runs := []merkletest.Run{{Hash: one, Count: 1}, {Hash: two, Count: 1},
		{Hash: three, Count: 1}, {Hash: four, Count: 1}, {Hash: three, Count: 1}, {Hash: four, Count: 1},
		{Hash: three, Count: 1}, {Hash: four, Count: 1}}
	assertStreamingReference(t, &streaming, runs, 3)
}

func streamingHash(rng *rand.Rand) common.Hash {
	var hash common.Hash
	for i := 0; i < len(hash); i += 8 {
		binary.LittleEndian.PutUint64(hash[i:i+8], rng.Uint64())
	}
	return hash
}

func assertStreamingReference(t *testing.T, streaming *StreamingBuilder, runs []merkletest.Run, height uint32) {
	t.Helper()
	tree, err := streaming.Build()
	require.NoError(t, err)
	require.Equal(t, height, tree.Height)
	require.Equal(t, merkletest.Root(runs, height), tree.RootHash)
	proof, err := tree.ProveLast()
	require.NoError(t, err)
	node, siblings := merkletest.LastProof(runs, height)
	require.Equal(t, node, proof.Node)
	require.Equal(t, siblings, proof.Siblings)
	require.Equal(t, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(height)), big.NewInt(1)), proof.Pos)
	if height > 0 {
		left, right := merkletest.RootChildren(runs, height)
		actualLeft, actualRight, err := tree.Subtrees.Children()
		require.NoError(t, err)
		require.Equal(t, left, actualLeft.RootHash)
		require.Equal(t, right, actualRight.RootHash)
		proofLeft, proofRight, err := proof.BuildRootChildren()
		require.NoError(t, err)
		require.Equal(t, left, proofLeft)
		require.Equal(t, right, proofRight)
	}
}

func TestStreamingBuilderErrors(t *testing.T) {
	a := TreeLeaf(common.HexToHash("0x1234"))
	b := TreeLeaf(common.HexToHash("0x5678"))
	var streaming StreamingBuilder
	_, err := streaming.Build()
	require.EqualError(t, err, "empty merkle builder: merkle: invalid input")
	cases := []struct {
		name string
		leaf *Tree
		reps uint64
		text string
	}{
		{"nil", nil, 1, "invalid parameter: merkle: invalid input"},
		{"nil before repetitions", nil, 0, "invalid parameter: merkle: invalid input"},
		{"zero", a, 0, "invalid repetitions: 0: merkle: invalid input"},
	}
	for _, test := range cases {
		before := streaming
		require.EqualError(t, streaming.AppendRepeated(test.leaf, test.reps), test.text, test.name)
		require.Equal(t, before, streaming)
	}
	require.NoError(t, streaming.Append(a))
	for _, test := range cases {
		before := streaming
		require.EqualError(t, streaming.AppendRepeated(test.leaf, test.reps), test.text, test.name)
		require.Equal(t, before, streaming)
	}
	before := streaming
	require.EqualError(t, streaming.Append(a.Iterated(1)),
		"mismatched tree sizes, height: 0 and leaf height: 1: merkle: invalid input")
	require.Equal(t, before, streaming)
	require.EqualError(t, streaming.AppendRepeated(a.Iterated(1), 0), "invalid repetitions: 0: merkle: invalid input")
	require.Equal(t, before, streaming)
	require.NoError(t, streaming.AppendRepeated(b, 2))
	_, err = streaming.Build()
	require.EqualError(t, err, "builder has 3 leafs, which is not a power of two: merkle: invalid input")
	require.NoError(t, streaming.Append(a))
	first, err := streaming.Build()
	require.NoError(t, err)
	second, err := streaming.Build()
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, streaming.AppendRepeated(b, 4))
	assertStreamingReference(t, &streaming, []merkletest.Run{
		{Hash: a.RootHash, Count: 1}, {Hash: b.RootHash, Count: 2},
		{Hash: a.RootHash, Count: 1}, {Hash: b.RootHash, Count: 4}}, 3)
}

func TestStreamingBuilderUint64Boundary(t *testing.T) {
	leaf := TreeLeaf(common.HexToHash("0xabcd"))
	var streaming StreamingBuilder
	require.NoError(t, streaming.AppendRepeated(leaf, uint64(1)<<63))
	assertStreamingReference(t, &streaming, []merkletest.Run{{Hash: leaf.RootHash, Count: uint64(1) << 63}}, 63)
	require.NoError(t, streaming.AppendRepeated(leaf, (uint64(1)<<63)-1))
	for _, reps := range []uint64{1, 2, math.MaxUint64} {
		before := streaming
		require.EqualError(t, streaming.AppendRepeated(leaf, reps), "merkle tree overflow: merkle: invalid input")
		require.Equal(t, before, streaming)
	}
	before := streaming
	require.EqualError(t, streaming.AppendRepeated(leaf.Iterated(1), 1),
		"mismatched tree sizes, height: 0 and leaf height: 1: merkle: invalid input")
	require.Equal(t, before, streaming)
	var maximum StreamingBuilder
	require.NoError(t, maximum.AppendRepeated(leaf, math.MaxUint64))
	_, err := maximum.Build()
	require.EqualError(t, err, "builder has 18446744073709551615 leafs, which is not a power of two: merkle: invalid input")
}

func TestStreamingBuilderOpaqueLastLeaf(t *testing.T) {
	leaf := TreeLeaf(common.HexToHash("0xcafe")).Iterated(6)
	opaque := OpaqueTree(common.HexToHash("0xface"), 6)
	var streaming StreamingBuilder
	require.NoError(t, streaming.AppendRepeated(leaf, (uint64(1)<<48)-1))
	require.NoError(t, streaming.Append(opaque))
	tree, err := streaming.Build()
	require.NoError(t, err)
	require.Equal(t, uint32(54), tree.Height)
	runs := []merkletest.Run{{Hash: leaf.RootHash, Count: (uint64(1) << 48) - 1}, {Hash: opaque.RootHash, Count: 1}}
	require.Equal(t, merkletest.Root(runs, 48), tree.RootHash)
	_, err = tree.ProveLast()
	require.ErrorIs(t, err, ErrOpaqueTree)
}

func TestStreamingBuilderMatchesRecursiveReference(t *testing.T) {
	t.Run("other indices", assertStreamingProofsAtOtherIndices)
	rng := rand.New(rand.NewPCG(23, 71)) //nolint:gosec // Reproducible test cases, not security material.
	for trial := range streamingDifferentialCases {
		t.Run(fmt.Sprint(trial), func(t *testing.T) {
			entryHeight := rng.Uint32N(7)
			entries := uint64(1) << rng.Uint32N(56)
			var runs []merkletest.Run
			var streaming StreamingBuilder
			for remaining := entries; remaining != 0; {
				reps := 1 + rng.Uint64N(remaining)
				hash := streamingHash(rng)
				structured := TreeLeaf(hash).Iterated(uint64(entryHeight))
				entry := structured
				// Keep the last entry structured; earlier opaque entries have known leaf contents.
				if reps != remaining && trial%2 == 0 {
					entry = OpaqueTree(structured.RootHash, entryHeight)
				}
				require.NoError(t, streaming.AppendRepeated(entry, reps))
				runs = append(runs, merkletest.Run{Hash: hash, Count: reps << entryHeight})
				remaining -= reps
			}
			height := uint32(bits.TrailingZeros64(entries)) + entryHeight //nolint:gosec // At most 55 entry levels.
			tree, err := streaming.Build()
			require.NoError(t, err)
			require.Equal(t, height, tree.Height)
			require.Equal(t, merkletest.Root(runs, height), tree.RootHash)
			proof, err := tree.ProveLast()
			require.NoError(t, err)
			node, siblings := merkletest.LastProof(runs, height)
			require.Equal(t, node, proof.Node)
			require.Equal(t, siblings, proof.Siblings)
			require.Equal(t, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(height)), big.NewInt(1)), proof.Pos)
			require.True(t, proof.VerifyRoot(tree.RootHash))
			if height > 0 {
				left, right := merkletest.RootChildren(runs, height)
				actualLeft, actualRight, err := tree.Subtrees.Children()
				require.NoError(t, err)
				require.Equal(t, left, actualLeft.RootHash)
				require.Equal(t, right, actualRight.RootHash)
				proofLeft, proofRight, err := proof.BuildRootChildren()
				require.NoError(t, err)
				require.Equal(t, left, proofLeft)
				require.Equal(t, right, proofRight)
			}
		})
	}
}

func assertStreamingProofsAtOtherIndices(t *testing.T) {
	t.Helper()
	left := common.HexToHash("0x1234")
	right := common.HexToHash("0x5678")
	var ordinary StreamingBuilder
	require.NoError(t, ordinary.Append(TreeLeaf(left)))
	require.NoError(t, ordinary.Append(TreeLeaf(right)))
	two, err := ordinary.Build()
	require.NoError(t, err)
	require.Equal(t, merkletest.Root([]merkletest.Run{{Hash: left, Count: 1}, {Hash: right, Count: 1}}, 1), two.RootHash)
	for _, index := range []int64{0, 1} {
		proof, err := two.ProveLeaf(big.NewInt(index))
		require.NoError(t, err)
		require.Equal(t, []common.Hash{left, right}[index], proof.Node)
		require.True(t, proof.VerifyRoot(two.RootHash))
	}
	var pruned StreamingBuilder
	require.NoError(t, pruned.AppendRepeated(TreeLeaf(left), 3))
	require.NoError(t, pruned.Append(TreeLeaf(right)))
	four, err := pruned.Build()
	require.NoError(t, err)
	_, err = four.ProveLeaf(big.NewInt(0))
	require.ErrorIs(t, err, ErrOpaqueTree)
	proof, err := four.ProveLast()
	require.NoError(t, err)
	require.True(t, proof.VerifyRoot(four.RootHash))
}

func BenchmarkStreamingBuilderAppend(b *testing.B) {
	leaf := OpaqueTree(common.HexToHash("0xabcd"), 6)
	var builder StreamingBuilder
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := builder.Append(leaf); err != nil {
			b.Fatal(err)
		}
	}
	runtime.KeepAlive(&builder)
}
