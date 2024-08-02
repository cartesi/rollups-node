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
	Bytes                 = hexutil.Bytes
	InputCompletionStatus string
	ClaimStatus           string
	ApplicationStatus     string
	DefaultBlock          string
	EpochStatus           string
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

const (
	EpochStatusReceivingInputs    EpochStatus = "RECEIVING_INPUTS"
	EpochStatusReceivedLastInput  EpochStatus = "RECEIVED_LAST_INPUT"
	EpochStatusProcessedAllInputs EpochStatus = "PROCESSED_ALL_INPUTS"
	EpochStatusCalculatedClaim    EpochStatus = "CALCULATED_CLAIM"
	EpochStatusSubmittedClaim     EpochStatus = "SUBMITTED_CLAIM"
	EpochStatusAcceptedClaim      EpochStatus = "ACCEPTED_CLAIM"
	EpochStatusRejectedClaim      EpochStatus = "REJECTED_CLAIM"
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
	LastProcessedBlock uint64
	EpochLength        uint64
	Status             ApplicationStatus
}

type Epoch struct {
	Id              uint64
	AppAddress      Address
	Index           uint64
	FirstBlock      uint64
	LastBlock       uint64
	ClaimHash       *Hash
	TransactionHash *Hash
	Status          EpochStatus
}

type Input struct {
	Id               uint64
	Index            uint64
	CompletionStatus InputCompletionStatus
	RawData          Bytes
	BlockNumber      uint64
	MachineHash      *Hash
	OutputsHash      *Hash
	AppAddress       Address
	EpochId          uint64
}

type Output struct {
	Id                   uint64
	Index                uint64
	RawData              Bytes
	Hash                 *Hash
	OutputHashesSiblings []Hash
	InputId              uint64
}

type Report struct {
	Id      uint64
	Index   uint64
	RawData Bytes
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

type Snapshot struct {
	Id         uint64
	InputId    uint64
	AppAddress Address
	URI        string
}
