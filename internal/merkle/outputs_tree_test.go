// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/cartesi/rollups-node/internal/merkle/merkletest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	outputsMaxCaseLeaves        = 600
	outputsRightSpan            = 5000
	outputsSmallSpan            = 64
	outputsMiddleSpan           = 1 << 14
	outputsCaseClasses          = 6
	outputsRareHeight           = 10
	outputsSmallHeights         = 9
	outputsEpochs               = 6
	outputsEpochLeaves          = 700
	outputsReferenceHeightCount = 7
	outputsReferencePowerCount  = 10
)

func outputTestHash(index uint64) common.Hash {
	var hash common.Hash
	binary.BigEndian.PutUint64(hash[:], index)
	hash[len(hash)-1] = 1
	return hash
}

func outputTestRandom(first, second uint64) *rand.Rand {
	return rand.New(rand.NewPCG(first, second)) //nolint:gosec // Fixed seeds make differential fixtures reproducible.
}

func outputTestPreContext(rng *rand.Rand, begin uint64, post []common.Hash) []common.Hash {
	if begin == 0 {
		return slices.Clone(post)
	}
	siblings := make([]common.Hash, TREE_DEPTH)
	for index := range siblings {
		siblings[index] = outputTestHash(rng.Uint64())
	}
	return CreatePreContextFromProof(begin-1, outputTestHash(rng.Uint64()), siblings)
}

func outputTestRanges(ranges func(func(uint64, uint64) bool)) [][2]uint64 {
	var result [][2]uint64
	for first, last := range ranges {
		result = append(result, [2]uint64{first, last})
	}
	return result
}

func streamOutputTest(
	t *testing.T,
	pre, post, leaves []common.Hash,
	begin uint64,
	height uint,
) (common.Hash, []common.Hash) {
	t.Helper()
	builder, err := NewOutputsTreeBuilder(pre, post, begin, uint64(len(leaves)), height)
	require.NoError(t, err)
	for first, last := range builder.ChunkRanges() {
		for index := first; index <= last; index++ {
			builder.Append(leaves[index-begin])
		}
	}
	tree := builder.Build()
	require.Equal(t, outputTestRanges(builder.ChunkRanges()), outputTestRanges(tree.ChunkRanges()))
	prover := tree.NewProver()
	matrix := make([]common.Hash, len(leaves)*TREE_DEPTH)
	for first, last := range tree.ChunkRanges() {
		var chunk *ChunkProofs
		for index := first; index <= last; index++ {
			chunk, err = prover.Append(leaves[index-begin])
			require.NoError(t, err)
			if index != last {
				require.Nil(t, chunk)
			}
		}
		require.NotNil(t, chunk)
		require.Equal(t, leaves[first-begin:last-begin+1], chunk.Leaves)
		require.Len(t, chunk.Siblings, len(chunk.Leaves)*TREE_DEPTH)
		copy(matrix[(first-begin)*TREE_DEPTH:], chunk.Siblings)
	}
	return tree.Root(), matrix
}

func compareOutputsWithMatrix(t *testing.T, rng *rand.Rand, begin, count uint64, height uint) {
	t.Helper()
	values := []common.Hash{{1}, {2}, {3}, {4}}
	colors := values[:rng.IntN(len(values))+1]
	leaves := make([]common.Hash, count)
	for index := range leaves {
		leaves[index] = colors[rng.IntN(len(colors))]
	}
	post := CreatePostContext()
	pre := outputTestPreContext(rng, begin, post)
	want, err := merkletest.ComputeSiblingsMatrix(pre, leaves, post, begin)
	require.NoError(t, err)
	root, got := streamOutputTest(t, pre, post, leaves, begin, height)
	require.Equal(t, want, got)
	require.Equal(t, merkletest.ComputeRootHashFromProof(begin, leaves[0], want[:TREE_DEPTH]), root)
}

