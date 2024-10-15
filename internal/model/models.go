// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const HashLength = common.HashLength

type (
	Hash                  = common.Hash
	Address               = common.Address
	Bytes                 = hexutil.Bytes
	InputCompletionStatus string
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
	InputStatusOutputsLimitExceeded       InputCompletionStatus = "OUTPUTS_LIMIT_EXCEEDED"
	InputStatusCycleLimitExceeded         InputCompletionStatus = "CYCLE_LIMIT_EXCEEDED"
	InputStatusTimeLimitExceeded          InputCompletionStatus = "TIME_LIMIT_EXCEEDED"
	InputStatusPayloadLengthLimitExceeded InputCompletionStatus = "PAYLOAD_LENGTH_LIMIT_EXCEEDED"
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
	EpochStatusOpen               EpochStatus = "OPEN"
	EpochStatusClosed             EpochStatus = "CLOSED"
	EpochStatusProcessedAllInputs EpochStatus = "PROCESSED_ALL_INPUTS"
	EpochStatusClaimComputed      EpochStatus = "CLAIM_COMPUTED"
	EpochStatusClaimSubmitted     EpochStatus = "CLAIM_SUBMITTED"
	EpochStatusClaimAccepted      EpochStatus = "CLAIM_ACCEPTED"
	EpochStatusClaimRejected      EpochStatus = "CLAIM_REJECTED"
)

type NodePersistentConfig struct {
	DefaultBlock            DefaultBlock
	InputBoxDeploymentBlock uint64
	InputBoxAddress         Address
	ChainId                 uint64
}

type ExecutionParameters struct {
	AdvanceIncCycles      uint64
	AdvanceMaxCycles      uint64
	InspectIncCycles      uint64
	InspectMaxCycles      uint64
	AdvanceIncDeadline    time.Duration
	AdvanceMaxDeadline    time.Duration
	InspectIncDeadline    time.Duration
	InspectMaxDeadline    time.Duration
	LoadDeadline          time.Duration
	StoreDeadline         time.Duration
	FastDeadline          time.Duration
	MaxConcurrentInspects int64
}

type MachineConfig struct {
	AppAddress         Address
	SnapshotPath       string
	SnapshotInputIndex *uint64
	ExecutionParameters
}

type Application struct {
	Id                   uint64
	ContractAddress      Address
	TemplateHash         Hash
	TemplateUri          string
	LastProcessedBlock   uint64
	LastClaimCheckBlock  uint64
	LastOutputCheckBlock uint64
	Status               ApplicationStatus
	IConsensusAddress    Address
}

type Epoch struct {
	Id              uint64
	Index           uint64
	FirstBlock      uint64
	LastBlock       uint64
	ClaimHash       *Hash
	TransactionHash *Hash
	Status          EpochStatus
	AppAddress      Address
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
	TransactionHash      *Hash
}

type Report struct {
	Id      uint64
	Index   uint64
	RawData Bytes
	InputId uint64
}

type Snapshot struct {
	Id         uint64
	URI        string
	InputId    uint64
	AppAddress Address
}
