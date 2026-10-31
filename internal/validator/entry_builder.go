// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

// entryBuilder maps stored entry roots and epoch padding to protocol leaves.
type entryBuilder struct {
	builder    merkle.StreamingBuilder
	finalEntry *merkle.Tree
	appendErr  error
}

func newEntryBuilder(finalHash common.Hash, height uint32) *entryBuilder {
	return &entryBuilder{finalEntry: merkle.TreeLeaf(finalHash).Iterated(uint64(height))}
}

func (b *entryBuilder) appendRow(row model.EpochStateHash) {
	if b.appendErr != nil {
		return
	}
	entry := merkle.OpaqueTree(row.MachineHash, b.finalEntry.Height)
	if row.MachineHash == b.finalEntry.RootHash {
		entry = b.finalEntry
	}
	b.appendErr = b.builder.AppendRepeated(entry, row.Repetitions)
}

func (b *entryBuilder) finish(inputCount uint64) (*merkle.Tree, error) {
	if b.appendErr != nil {
		return nil, b.appendErr
	}
	// The caller bounds inputCount by the protocol's input capacity.
	remainingEntries := (model.MaxAdvanceStatesPerEpoch - inputCount) << model.Log2InputHashCollectionCapacity
	if remainingEntries != 0 {
		b.appendErr = b.builder.AppendRepeated(b.finalEntry, remainingEntries)
		if b.appendErr != nil {
			return nil, b.appendErr
		}
	}
	return b.builder.Build()
}