func TestOutputsTreeFixedBoundaries(t *testing.T) {
	rng := outputTestRandom(0, 0)
	const (
		capacity    = uint64(1) << TREE_DEPTH
		mixedBegin  = 13
		mixedCount  = 21
		mixedHeight = 4
		rightCount  = 17
	)
	for _, height := range []uint{0, TREE_DEPTH} {
		compareOutputsWithMatrix(t, rng, 0, 1, height)
		compareOutputsWithMatrix(t, rng, mixedBegin, mixedCount, height)
		compareOutputsWithMatrix(t, rng, capacity-1, 1, height)
		compareOutputsWithMatrix(t, rng, capacity-rightCount, rightCount, height)
	}
	compareOutputsWithMatrix(t, rng, mixedBegin, mixedCount, mixedHeight)
	post := CreatePostContext()
	builder, err := NewOutputsTreeBuilder(post, post, mixedBegin, mixedCount, mixedHeight)
	require.NoError(t, err)
	require.Equal(t, [][2]uint64{{13, 15}, {16, 31}, {32, 33}}, outputTestRanges(builder.ChunkRanges()))
	var calls int
	for range builder.ChunkRanges() {
		calls++
		break
	}
	require.Equal(t, 1, calls)
	builder, err = NewOutputsTreeBuilder(post, post, 0, capacity, TREE_DEPTH)
	require.NoError(t, err)
	require.Equal(t, [][2]uint64{{0, capacity - 1}}, outputTestRanges(builder.ChunkRanges()))
}

func TestOutputsTreeChunkRangesAreSharedAndAligned(t *testing.T) {
	const begin, count, height = 13, 21, 4
	post := CreatePostContext()
	builder, err := NewOutputsTreeBuilder(post, post, begin, count, height)
	require.NoError(t, err)
	want := [][2]uint64{{13, 15}, {16, 31}, {32, 33}}
	require.Equal(t, want, outputTestRanges(builder.ChunkRanges()))
	for range count {
		builder.Append(common.Hash{1})
	}
	tree := builder.Build()
	require.Equal(t, want, outputTestRanges(tree.ChunkRanges()))
}

func TestOutputsTreeEqualsComputeSiblingsMatrix(t *testing.T) {
	rng := outputTestRandom(1, 1)
	const capacity = uint64(1) << TREE_DEPTH
	for index := range outputsDifferentialCases {
		count := rng.Uint64N(outputsMaxCaseLeaves) + 1
		var begin uint64
		switch index % outputsCaseClasses {
		case 0:
		case 1:
			begin = rng.Uint64N(outputsSmallSpan)
		case 2:
			begin = rng.Uint64N(outputsMiddleSpan)
		case 3:
			begin = (uint64(1) << (rng.Uint64N(TREE_DEPTH) + 1)) - 1
		case 4:
			begin = (rng.Uint64N(capacity/outputParentWidth) * outputParentWidth) + 1
		case 5:
			begin = capacity - rng.Uint64N(outputsRightSpan) - 1
		}
		count = min(count, capacity-begin)
		height := uint(rng.Uint64N(outputsSmallHeights))
		if index%outputsRareHeight == 0 {
			height = uint(rng.Uint64N(TREE_DEPTH + 1))
		}
		compareOutputsWithMatrix(t, rng, begin, count, height)
	}
}

func TestOutputsTreeEqualsReference(t *testing.T) {
	rng := outputTestRandom(1, 0)
	post := CreatePostContext()
	for range outputsReferenceHistories {
		history := make([]common.Hash, 0, outputsEpochLeaves*outputsEpochs)
		var previous *merkletest.OutputHistory
		for range outputsEpochs {
			begin := uint64(len(history))
			count := rng.Uint64N(outputsEpochLeaves) + 1
			if rng.Uint64N(outputParentWidth) == 0 {
				count = uint64(1) << rng.Uint64N(outputsReferencePowerCount)
			}
			leaves := make([]common.Hash, count)
			for index := range leaves {
				leaves[index] = outputTestHash(rng.Uint64())
			}
			pre := post
			if begin > 0 {
				pre = CreatePreContextFromProof(begin-1, history[begin-1], previous.Proof(begin-1))
			}
			history = append(history, leaves...)
			current := merkletest.NewOutputHistory(history)
			height := uint(rng.Uint64N(outputsReferenceHeightCount))
			root, siblings := streamOutputTest(t, pre, post, leaves, begin, height)
			require.Equal(t, current.Root(), root)
			for offset := range count {
				require.Equal(t, current.Proof(begin+offset), siblings[offset*TREE_DEPTH:(offset+1)*TREE_DEPTH])
			}
			previous = current
		}
	}
}

