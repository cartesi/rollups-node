// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	zero          = big.NewInt(0)
	one           = big.NewInt(1)
	overflowValue = new(big.Int).Lsh(one, 256)
	overflowMask  = new(big.Int).Sub(overflowValue, one)
)

// MerkleProof: dave/common-rs/merkle/src/tree.rs
type Proof struct {
	Pos      *big.Int
	Node     common.Hash
	Siblings []common.Hash
}

func Leaf(node common.Hash, pos *big.Int) *Proof {
	return &Proof{
		Node:     node,
		Pos:      pos,
		Siblings: nil,
	}
}

func (proof *Proof) BuildRoot() common.Hash {
	two := big.NewInt(2)
	rootHash := proof.Node

	for i, s := range proof.Siblings {

		// ((pos >> i) % 2) == 0
		if new(big.Int).Rem(new(big.Int).Rsh(proof.Pos, uint(i)), two).Cmp(zero) == 0 {
			rootHash = crypto.Keccak256Hash(rootHash[:], s[:])
		} else {
			rootHash = crypto.Keccak256Hash(s[:], rootHash[:])
		}
	}
	return rootHash
}

func (proof *Proof) BuildRootChildren() (common.Hash, common.Hash, error) {
	if len(proof.Siblings) == 0 {
		zero := common.Hash{}
		return zero, zero, errors.New("Siblings array is empty")
	}
	two := big.NewInt(2)
	height := len(proof.Siblings)
	childHash := proof.Node

	for i, s := range proof.Siblings[:height-1] {

		// ((pos >> i) % 2) == 0
		if new(big.Int).Rem(new(big.Int).Rsh(proof.Pos, uint(i)), two).Cmp(zero) == 0 {
			childHash = crypto.Keccak256Hash(childHash[:], s[:])
		} else {
			childHash = crypto.Keccak256Hash(s[:], childHash[:])
		}
	}

	// ((pos >> (height-1)) % 2) == 0
	if new(big.Int).Rem(new(big.Int).Rsh(proof.Pos, uint(height-1)), two).Cmp(zero) == 0 {
		return childHash, proof.Siblings[height-1], nil
	} else {
		return proof.Siblings[height-1], childHash, nil
	}
}

func (proof *Proof) VerifyRoot(other common.Hash) bool {
	return proof.BuildRoot() == other
}

func (proof *Proof) PushHash(h common.Hash) {
	proof.Siblings = append(proof.Siblings, h)
}

func RootChildrenFromProof(leaf common.Hash, siblings []common.Hash, index uint64) (common.Hash, common.Hash, error) {
	p := &Proof{
		Pos:      new(big.Int).SetUint64(index),
		Node:     leaf,
		Siblings: siblings,
	}
	return p.BuildRootChildren()
}

////////////////////////////////////////////////////////////////////////////////

// MerkleTree: dave/common-rs/merkle/src/tree.rs
type Tree struct {
	RootHash common.Hash
	Height   uint32
	Subtrees *InnerNode
}

// InnerNode: dave/common-rs/merkle/src/tree.rs
// Emulate the rust enum type with a struct containing both {Pair, Iterated}.
type InnerNode struct {
	// Pair
	LHS, RHS *Tree

	// Iterated
	Child *Tree
}

func (inner *InnerNode) Valid() bool {
	isPair := (inner.LHS != nil && inner.RHS != nil)
	isIterated := inner.Child != nil
	return (isPair || isIterated) && !(isPair && isIterated) // xor
}

func (inner *InnerNode) Children() (*Tree, *Tree) {
	if !inner.Valid() {
		panic(fmt.Sprintf("invalid InnerNode state: %v\n", inner))
	}

	if inner.Child != nil {
		return inner.Child, inner.Child
	} else {
		return inner.LHS, inner.RHS
	}
}

func TreeLeaf(hash common.Hash) *Tree {
	return &Tree{
		Height:   0,
		RootHash: hash,
		Subtrees: nil,
	}
}

func (tree *Tree) GetRootHash() common.Hash {
	return tree.RootHash
}

