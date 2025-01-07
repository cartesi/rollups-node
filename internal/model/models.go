// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Application struct {
	ID                   int64 `sql:"primary_key"`
	Name                 string
	IApplicationAddress  common.Address
	IConsensusAddress    common.Address
	TemplateHash         common.Hash
	TemplateURI          string
	EpochLength          uint64
	State                ApplicationState
	Reason               *string
	LastProcessedBlock   uint64
	LastClaimCheckBlock  uint64
	LastOutputCheckBlock uint64
	ProcessedInputs      uint64
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ExecutionParameters  ExecutionParameters
}

type ApplicationState string

const (
	ApplicationState_Enabled    ApplicationState = "ENABLED"
	ApplicationState_Disabled   ApplicationState = "DISABLED"
	ApplicationState_Inoperable ApplicationState = "INOPERABLE"
)

var ApplicationStateAllValues = []ApplicationState{
	ApplicationState_Enabled,
	ApplicationState_Disabled,
	ApplicationState_Inoperable,
}

func (e *ApplicationState) Scan(value interface{}) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("Invalid scan value for ApplicationState enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "ENABLED":
		*e = ApplicationState_Enabled
	case "DISABLED":
		*e = ApplicationState_Disabled
	case "INOPERABLE":
		*e = ApplicationState_Inoperable
	default:
		return errors.New("Invalid scan value '" + enumValue + "' for ApplicationState enum")
	}

	return nil
}

func (e ApplicationState) String() string {
	return string(e)
}

type SnapshotPolicy string

const (
	SnapshotPolicy_None      SnapshotPolicy = "NONE"
	SnapshotPolicy_EachInput SnapshotPolicy = "EACH_INPUT"
	SnapshotPolicy_EachEpoch SnapshotPolicy = "EACH_EPOCH"
)

var SnapshotPolicyAllValues = []SnapshotPolicy{
	SnapshotPolicy_None,
	SnapshotPolicy_EachInput,
	SnapshotPolicy_EachEpoch,
}

func (e *SnapshotPolicy) Scan(value interface{}) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("Invalid scan value for SnapshotPolicy enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "NONE":
		*e = SnapshotPolicy_None
	case "EACH_INPUT":
		*e = SnapshotPolicy_EachInput
	case "EACH_EPOCH":
		*e = SnapshotPolicy_EachEpoch
	default:
		return errors.New("Invalid scan value '" + enumValue + "' for SnapshotPolicy enum")
	}

	return nil
}

func (e SnapshotPolicy) String() string {
	return string(e)
}