func TestOutputsTreeRejectsBadBuilderInput(t *testing.T) {
	post := CreatePostContext()
	const capacity = uint64(1) << TREE_DEPTH
	for _, test := range []struct {
		pre, post    []common.Hash
		begin, count uint64
		height       uint
	}{
		{nil, post, 0, 1, 0}, {post, nil, 0, 1, 0},
		{post[:TREE_DEPTH-1], post, 0, 1, 0}, {post, post[:TREE_DEPTH-1], 0, 1, 0},
		{post, post, 0, 0, 0}, {post, post, capacity, 1, 0},
		{post, post, capacity - 1, outputParentWidth, 0},
		{post, post, 0, math.MaxUint64, 0}, {post, post, 0, 1, TREE_DEPTH + 1},
	} {
		builder, err := NewOutputsTreeBuilder(test.pre, test.post, test.begin, test.count, test.height)
		require.ErrorIs(t, err, ErrBadInput)
		require.Nil(t, builder)
	}

}

func buildOutputTestTree(t *testing.T) (*OutputsTree, []common.Hash) {
	t.Helper()
	const count = 3
	post := CreatePostContext()
	leaves := []common.Hash{{1}, {2}, {3}}
	builder, err := NewOutputsTreeBuilder(post, post, 0, count, 1)
	require.NoError(t, err)
	for _, hash := range leaves {
		builder.Append(hash)
	}
	tree := builder.Build()
	return tree, leaves
}

func TestOutputsProverRejectsChangedRows(t *testing.T) {
	tree, leaves := buildOutputTestTree(t)
	prover := tree.NewProver()
	_, err := prover.Append(leaves[0])
	require.NoError(t, err)
	_, err = prover.Append(common.Hash{99})
	require.ErrorIs(t, err, ErrOutputsChanged)
}

func TestOutputsTreeIndependentConcurrentProvers(t *testing.T) {
	tree, leaves := buildOutputTestTree(t)
	root := tree.Root()
	t.Run("original", func(t *testing.T) {
		t.Parallel()
		prover := tree.NewProver()
		for _, hash := range leaves {
			_, err := prover.Append(hash)
			require.NoError(t, err)
		}
		require.Equal(t, root, tree.Root())
	})
	t.Run("changed", func(t *testing.T) {
		t.Parallel()
		prover := tree.NewProver()
		_, err := prover.Append(leaves[0])
		require.NoError(t, err)
		_, err = prover.Append(common.Hash{99})
		require.ErrorIs(t, err, ErrOutputsChanged)
		require.Equal(t, root, tree.Root())
	})
}

func TestOutputsTreeCopiesLeavesAndContexts(t *testing.T) {
	post := CreatePostContext()
	pre := slices.Clone(post)
	leaves := []common.Hash{{1}, {2}, {3}}
	const begin = 1
	want, err := merkletest.ComputeSiblingsMatrix(pre, leaves, post, begin)
	require.NoError(t, err)
	builder, err := NewOutputsTreeBuilder(pre, post, begin, uint64(len(leaves)), 1)
	require.NoError(t, err)
	for index := range pre {
		pre[index] = common.Hash{99}
	}
	for index := range post {
		post[index] = common.Hash{98}
	}
	for _, hash := range leaves {
		builder.Append(hash)
	}
	tree := builder.Build()
	prover := tree.NewProver()
	first, err := prover.Append(leaves[0])
	require.NoError(t, err)
	require.Equal(t, leaves[:1], first.Leaves)
	retained := slices.Clone(first.Leaves)
	for _, hash := range leaves[1:] {
		_, err = prover.Append(hash)
		require.NoError(t, err)
	}
	require.Equal(t, retained, first.Leaves)
	require.Equal(t, want[:TREE_DEPTH], first.Siblings)
}
