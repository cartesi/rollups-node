// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package contains the the definition of the structs shared between the node components.
package model

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Integer type for identifers and indices.
type Index int32

// Rollups session that runs in the node.
//
// The node can have multiple sessions to either read from or validate a DApp.
type Session struct {

	// Session identifier, which is generated when it is created.
	ID Index

	// DApp associated with the session.
	DAppID

	// Kind of the session; see the enum for more details.
	SessionKind

	// Epoch duration in number of blocks.
	EpochDuration uint64

	// Last block retrieved from the blockchain.
	LastBlock Index
}

// DApp Identifier.
type DAppID struct {

	// Ethereum Chain ID.
	ChainID Index

	// DApp Address.
	Address common.Address
}

// Enum for the kind of the session.
type SessionKind string

const (
	// In validation sessions, the node generates the proofs and sends the corresponding claim
	// to the blockchain.
	SessionKindValidator SessionKind = "Validator"

	// In reader sessions, the node reads the claims from the blockchain and generates proofs.
	SessionKindReader SessionKind = "Reader"
)

// Rollups advance-state inputs.
type Input struct {

	// Session associated with the input.
	SessionID Index

	// Index of the input for the given DApp.
	// Inputs start from 0 and have no gaps.
	InputIndex Index

	// Input payload.
	// From the node point-of-view, the input can be anything.
	Payload []byte

	// Address of the message sender.
	Sender common.Address

	// Block number when the input was added the blockchain.
	BlockNumber uint64

	// Timestamp of the block when the input was added to the blockchain.
	Timestamp time.Time

	// Completion status of the input.
	CompletionStatus

	// Cartesi machine hash after processing the input.
	MachineHash common.Hash
}

// Completion status of an input.
type CompletionStatus string

const (
	CompletionStatusUnprocessed                CompletionStatus = "Unprocessed"
	CompletionStatusAccepted                   CompletionStatus = "Accepted"
	CompletionStatusRejected                   CompletionStatus = "Rejected"
	CompletionStatusException                  CompletionStatus = "Exception"
	CompletionStatusMachineHalted              CompletionStatus = "MachineHalted"
	CompletionStatusCycleLimitExceeded         CompletionStatus = "CycleLimitExceeded"
	CompletionStatusTimeLimitExceeded          CompletionStatus = "TimeLimitExceeded"
	CompletionStatusPayloadLengthLimitExceeded CompletionStatus = "PayloadLengthLimitExceeded"
)

// Rollups output.
//
// The Cartesi machine produces outputs after processing an input.
// If the machine does not accept the input, all outputs are rejected.
// Outputs have proofs which are used to validate them on the blockchain.
type Output struct {

	// Session associated with the output.
	SessionID Index

	// Input associated with the output.
	InputIndex Index

	// Local index of the output for the given input.
	OutputIndex Index

	// Output payload.
	// From the point-of-view of the model, the payload can be anything.
	Payload []byte

	// Output kind.
	Kind OutputKind
}

// Kind of the output.
type OutputKind string

const (
	OutputKindVoucher OutputKind = "Voucher"
	OutputKindNotice  OutputKind = "Notice"
)

// Rollups report.
//
// The Cartesi machine produces reports after processing an input.
// Reports are saved regardless of the completion status.
// Different from outputs, reports do not have proofs and cannot be validated on-chain.
type Report struct {

	// Session associated with the report.
	SessionID Index

	// Input associated with the report.
	InputIndex Index

	// Local index of the report for the given input.
	ReportIndex Index

	// Report payload.
	// From the point-of-view of the model, the payload can be anything.
	Payload []byte
}

// Rollups claim.
//
// The validator rollups node computes a claim hash that represents the state of the rollups in a
// given time. Then, it sends this claim to the blockchain so outputs can be validated.
// Alongside the claim, the node also produces the proofs required by the on-chain code to validate
// the outputs.
//
// The node should query the blockchain for claim generated claims.
// In a reading session, the node will generate the proofs for the existing claims.
// In a validator session, the node will consult the existing claims to compute the next one.
type Claim struct {

	// Session associated with the claim.
	SessionID Index

	// First input index associated with the claim.
	FirstInputIndex Index

	// Last input index associated with the claim.
	LastInputIndex Index

	// Hash of the claim.
	Hash common.Hash
}

// Rollups proof for outputs.
//
// Proofs can be used to execute vouchers or validate notices on-chain.
type Proof struct {

	// Session associated with the output's proof.
	SessionID Index

	// Input associated with the output's proof.
	InputIndex Index

	// Local index of the output for the given input.
	OutputIndex Index

	// Local input index within the context of the related epoch
	InputIndexWithinEpoch Index

	// Output index within the context of the input that produced it
	OutputIndexWithinInput Index

	// Merkle root of all output hashes of the related input
	OutputHashesRootHash common.Hash

	// Merkle root of all voucher hashes of the related epoch
	VouchersEpochRootHash common.Hash

	// Merkle root of all notice hashes of the related epoch
	NoticesEpochRootHash common.Hash

	// Hash of the machine state claimed for the related epoch
	MachineStateHash common.Hash

	// Proof that this output hash is in the output-hashes merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	OutputHashInOutputHashesSiblings []common.Hash

	// Proof that this output-hashes root hash is in epoch's output merkle tree.
	// This array of siblings is bottom-up ordered (from the leaf to the root).
	OutputHashesInEpochSiblings []common.Hash
}
