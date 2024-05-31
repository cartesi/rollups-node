// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Hash = common.Hash
type Address = common.Address
type InputCompletionStatus string

const (
	InputStatusNone                       InputCompletionStatus = "NONE"
	InputStatusAccepted                   InputCompletionStatus = "ACCEPTED"
	InputStatusRejected                   InputCompletionStatus = "REJECTED"
	InputStatusException                  InputCompletionStatus = "EXCEPTION"
	InputStatusMachineHalted              InputCompletionStatus = "MACHINE_HALTED"
	InputStatusCycleLimitExceeded         InputCompletionStatus = "CYCLE_LIMIT_EXCEEDED"
	InputStatusTimeLimitExceeded          InputCompletionStatus = "TIME_LIMIT_EXCEEDED"
	InputStatusPayloadLengthLimitExceeded InputCompletionStatus = "PAYLOAD_LENGTH_LIMIT_EXCEEDED"
)

type Input struct {
	// Input index starting from genesis
	Index uint64
	// Status of the input
	CompletionStatus InputCompletionStatus
	// Input data as a blob, starting with '0x'
	Blob hexutil.Bytes
	// Block number with the input
	BlockNumber uint64
	// Hash of the machine state claimed for the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	MachineStateHash Hash
}

type Output struct {
	// Input whose processing produced the output
	InputIndex uint64
	// Output index within the context of the input that produced it
	Index uint64
	// Output data as a blob, starting with '0x'
	Blob hexutil.Bytes
}

type Epoch struct {
	// Block where the epoch started, inclusive
	StartBlock uint64
	// Block where the epoch finished, inclusive
	EndBlock uint64
}

// The inclusive range of inputs processed during the epoch
type InputRange struct {
	First uint64
	Last  uint64
}

type Claim struct {
	// Claim index starting from genesis
	Id uint64
	// Inputs that were processed in this claims
	InputRange InputRange
	// Epoch hash of the related epoch
	EpochHash Hash
	// Address of the application for this claim
	AppAddress Address
}

type Proof struct {
	// Input whose processing produced the output
	InputIndex uint64
	// The range of inputs processed during the epoch
	InputRange InputRange
	// Local input index within the context of the related epoch
	InputIndexWithinEpoch uint64
	// Output index within the context of the input that produced it
	OutputIndexWithinInput uint64
	// Merkle root of hashes of all outputs emitted by the related input
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	OutputHashesRootHash Hash
	// Merkle root of hashes of all outputs within the epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	OutputsEpochRootHash Hash
	// Hash of the machine state claimed for the related epoch
	// given in Ethereum hex binary format (32 bytes), starting with '0x'
	MachineStateHash Hash
	// Proof that this output hash is in the output-hashes merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	// Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'.
	OutputHashInOutputHashesSiblings []Hash
	// Proof that this output-hashes root hash is in epoch's output merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	// Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'.
	OutputHashesInEpochSiblings []Hash
}
