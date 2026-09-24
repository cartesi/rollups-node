// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"encoding/binary"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// KeccakStateProof builds a valid machine proof with accepted-state registers.
// DummyStateProof remains available for non-cryptographic fixtures.
func KeccakStateProof(outputsRoot common.Hash) StateProof {
	const (
		iflagsYDataBlockAddress    uint64 = 0x300
		htifTohostDataBlockAddress uint64 = 0x320
		txBufferDataBlockAddress          = 0x60800000
		acceptedHtifTohost         uint64 = 2<<56 | 1<<48 | 1<<32
	)
	var iflagsYDataBlock common.Hash
	binary.LittleEndian.PutUint64(iflagsYDataBlock[8:], 1)
	var htifTohostDataBlock common.Hash
	binary.LittleEndian.PutUint64(htifTohostDataBlock[16:], acceptedHtifTohost)
	blocks := map[uint64]common.Hash{
		iflagsYDataBlockAddress:    iflagsYDataBlock,
		htifTohostDataBlockAddress: htifTohostDataBlock,
		txBufferDataBlockAddress:   outputsRoot,
	}
	machineHash, proofs := buildKeccakStateProofs(blocks)
	return StateProof{
		MachineHash:         machineHash,
		IflagsYDataBlock:    iflagsYDataBlock,
		IflagsYProof:        proofs[iflagsYDataBlockAddress],
		HtifTohostDataBlock: htifTohostDataBlock,
		HtifTohostProof:     proofs[htifTohostDataBlockAddress],
		TxBufferDataBlock:   outputsRoot,
		TxBufferProof:       proofs[txBufferDataBlockAddress],
	}
}

func buildKeccakStateProofs(blocks map[uint64]common.Hash) (common.Hash, map[uint64][][32]byte) {
	const log2DataBlockSize = 5
	defaultHashes := make([]common.Hash, StateProofSiblingCount+1)
	defaultHashes[0] = crypto.Keccak256Hash(make([]byte, 1<<log2DataBlockSize))
	for level := 1; level <= StateProofSiblingCount; level++ {
		defaultHashes[level] = crypto.Keccak256Hash(defaultHashes[level-1][:], defaultHashes[level-1][:])
	}
	current := make(map[uint64]common.Hash, len(blocks))
	indexes := make(map[uint64]uint64, len(blocks))
	siblings := make(map[uint64][][32]byte, len(blocks))
	for address, block := range blocks {
		index := address >> log2DataBlockSize
		indexes[address] = index
		current[index] = crypto.Keccak256Hash(block[:])
		siblings[address] = make([][32]byte, 0, StateProofSiblingCount)
	}
	for level := 0; level < StateProofSiblingCount; level++ {
		for address, originalIndex := range indexes {
			index := originalIndex >> level
			sibling, ok := current[index^1]
			if !ok {
				sibling = defaultHashes[level]
			}
			siblings[address] = append(siblings[address], sibling)
		}
		parents := make(map[uint64]struct{}, len(current))
		for index := range current {
			parents[index>>1] = struct{}{}
		}
		next := make(map[uint64]common.Hash, len(parents))
		for parent := range parents {
			left, ok := current[parent<<1]
			if !ok {
				left = defaultHashes[level]
			}
			right, ok := current[parent<<1|1]
			if !ok {
				right = defaultHashes[level]
			}
			next[parent] = crypto.Keccak256Hash(left[:], right[:])
		}
		current = next
	}
	return current[0], siblings
}
