// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const outputParentWidth = 2

// ErrOutputsChanged identifies different leaves or counts between the two passes.
var ErrOutputsChanged = errors.New("outputs changed between passes")

type outputTreeRange struct {
	begin, end  uint64
	chunkHeight uint
}

// OutputsTreeBuilder collects one root per globally aligned chunk.
// A builder belongs to one pass and is not safe for concurrent use.
type OutputsTreeBuilder struct {
	outputTreeRange
	pre, post        []common.Hash
	next, chunkBegin uint64
	buffer, roots    []common.Hash
}

// NewOutputsTreeBuilder starts a nonempty range in the depth-63 outputs tree.
func NewOutputsTreeBuilder(
	pre, post []common.Hash,
	begin, count uint64,
	chunkHeight uint,
) (*OutputsTreeBuilder, error) {
	capacity := uint64(1) << TREE_DEPTH
	if len(pre) < TREE_DEPTH || len(post) < TREE_DEPTH {
		return nil, fmt.Errorf("outputs contexts require %d hashes: %w", TREE_DEPTH, ErrBadInput)
	}
	if chunkHeight > TREE_DEPTH || count == 0 || begin >= capacity || count > capacity-begin {
		return nil, fmt.Errorf("invalid outputs range or chunk height: %w", ErrBadInput)
	}
	return &OutputsTreeBuilder{
		outputTreeRange: outputTreeRange{begin: begin, end: begin + count, chunkHeight: chunkHeight},
		pre:             slices.Clone(pre[:TREE_DEPTH]),
		post:            slices.Clone(post[:TREE_DEPTH]),
		next:            begin,
		chunkBegin:      begin,
	}, nil
}

// ChunkRanges yields inclusive output-index ranges for the first pass.
func (builder *OutputsTreeBuilder) ChunkRanges() iter.Seq2[uint64, uint64] {
	return outputChunkRanges(builder.outputTreeRange)
}

// Append adds a hash from the declared range without retaining the epoch's leaves.
// The caller validates the scanned row index and count.
func (builder *OutputsTreeBuilder) Append(hash common.Hash) {
	builder.buffer = append(builder.buffer, hash)
	builder.next++
	if builder.next == outputChunkEnd(builder.chunkBegin, builder.outputTreeRange) {
		root := foldOutputChunk(builder.buffer, builder.chunkBegin, builder.pre, builder.post, builder.chunkHeight, nil)
		builder.roots = append(builder.roots, root)
		builder.buffer = builder.buffer[:0]
		builder.chunkBegin = builder.next
	}
}

// Build keeps the chunk roots after the caller validates the complete range.
func (builder *OutputsTreeBuilder) Build() *OutputsTree {
	levels := [][]common.Hash{builder.roots}
	begin := builder.begin >> builder.chunkHeight
	for level := builder.chunkHeight; level < TREE_DEPTH; level++ {
		parents := foldOutputParents(nil, levels[len(levels)-1], begin, builder.pre[level], builder.post[level])
		levels = append(levels, parents)
		begin /= outputParentWidth
	}
	return &OutputsTree{
		outputTreeRange: builder.outputTreeRange,
		pre:             builder.pre,
		post:            builder.post,
		levels:          levels,
	}
}

// OutputsTree retains chunk roots and their ancestors for the second pass.
// It is immutable after Build and can supply independent provers.
type OutputsTree struct {
	outputTreeRange
	pre, post []common.Hash
	levels    [][]common.Hash
}

// Root returns the cumulative outputs root for this epoch.
func (tree *OutputsTree) Root() common.Hash {
	return tree.levels[len(tree.levels)-1][0]
}

// ChunkRanges yields the same inclusive ranges as the first-pass builder.
func (tree *OutputsTree) ChunkRanges() iter.Seq2[uint64, uint64] {
	return outputChunkRanges(tree.outputTreeRange)
}

// ChunkProofs owns the original leaves and their flat, depth-63 sibling matrix.
type ChunkProofs struct {
	Leaves   []common.Hash
	Siblings []common.Hash
}

// OutputsProver rebuilds one chunk at a time and checks it against pass one.
// A prover is not safe for concurrent use.
type OutputsProver struct {
	tree             *OutputsTree
	next, chunkBegin uint64
	buffer           []common.Hash
}

