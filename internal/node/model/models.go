// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type (
	Hash                  = common.Hash
	Address               = common.Address
	InputCompletionStatus string
	ClaimStatus           string
	ApplicationStatus     string
	DefaultBlock          string
)

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

const (
	ClaimStatusPending   ClaimStatus = "PENDING"
	ClaimStatusSubmitted ClaimStatus = "SUBMITTED"
	ClaimStatusFinalized ClaimStatus = "FINALIZED"
)

const (
	ApplicationStatusRunning    ApplicationStatus = "RUNNING"
	ApplicationStatusNotRunning ApplicationStatus = "NOT RUNNING"
)

const (
	DefaultBlockStatusLatest    DefaultBlock = "LATEST"
	DefaultBlockStatusFinalized DefaultBlock = "FINALIZED"
	DefaultBlockStatusPending   DefaultBlock = "PENDING"
	DefaultBlockStatusSafe      DefaultBlock = "SAFE"
)

type NodePersistentConfig struct {
	DefaultBlock            DefaultBlock
	InputBoxDeploymentBlock uint64
	InputBoxAddress         Address
	ChainId                 uint64
	IConsensusAddress       Address
}

type Application struct {
	Id                 uint64
	ContractAddress    Address
	TemplateHash       Hash
	SnapshotURI        string
	LastProcessedBlock uint64
	EpochLength        uint64
	Status             ApplicationStatus
}

type Input struct {
	Id               uint64
	Index            uint64
	CompletionStatus InputCompletionStatus
	RawData          hexutil.Bytes
	BlockNumber      uint64
	MachineHash      *Hash
	OutputsHash      *Hash
	AppAddress       Address
}

type Output struct {
	Id                   uint64
	Index                uint64
	RawData              hexutil.Bytes
	Hash                 *Hash
	OutputHashesSiblings []Hash
	InputId              uint64
}

type Report struct {
	Id      uint64
	Index   uint64
	RawData hexutil.Bytes
	InputId uint64
}

type Claim struct {
	Id                   uint64
	Index                uint64
	Status               ClaimStatus
	OutputMerkleRootHash Hash
	TransactionHash      *Hash
	AppAddress           Address
}