func (tree *Tree) FindChildByHash(hash common.Hash) *InnerNode {
	if inner := tree.Subtrees; inner != nil {
		if !inner.Valid() {
			panic(fmt.Sprintf("invalid InnerNode state: %v\n", inner))
		}

		if inner.Child != nil {
			child := inner.Child.FindChildByHash(hash)
			if child != nil {
				return child
			}
		} else {
			lhs := inner.LHS.FindChildByHash(hash)
			if lhs != nil {
				return lhs
			}

			rhs := inner.LHS.FindChildByHash(hash)
			if rhs != nil {
				return rhs
			}
		}
	}
	return nil // not found
}

func (tree *Tree) Join(other *Tree) *Tree {
	return &Tree{
		RootHash: crypto.Keccak256Hash(tree.RootHash[:], other.RootHash[:]),
		Height:   tree.Height + 1,
		Subtrees: &InnerNode{
			LHS: tree,
			RHS: other,
		},
	}
}

func (tree *Tree) Iterated(rep uint64) *Tree {
	root := tree
	for range rep {
		root = &Tree{
			RootHash: crypto.Keccak256Hash(root.RootHash[:], root.RootHash[:]),
			Height:   root.Height + 1,
			Subtrees: &InnerNode{
				Child: root,
			},
		}
	}
	return root
}

func (tree *Tree) ProveLeaf(index *big.Int) *Proof {
	return tree.ProveLeafRec(index)
}

func (tree *Tree) ProveLast() *Proof {
	// index = (1 << height) - 1
	index := new(big.Int).Sub(
		new(big.Int).Lsh(
			one,
			uint(tree.Height),
		),
		one,
	)
	return tree.ProveLeaf(index)
}

func (tree *Tree) ProveLeafRec(index *big.Int) *Proof {
	numLeafs := new(big.Int).Lsh(one, uint(tree.Height))
	if numLeafs.Cmp(index) <= 0 {
		panic(fmt.Sprintf("index out of bounds: %v, %v", numLeafs, index))
	}

	subtree := tree.Subtrees
	if subtree == nil {
		if index.Cmp(zero) != 0 {
			panic(fmt.Sprintf("invalid Tree state: %v", tree))
		}
		if tree.Height != 0 {
			panic(fmt.Sprintf("invalid Tree state: %v", tree))
		}
		return Leaf(tree.RootHash, index)
	}

	shiftAmount := uint(tree.Height - 1)
	isLeftLeaf := new(big.Int).Rsh(index, shiftAmount).Cmp(zero) == 0

	// innerIndex = index & !(1 << shiftAmount)
	innerIndex := new(big.Int).And(
		index,
		new(big.Int).Not(
			new(big.Int).Lsh(
				one,
				shiftAmount,
			),
		),
	)

	lhs, rhs := subtree.Children()
	if isLeftLeaf {
		proof := lhs.ProveLeafRec(innerIndex)
		proof.PushHash(rhs.RootHash)
		proof.Pos = index
		return proof
	} else {
		proof := rhs.ProveLeafRec(innerIndex)
		proof.PushHash(lhs.RootHash)
		proof.Pos = index
		return proof
	}
}

////////////////////////////////////////////////////////////////////////////////

// Node: common-rs/merkle/src/tree_builder.rs
type Node struct {
	Tree             *Tree
	AccumulatedCount *big.Int
}

type Builder struct {
	Trees []Node
}

func (b *Builder) Height() (uint32, bool) {
	n := len(b.Trees)
	if n == 0 {
		return 0, false
	}
	return b.Trees[n-1].Tree.Height, true
}

func (b *Builder) Count() (*big.Int, bool) {
	n := len(b.Trees)
	if n == 0 {
		return nil, false
	}
	return b.Trees[n-1].AccumulatedCount, true
}

func (b *Builder) CanBuild() bool {
	n := len(b.Trees)
	if n == 0 {
		return false
	}
	return isPow2(b.Trees[n-1].AccumulatedCount)
}

