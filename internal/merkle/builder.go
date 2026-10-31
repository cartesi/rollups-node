// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	zero = big.NewInt(0)
	one  = big.NewInt(1)
)

var (
	ErrBadInput   = errors.New("merkle: invalid input")
	ErrInvariant  = errors.New("merkle: internal invariant violated")
	ErrOpaqueTree = errors.New("merkle: proof enters an opaque subtree")
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
	opaque   bool
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

// OpaqueTree represents a subtree known only by its root and height.
// Height zero is an ordinary leaf and remains provable.
func OpaqueTree(root common.Hash, height uint32) *Tree {
	if height == 0 {
		return TreeLeaf(root)
	}
	return &Tree{RootHash: root, Height: height, opaque: true}
}

func (tree *Tree) GetRootHash() common.Hash {
	return tree.RootHash
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
		if tree.opaque {
			return nil, fmt.Errorf("proof enters subtree of height %d, root %v: %w", tree.Height, tree.RootHash, ErrOpaqueTree)
		}
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

	// innerIndex = index & ~(1 << shiftAmount)
	innerIndex := new(big.Int).AndNot(
		index,
		new(big.Int).Lsh(
			one,
			shiftAmount,
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
		return proof, nil
	} else {
		proof, err := rhs.ProveLeafRec(innerIndex)
		if err != nil {
			return nil, err
		}
		proof.PushHash(lhs.RootHash)
		proof.Pos = index
		return proof, nil
	}
}

////////////////////////////////////////////////////////////////////////////////
