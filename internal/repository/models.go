// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package data

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Input struct {
	// Input index starting from genesis
	Index int `json:"index"`
	// Status of the input
	Status string `json:"status"`
	// Input data as a blob, starting with '0x'
	Blob hexutil.Bytes `json:"blob"`
}

type Output struct {
	// Input whose processing produced the output
	InputIndex int `json:"inputIndex"`
	// Output index within the context of the input that produced it
	Index int `json:"index"`
	// Output data as a blob, starting with '0x'
	Blob hexutil.Bytes `json:"blob"`
}

type Proof struct {
	// Input whose processing produced the output
	InputIndex int `json:"inputIndex"`
	// Output that generated the proof
	OutputIndex int `json:"outputIndex"`
	// First index of the input range
	FirstInputIndex int `json:"firstIndex"`
	// Last index of the input range
	LastInputIndex int `json:"lastIndex"`
	// Local input index within the context of the related epoch
	InputIndexWithinEpoch int `json:"inputIndexWithinEpoch"`
	// Output index within the context of the input that produced it
	OutputIndexWithinInput int `json:"outputIndexWithinInput"`
	// Merkle root of all output hashes of the related input
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	OutputHashesRootHash hexutil.Bytes `json:"outputHashesRootHash"`
	// Merkle root of all notice hashes of the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	OutputsEpochRootHash hexutil.Bytes `json:"noticesEpochRootHash"`
	// Hash of the machine state claimed for the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	MachineStateHash hexutil.Bytes `json:"machineStateHash"`
	// Proof that this output hash is in the output-hashes merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	// Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'.
	OutputHashInOutputHashesSiblings []hexutil.Bytes `json:"outputHashInOutputHashesSiblings"`
	// Proof that this output-hashes root hash is in epoch's output merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	// Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'.
	OutputHashesInEpochSiblings []hexutil.Bytes `json:"outputHashesInEpochSiblings"`
}