// NewProver creates an independent second pass over the tree's range.
func (tree *OutputsTree) NewProver() *OutputsProver {
	return &OutputsProver{tree: tree, next: tree.begin, chunkBegin: tree.begin}
}

// Append returns proofs when the next hash completes its aligned chunk.
// The caller validates row counts and stops at the first error.
func (prover *OutputsProver) Append(hash common.Hash) (*ChunkProofs, error) {
	prover.buffer = append(prover.buffer, hash)
	prover.next++
	if prover.next != outputChunkEnd(prover.chunkBegin, prover.tree.outputTreeRange) {
		return nil, nil
	}
	proofs, err := prover.proveChunk()
	if err != nil {
		return nil, err
	}
	prover.buffer = prover.buffer[:0]
	prover.chunkBegin = prover.next
	return proofs, nil
}

func (prover *OutputsProver) proveChunk() (*ChunkProofs, error) {
	tree := prover.tree
	leaves := slices.Clone(prover.buffer)
	siblings := make([]common.Hash, len(leaves)*TREE_DEPTH)
	root := foldOutputChunk(prover.buffer, prover.chunkBegin, tree.pre, tree.post, tree.chunkHeight, siblings)
	chunkIndex := (prover.chunkBegin >> tree.chunkHeight) - (tree.begin >> tree.chunkHeight)
	if root != tree.levels[0][chunkIndex] {
		return nil, fmt.Errorf("output chunk root differs from first pass: %w", ErrOutputsChanged)
	}
	for level := tree.chunkHeight; level < TREE_DEPTH; level++ {
		nodes := tree.levels[level-tree.chunkHeight]
		begin := tree.begin >> level
		end := begin + uint64(len(nodes))
		index := (prover.chunkBegin >> level) ^ 1
		sibling := outputNode(nodes, begin, end, index, tree.pre[level], tree.post[level])
		for leaf := range leaves {
			siblings[leaf*TREE_DEPTH+int(level)] = sibling
		}
	}
	return &ChunkProofs{Leaves: leaves, Siblings: siblings}, nil
}

func outputChunkRanges(span outputTreeRange) iter.Seq2[uint64, uint64] {
	return func(yield func(uint64, uint64) bool) {
		for first := span.begin; first < span.end; {
			end := outputChunkEnd(first, span)
			if !yield(first, end-1) {
				return
			}
			first = end
		}
	}
}

func outputChunkEnd(first uint64, span outputTreeRange) uint64 {
	mask := (uint64(1) << span.chunkHeight) - 1
	return min(span.end, (first|mask)+1)
}

func foldOutputChunk(
	nodes []common.Hash,
	first uint64,
	pre, post []common.Hash,
	height uint,
	siblings []common.Hash,
) common.Hash {
	leafCount := len(nodes)
	begin := first
	for level := uint(0); level < height; level++ {
		end := begin + uint64(len(nodes))
		if siblings != nil {
			for leaf := range leafCount {
				index := ((first + uint64(leaf)) >> level) ^ 1
				siblings[leaf*TREE_DEPTH+int(level)] = outputNode(nodes, begin, end, index, pre[level], post[level])
			}
		}
		nodes = foldOutputParents(nodes, nodes, begin, pre[level], post[level])
		begin /= outputParentWidth
	}
	return nodes[0]
}

// foldOutputParents serves both in-place chunk folds and retained upper levels.
func foldOutputParents(
	destination, children []common.Hash,
	begin uint64,
	before, after common.Hash,
) []common.Hash {
	end := begin + uint64(len(children))
	parentCount := len(children) / outputParentWidth
	if begin%outputParentWidth != 0 || len(children)%outputParentWidth != 0 {
		parentCount++
	}
	if destination == nil {
		destination = make([]common.Hash, parentCount)
	} else {
		destination = destination[:parentCount]
	}
	parentBegin, parentEnd := begin/outputParentWidth, (end+1)/outputParentWidth
	for parent := parentBegin; parent < parentEnd; parent++ {
		left := outputNode(children, begin, end, outputParentWidth*parent, before, after)
		right := outputNode(children, begin, end, outputParentWidth*parent+1, before, after)
		destination[parent-parentBegin] = crypto.Keccak256Hash(left[:], right[:])
	}
	return destination
}

func outputNode(nodes []common.Hash, begin, end, index uint64, before, after common.Hash) common.Hash {
	if index < begin {
		return before
	}
	if index >= end {
		return after
	}
	return nodes[index-begin]
}