type ExecutionParameters struct {
	ApplicationID         int64 `sql:"primary_key"`
	SnapshotPolicy        SnapshotPolicy
	SnapshotRetention     uint64
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
	MaxConcurrentInspects uint32
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Epoch struct {
	ApplicationID        int64  `sql:"primary_key"`
	Index                uint64 `sql:"primary_key"`
	FirstBlock           uint64
	LastBlock            uint64
	ClaimHash            *common.Hash
	ClaimTransactionHash *common.Hash
	Status               EpochStatus
	VirtualIndex         uint64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type EpochStatus string

const (
	EpochStatus_Open            EpochStatus = "OPEN"
	EpochStatus_Closed          EpochStatus = "CLOSED"
	EpochStatus_InputsProcessed EpochStatus = "INPUTS_PROCESSED"
	EpochStatus_ClaimComputed   EpochStatus = "CLAIM_COMPUTED"
	EpochStatus_ClaimSubmitted  EpochStatus = "CLAIM_SUBMITTED"
	EpochStatus_ClaimAccepted   EpochStatus = "CLAIM_ACCEPTED"
	EpochStatus_ClaimRejected   EpochStatus = "CLAIM_REJECTED"
)

var EpochStatusAllValues = []EpochStatus{
	EpochStatus_Open,
	EpochStatus_Closed,
	EpochStatus_InputsProcessed,
	EpochStatus_ClaimComputed,
	EpochStatus_ClaimSubmitted,
	EpochStatus_ClaimAccepted,
	EpochStatus_ClaimRejected,
}

func (e *EpochStatus) Scan(value interface{}) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("Invalid scan value for EpochStatus enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "OPEN":
		*e = EpochStatus_Open
	case "CLOSED":
		*e = EpochStatus_Closed
	case "INPUTS_PROCESSED":
		*e = EpochStatus_InputsProcessed
	case "CLAIM_COMPUTED":
		*e = EpochStatus_ClaimComputed
	case "CLAIM_SUBMITTED":
		*e = EpochStatus_ClaimSubmitted
	case "CLAIM_ACCEPTED":
		*e = EpochStatus_ClaimAccepted
	case "CLAIM_REJECTED":
		*e = EpochStatus_ClaimRejected
	default:
		return errors.New("Invalid scan value '" + enumValue + "' for EpochStatus enum")
	}

	return nil
}

func (e EpochStatus) String() string {
	return string(e)
}

type Input struct {
	EpochApplicationID   int64 `sql:"primary_key"`
	EpochIndex           uint64
	Index                uint64 `sql:"primary_key"`
	BlockNumber          uint64
	RawData              []byte
	Status               InputCompletionStatus
	MachineHash          *common.Hash
	OutputsHash          *common.Hash
	TransactionReference common.Hash
	SnapshotURI          *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type InputCompletionStatus string

const (
	InputCompletionStatus_None                       InputCompletionStatus = "NONE"
	InputCompletionStatus_Accepted                   InputCompletionStatus = "ACCEPTED"
	InputCompletionStatus_Rejected                   InputCompletionStatus = "REJECTED"
	InputCompletionStatus_Exception                  InputCompletionStatus = "EXCEPTION"
	InputCompletionStatus_MachineHalted              InputCompletionStatus = "MACHINE_HALTED"
	InputCompletionStatus_OutputsLimitExceeded       InputCompletionStatus = "OUTPUTS_LIMIT_EXCEEDED"
	InputCompletionStatus_CycleLimitExceeded         InputCompletionStatus = "CYCLE_LIMIT_EXCEEDED"
	InputCompletionStatus_TimeLimitExceeded          InputCompletionStatus = "TIME_LIMIT_EXCEEDED"
	InputCompletionStatus_PayloadLengthLimitExceeded InputCompletionStatus = "PAYLOAD_LENGTH_LIMIT_EXCEEDED"
)

var InputCompletionStatusAllValues = []InputCompletionStatus{
	InputCompletionStatus_None,
	InputCompletionStatus_Accepted,
	InputCompletionStatus_Rejected,
	InputCompletionStatus_Exception,
	InputCompletionStatus_MachineHalted,
	InputCompletionStatus_CycleLimitExceeded,
	InputCompletionStatus_TimeLimitExceeded,
	InputCompletionStatus_PayloadLengthLimitExceeded,
}

func (e *InputCompletionStatus) Scan(value interface{}) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("Invalid scan value for InputCompletionStatus enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "NONE":
		*e = InputCompletionStatus_None
	case "ACCEPTED":
		*e = InputCompletionStatus_Accepted
	case "REJECTED":
		*e = InputCompletionStatus_Rejected
	case "EXCEPTION":
		*e = InputCompletionStatus_Exception
	case "MACHINE_HALTED":
		*e = InputCompletionStatus_MachineHalted
	case "CYCLE_LIMIT_EXCEEDED":
		*e = InputCompletionStatus_CycleLimitExceeded
	case "TIME_LIMIT_EXCEEDED":
		*e = InputCompletionStatus_TimeLimitExceeded
	case "PAYLOAD_LENGTH_LIMIT_EXCEEDED":
		*e = InputCompletionStatus_PayloadLengthLimitExceeded
	default:
		return errors.New("Invalid scan value '" + enumValue + "' for InputCompletionStatus enum")
	}

	return nil
}

func (e InputCompletionStatus) String() string {
	return string(e)
}

type Output struct {
	InputEpochApplicationID  int64 `sql:"primary_key"`
	InputIndex               uint64
	Index                    uint64 `sql:"primary_key"`
	RawData                  []byte
	Hash                     *common.Hash
	OutputHashesSiblings     []common.Hash
	ExecutionTransactionHash *common.Hash
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type Report struct {
	InputEpochApplicationID int64 `sql:"primary_key"`
	InputIndex              uint64
	Index                   uint64 `sql:"primary_key"`
	RawData                 []byte
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

const (
	BaseConfigKey string = "BASE_NODE"
)

type NodeConfig[T any] struct {
	Key       string
	Value     T
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NodeConfigValue struct {
	DefaultBlock            DefaultBlock
	InputReaderEnabled      bool
	ClaimSubmissionEnabled  bool
	InputBoxDeploymentBlock uint64
	InputBoxAddress         string
	ChainID                 uint64
}

type AdvanceResult struct {
	InputIndex  uint64
	Status      InputCompletionStatus
	Outputs     [][]byte
	Reports     [][]byte
	OutputsHash common.Hash
	MachineHash *common.Hash
}

type InspectResult struct {
	ProcessedInputs uint64
	Accepted        bool
	Reports         [][]byte
	Error           error
}

// FIXME: remove this type. Migrate claim to use Application + Epoch
type ClaimRow struct {
	Epoch
	IApplicationAddress common.Address
	IConsensusAddress   common.Address
}

type DefaultBlock string

const (
	DefaultBlock_Finalized DefaultBlock = "FINALIZED"
	DefaultBlock_Latest    DefaultBlock = "LATEST"
	DefaultBlock_Pending   DefaultBlock = "PENDING"
	DefaultBlock_Safe      DefaultBlock = "SAFE"
)

var DefaultBlockAllValues = []DefaultBlock{
	DefaultBlock_Finalized,
	DefaultBlock_Latest,
	DefaultBlock_Pending,
	DefaultBlock_Safe,
}

func (e *DefaultBlock) Scan(value interface{}) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("Invalid scan value for DefaultBlock enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "FINALIZED":
		*e = DefaultBlock_Finalized
	case "LATEST":
		*e = DefaultBlock_Latest
	case "PENDING":
		*e = DefaultBlock_Pending
	case "SAFE":
		*e = DefaultBlock_Safe
	default:
		return errors.New("Invalid scan value '" + enumValue + "' for DefaultBlock enum")
	}

	return nil
}

func (e DefaultBlock) String() string {
	return string(e)
}

func Pointer[T any](v T) *T {
	return &v
}
