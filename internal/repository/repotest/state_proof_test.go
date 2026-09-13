// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestKeccakStateProof(t *testing.T) {
	outputsRoot := common.HexToHash("0xa1")
	proof := KeccakStateProof(outputsRoot)
	require.True(t, proof.IsComplete())
	require.Equal(t, outputsRoot, proof.TxBufferDataBlock)
	require.Equal(t, uint64(1), binary.LittleEndian.Uint64(proof.IflagsYDataBlock[8:16]))
	require.Equal(t, uint64(2)<<56|uint64(1)<<48|uint64(1)<<32, binary.LittleEndian.Uint64(proof.HtifTohostDataBlock[16:24]))
	for _, leaf := range []struct {
		name     string
		address  uint64
		data     common.Hash
		siblings [][32]byte
	}{
		{name: "iflags.Y", address: 0x300, data: proof.IflagsYDataBlock, siblings: proof.IflagsYProof},
		{name: "htif.tohost", address: 0x320, data: proof.HtifTohostDataBlock, siblings: proof.HtifTohostProof},
		{name: "outputs root", address: 0x60800000, data: proof.TxBufferDataBlock, siblings: proof.TxBufferProof},
	} {
		t.Run(leaf.name, func(t *testing.T) {
			hash := crypto.Keccak256Hash(leaf.data[:])
			index := leaf.address >> 5
			for _, sibling := range leaf.siblings {
				if index&1 == 0 {
					hash = crypto.Keccak256Hash(hash[:], sibling[:])
				} else {
					hash = crypto.Keccak256Hash(sibling[:], hash[:])
				}
				index >>= 1
			}
			require.Equal(t, proof.MachineHash, hash)
		})
	}
}