func (b *Builder) Append(leaf *Tree) {
	b.AppendRepeated(leaf, big.NewInt(1))
}

func (b *Builder) AppendRepeatedUint64(leaf *Tree, reps uint64) {
	b.AppendRepeated(leaf, new(big.Int).SetUint64(reps))
}

func (b *Builder) AppendRepeated(leaf *Tree, reps *big.Int) {
	if reps.Cmp(zero) <= 0 {
		panic("invalid repetitions")
	}

	accumulatedCount := b.CalculateAccumulatedCount(reps)
	if height, ok := b.Height(); ok {
		if height != leaf.Height {
			panic("mismatched tree size")
		}
	}
	b.Trees = append(b.Trees, Node{
		Tree:             leaf,
		AccumulatedCount: accumulatedCount,
	})
}

func (b *Builder) Build() *Tree {
	if count, ok := b.Count(); ok {
		if !isCountPow2(count) {
			panic(fmt.Sprintf("builder has %v leafs, which is not a power of two", count))
		}
		log2Size := countTrailingZeroes(count)
		return buildMerkle(b.Trees, log2Size, big.NewInt(0))
	} else {
		panic("no leafs in the merkle builder")
	}
}

func (b *Builder) CalculateAccumulatedCount(reps *big.Int) *big.Int {
	n := len(b.Trees)
	if n != 0 {
		if reps.Cmp(zero) == 0 {
			panic("merkle builder is full")
		}

		accumulatedCount := new(big.Int).And(
			new(big.Int).Add(reps, b.Trees[n-1].AccumulatedCount),
			overflowMask,
		)
		if reps.Cmp(accumulatedCount) >= 0 {
			panic("merkle tree overflow")
		}
		return accumulatedCount
	} else {
		return reps
	}
}

func buildMerkle(trees []Node, log2Size uint, stride *big.Int) *Tree {
	size := new(big.Int).And(
		new(big.Int).Lsh(one, log2Size),
		overflowMask,
	)

	firstTime := new(big.Int).Add(new(big.Int).Mul(stride, size), one)
	lastTime := new(big.Int).Mul(new(big.Int).Add(stride, one), size)

	firstCell := findCellContaining(trees, firstTime)
	lastCell := findCellContaining(trees, lastTime)

	if firstCell == lastCell {
		tree := trees[firstCell].Tree
		iterated := tree.Iterated(uint64(log2Size))
		return iterated
	}

	left := buildMerkle(trees[firstCell:(lastCell+1)],
		log2Size-1,
		new(big.Int).Lsh(stride, 1),
	)

	right := buildMerkle(trees[firstCell:(lastCell+1)],
		log2Size-1,
		new(big.Int).Add(new(big.Int).Lsh(stride, 1), one),
	)

	return left.Join(right)
}

func findCellContaining(trees []Node, elem *big.Int) uint {
	left := uint(0)
	right := uint(len(trees) - 1)

	for left < right {
		needle := left + (right-left)/2

		x := new(big.Int).And(
			new(big.Int).Sub(trees[needle].AccumulatedCount, one),
			overflowMask,
		)
		y := new(big.Int).And(
			new(big.Int).Sub(elem, one),
			overflowMask,
		)
		if x.Cmp(y) < 0 {
			left = needle + 1
		} else {
			right = needle
		}
	}
	return left
}

////////////////////////////////////////////////////////////////////////////////

func isPow2(x *big.Int) bool {
	if x.Sign() <= 0 {
		return false
	}

	// x & (x-1) == 0
	return new(big.Int).And(
		x,
		new(big.Int).Sub(
			x,
			one,
		),
	).Cmp(zero) == 0
}

func isCountPow2(x *big.Int) bool {
	return x.Cmp(big.NewInt(0)) == 0 || isPow2(x)
}

func countTrailingZeroes(x *big.Int) uint {
	count := uint(0)

	// each byte from least to most significant
brk:
	for _, b := range slices.Backward(x.Bytes()) {
		for i := range 8 {
			if b>>i&1 != 0 {
				break brk
			}
			count++
		}
	}
	return count
}
