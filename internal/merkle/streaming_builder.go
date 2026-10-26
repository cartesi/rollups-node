// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package merkle

import (
	"fmt"
	"hash"
	"math"
	"math/bits"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/keccak"
)

// StreamingBuilder builds trees with memory independent of the number of appends.
// It keeps one root per level and the last aligned block of the latest run.
// Build retains the last leaf for proof generation. Other indices can return
// ErrOpaqueTree; every successful proof is correct. An opaque last entry above
// height zero cannot be proved, by this representation.
//
// AppendRepeated and Build reject invalid inputs, with two special cases:
// totals of 2^64 entries or more overflow, and height is checked before overflow.
// The zero value is ready to use. It is not safe for concurrent use.
type StreamingBuilder struct {
	frontier     [64]common.Hash
	count        uint64 // Carried entries, excluding the pending block.
	pendingTree  *Tree  // A non-nil pointer also marks initialization.
	pendingRoot  common.Hash
	pendingPower uint32
	hasher       hash.Hash
	hashInput    [2 * common.HashLength]byte
	hashOutput   common.Hash
}

func (b *StreamingBuilder) Append(leaf *Tree) error {
	return b.AppendRepeated(leaf, 1)
}

// AppendRepeated appends reps equal-height entries. A rejected append changes nothing.
func (b *StreamingBuilder) AppendRepeated(leaf *Tree, reps uint64) error {
	if leaf == nil {
		return fmt.Errorf("invalid parameter: %w", ErrBadInput)
	}
	if reps == 0 {
		return fmt.Errorf("invalid repetitions: %v: %w", reps, ErrBadInput)
	}
	var total uint64
	if b.pendingTree != nil {
		if b.pendingTree.Height != leaf.Height {
			return fmt.Errorf("mismatched tree sizes, height: %v and leaf height: %v: %w", b.pendingTree.Height, leaf.Height, ErrBadInput)
		}
		total = b.count + (uint64(1) << b.pendingPower)
	}
	if reps > math.MaxUint64-total {
		return fmt.Errorf("merkle tree overflow: %w", ErrBadInput)
	}

	if b.pendingTree != nil {
		b.carry(b.pendingRoot, b.pendingPower)
	}
	var repeated [64]common.Hash
	repeated[0] = leaf.RootHash
	var computed uint32
	for remaining := reps; remaining != 0; {
		// remaining is positive, so its logarithm and the aligned power are in [0, 63].
		power := uint32(min(bits.TrailingZeros64(b.count), bits.Len64(remaining)-1)) //nolint:gosec
		for computed < power {
			repeated[computed+1] = b.hashPair(repeated[computed], repeated[computed])
			computed++
		}
		block := uint64(1) << power
		if remaining == block {
			b.pendingTree = leaf
			b.pendingRoot = repeated[power]
			b.pendingPower = power
			return nil
		}
		b.carry(repeated[power], power)
		remaining -= block
	}
	return nil
}

func (b *StreamingBuilder) carry(root common.Hash, power uint32) {
	level := power
	for b.count&(uint64(1)<<level) != 0 {
		root = b.hashPair(b.frontier[level], root)
		level++
	}
	b.frontier[level] = root
	b.count += uint64(1) << power
}

func (b *StreamingBuilder) hashPair(left, right common.Hash) common.Hash {
	if b.hasher == nil {
		b.hasher = keccak.NewLegacyKeccak256()
	}
	copy(b.hashInput[:common.HashLength], left[:])
	copy(b.hashInput[common.HashLength:], right[:])
	b.hasher.Reset()
	_, _ = b.hasher.Write(b.hashInput[:])
	b.hasher.Sum(b.hashOutput[:0])
	return b.hashOutput
}

// Build returns a pruned tree without changing the builder.
func (b *StreamingBuilder) Build() (*Tree, error) {
	if b.pendingTree == nil {
		return nil, fmt.Errorf("empty merkle builder: %w", ErrBadInput)
	}
	total := b.count + (uint64(1) << b.pendingPower)
	if total&(total-1) != 0 {
		return nil, fmt.Errorf("builder has %v leafs, which is not a power of two: %w", total, ErrBadInput)
	}
	spine := b.pendingTree.Iterated(uint64(b.pendingPower))
	for level := b.pendingPower; (uint64(1) << level) < total; level++ {
		spine = OpaqueTree(b.frontier[level], b.pendingTree.Height+level).Join(spine)
	}
	return spine, nil
}
