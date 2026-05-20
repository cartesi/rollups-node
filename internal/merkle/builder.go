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

var (
	ErrBadInput  = errors.New("merkle: invalid input")
	ErrInvariant = errors.New("merkle: internal invariant violated")
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
		return zero, zero, fmt.Errorf("siblings array is empty: %w", ErrBadInput)
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

func (inner *InnerNode) Children() (*Tree, *Tree, error) {
	if !inner.Valid() {
		return nil, nil, fmt.Errorf("invalid InnerNode state: %+v: %w", inner, ErrInvariant)
	}

	if inner.Child != nil {
		return inner.Child, inner.Child, nil
	} else {
		return inner.LHS, inner.RHS, nil
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

func (tree *Tree) FindChildByHash(hash common.Hash) (*Tree, error) {
	if tree.RootHash == hash {
		return tree, nil
	}
	if inner := tree.Subtrees; inner != nil {
		lhs, rhs, err := inner.Children()
		if err != nil {
			return nil, err
		}

		child, err := lhs.FindChildByHash(hash)
		if err != nil {
			return nil, err
		}
		if child != nil {
			return child, nil
		}

		// For iterated nodes lhs == rhs, so the right-hand search is redundant.
		if lhs != rhs {
			child, err = rhs.FindChildByHash(hash)
			if err != nil {
				return nil, err
			}
			if child != nil {
				return child, nil
			}
		}
	}
	return nil, nil // not found
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

func (tree *Tree) ProveLeaf(index *big.Int) (*Proof, error) {
	return tree.ProveLeafRec(index)
}

func (tree *Tree) ProveLast() (*Proof, error) {
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

func (tree *Tree) ProveLeafRec(index *big.Int) (*Proof, error) {
	numLeafs := new(big.Int).Lsh(one, uint(tree.Height))
	if numLeafs.Cmp(index) <= 0 {
		return nil, fmt.Errorf("index out of bounds: %v, %v: %w", numLeafs, index, ErrBadInput)
	}

	subtree := tree.Subtrees
	if subtree == nil {
		if index.Cmp(zero) != 0 {
			return nil, fmt.Errorf("invalid Tree state: %v: %w", tree, ErrInvariant)
		}
		if tree.Height != 0 {
			return nil, fmt.Errorf("invalid Tree state: %v: %w", tree, ErrInvariant)
		}
		return Leaf(tree.RootHash, index), nil
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

	lhs, rhs, err := subtree.Children()
	if err != nil {
		return nil, err
	}

	if isLeftLeaf {
		proof, err := lhs.ProveLeafRec(innerIndex)
		if err != nil {
			return nil, err
		}
		proof.PushHash(rhs.RootHash)
		proof.Pos = index
		return proof, err
	} else {
		proof, err := rhs.ProveLeafRec(innerIndex)
		if err != nil {
			return nil, err
		}
		proof.PushHash(lhs.RootHash)
		proof.Pos = index
		return proof, err
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

func (b *Builder) Append(leaf *Tree) error {
	return b.AppendRepeated(leaf, big.NewInt(1))
}

func (b *Builder) AppendRepeatedUint64(leaf *Tree, reps uint64) error {
	return b.AppendRepeated(leaf, new(big.Int).SetUint64(reps))
}

func (b *Builder) AppendRepeated(leaf *Tree, reps *big.Int) error {
	if leaf == nil || reps == nil {
		return fmt.Errorf("invalid parameter: %w", ErrBadInput)
	}
	if reps.Cmp(zero) <= 0 {
		return fmt.Errorf("invalid repetitions: %v: %w", reps, ErrBadInput)
	}

	accumulatedCount, err := b.CalculateAccumulatedCount(reps)
	if err != nil {
		return err
	}

	if height, ok := b.Height(); ok {
		if height != leaf.Height {
			return fmt.Errorf("mismatched tree sizes, height: %v and leaf height: %v: %w", height, leaf.Height, ErrBadInput)
		}
	}
	b.Trees = append(b.Trees, Node{
		Tree:             leaf,
		AccumulatedCount: accumulatedCount,
	})
	return nil
}

func (b *Builder) Build() (*Tree, error) {
	if count, ok := b.Count(); ok {
		if !isCountPow2(count) {
			return nil, fmt.Errorf("builder has %v leafs, which is not a power of two: %w", count, ErrBadInput)
		}
		log2Size := countTrailingZeroes(count)
		return buildMerkle(b.Trees, log2Size, big.NewInt(0)), nil
	} else {
		return nil, fmt.Errorf("empty merkle builder: %w", ErrBadInput)
	}
}

func (b *Builder) CalculateAccumulatedCount(reps *big.Int) (*big.Int, error) {
	n := len(b.Trees)
	if n != 0 {
		if reps.Cmp(zero) == 0 {
			return nil, fmt.Errorf("merkle builder is full")
		}

		accumulatedCount := new(big.Int).And(
			new(big.Int).Add(reps, b.Trees[n-1].AccumulatedCount),
			overflowMask,
		)
		if reps.Cmp(accumulatedCount) >= 0 {
			return nil, fmt.Errorf("merkle tree overflow: %w", ErrBadInput)
		}
		return accumulatedCount, nil
	} else {
		return reps, nil
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
