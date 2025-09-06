// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Application struct {
	ID                   int64               `sql:"primary_key" json:"-"`
	Name                 string              `json:"name"`
	IApplicationAddress  common.Address      `json:"iapplication_address"`
	IConsensusAddress    common.Address      `json:"iconsensus_address"`
	IInputBoxAddress     common.Address      `json:"iinputbox_address"`
	TemplateHash         common.Hash         `json:"template_hash"`
	TemplateURI          string              `json:"-"`
	EpochLength          uint64              `json:"epoch_length"`
	DataAvailability     []byte              `json:"data_availability"`
	ConsensusType        Consensus           `json:"consensus_type"`
	State                ApplicationState    `json:"state"`
	Reason               *string             `json:"reason"`
	IInputBoxBlock       uint64              `json:"iinputbox_block"`
	LastEpochCheckBlock  uint64              `json:"last_epoch_check_block"`
	LastInputCheckBlock  uint64              `json:"last_input_check_block"`
	LastOutputCheckBlock uint64              `json:"last_output_check_block"`
	ProcessedInputs      uint64              `json:"processed_inputs"`
	CreatedAt            time.Time           `json:"created_at"`
	UpdatedAt            time.Time           `json:"updated_at"`
	ExecutionParameters  ExecutionParameters `json:"execution_parameters"`
}

// HasDataAvailabilitySelector checks if the application's DataAvailability
// starts with the given DataAvailabilitySelector
func (a *Application) HasDataAvailabilitySelector(selector DataAvailabilitySelector) bool {
	return selector.MatchesBytes(a.DataAvailability)
}

func (a *Application) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Application
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		*Alias
		DataAvailability     string `json:"data_availability"`
		IInputBoxBlock       string `json:"iinputbox_block"`
		LastEpochCheckBlock  string `json:"last_epoch_check_block"`
		LastInputCheckBlock  string `json:"last_input_check_block"`
		LastOutputCheckBlock string `json:"last_output_check_block"`
		EpochLength          string `json:"epoch_length"`
		ProcessedInputs      string `json:"processed_inputs"`
	}{
		Alias:                (*Alias)(a),
		DataAvailability:     "0x" + hex.EncodeToString(a.DataAvailability),
		IInputBoxBlock:       fmt.Sprintf("0x%x", a.IInputBoxBlock),
		LastEpochCheckBlock:  fmt.Sprintf("0x%x", a.LastEpochCheckBlock),
		LastInputCheckBlock:  fmt.Sprintf("0x%x", a.LastInputCheckBlock),
		LastOutputCheckBlock: fmt.Sprintf("0x%x", a.LastOutputCheckBlock),
		EpochLength:          fmt.Sprintf("0x%x", a.EpochLength),
		ProcessedInputs:      fmt.Sprintf("0x%x", a.ProcessedInputs),
	}
	return json.Marshal(aux)
}

func (a *Application) UnmarshalJSON(in []byte) error {
	type Alias Application
	aux := &struct {
		*Alias

		DataAvailability     string `json:"data_availability"`
		IInputBoxBlock       string `json:"iinputbox_block"`
		LastInputCheckBlock  string `json:"last_input_check_block"`
		LastOutputCheckBlock string `json:"last_output_check_block"`
		LastEpochCheckBlock  string `json:"last_epoch_check_block"`
		EpochLength          string `json:"epoch_length"`
		ProcessedInputs      string `json:"processed_inputs"`
	}{}

	var err error

	if err = json.Unmarshal(in, aux); err != nil {
		return err
	}

	*a = Application(*aux.Alias)

	// manually decode the following values as hex instead of the default (base64)
	a.DataAvailability, err = hexutil.Decode(aux.DataAvailability)
	if err != nil {
		return err
	}

	a.IInputBoxBlock, err = ParseHexUint64(aux.IInputBoxBlock)
	if err != nil {
		return err
	}

	a.LastInputCheckBlock, err = ParseHexUint64(aux.LastInputCheckBlock)
	if err != nil {
		return err
	}

	a.LastOutputCheckBlock, err = ParseHexUint64(aux.LastOutputCheckBlock)
	if err != nil {
		return err
	}

	a.LastEpochCheckBlock, err = ParseHexUint64(aux.LastEpochCheckBlock)
	if err != nil {
		return err
	}

	a.EpochLength, err = ParseHexUint64(aux.EpochLength)
	if err != nil {
		return err
	}

	a.ProcessedInputs, err = ParseHexUint64(aux.ProcessedInputs)
	if err != nil {
		return err
	}

	return nil
}

func (a *Application) IsDaveConsensus() bool {
	return a.ConsensusType == Consensus_PRT
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

func (e *ApplicationState) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for ApplicationState enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "ENABLED":
		*e = ApplicationState_Enabled
	case "DISABLED":
		*e = ApplicationState_Disabled
	case "INOPERABLE":
		*e = ApplicationState_Inoperable
	default:
		return errors.New("invalid value '" + enumValue + "' for ApplicationState enum")
	}

	return nil
}

func (e ApplicationState) String() string {
	return string(e)
}

type Consensus string

const (
	Consensus_Authority Consensus = "AUTHORITY"
	Consensus_Quorum    Consensus = "QUORUM"
	Consensus_PRT       Consensus = "PRT"
)

var ConsensusAllValues = []Consensus{
	Consensus_Authority,
	Consensus_Quorum,
	Consensus_PRT,
}

func (e *Consensus) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for ConsensusType enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "AUTHORITY":
		*e = Consensus_Authority
	case "QUORUM":
		*e = Consensus_Quorum
	case "PRT":
		*e = Consensus_PRT
	default:
		return errors.New("invalid value '" + enumValue + "' for Consensus enum")
	}

	return nil
}

func (e Consensus) String() string {
	return string(e)
}

const DATA_AVAILABILITY_SELECTOR_SIZE = 4

type DataAvailabilitySelector [DATA_AVAILABILITY_SELECTOR_SIZE]byte

// Known data availability selectors
var (
	// ABI encoded "InputBox(address)"
	DataAvailability_InputBox = DataAvailabilitySelector{0xb1, 0x2c, 0x9e, 0xde}
)

func (d *DataAvailabilitySelector) MarshalJSON() ([]byte, error) {
	return json.Marshal("0x" + hex.EncodeToString(d[:]))
}

// MatchesBytes checks if this selector matches the first bytes of the given byte slice
func (d DataAvailabilitySelector) MatchesBytes(data []byte) bool {
	if len(data) < DATA_AVAILABILITY_SELECTOR_SIZE {
		return false
	}
	for i := range DATA_AVAILABILITY_SELECTOR_SIZE {
		if data[i] != d[i] {
			return false
		}
	}
	return true
}

func (d *DataAvailabilitySelector) Scan(value any) error {
	var selector []byte
	switch v := value.(type) {
	case []byte:
		selector = v
	default:
		return errors.New("invalid scan value for DataAvailabilitySelector. Value has to be of type []byte")
	}

	if len(selector) != DATA_AVAILABILITY_SELECTOR_SIZE {
		return errors.New("invalid value for DataAvailabilitySelector")
	}
	copy(d[:], selector[:DATA_AVAILABILITY_SELECTOR_SIZE])

	return nil
}

type SnapshotPolicy string

const (
	SnapshotPolicy_None       SnapshotPolicy = "NONE"
	SnapshotPolicy_EveryInput SnapshotPolicy = "EVERY_INPUT"
	SnapshotPolicy_EveryEpoch SnapshotPolicy = "EVERY_EPOCH"
)

var SnapshotPolicyAllValues = []SnapshotPolicy{
	SnapshotPolicy_None,
	SnapshotPolicy_EveryInput,
	SnapshotPolicy_EveryEpoch,
}

func (e *SnapshotPolicy) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid scan value for SnapshotPolicy enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "NONE":
		*e = SnapshotPolicy_None
	case "EVERY_INPUT":
		*e = SnapshotPolicy_EveryInput
	case "EVERY_EPOCH":
		*e = SnapshotPolicy_EveryEpoch
	default:
		return errors.New("invalid scan value '" + enumValue + "' for SnapshotPolicy enum")
	}

	return nil
}

func (e SnapshotPolicy) String() string {
	return string(e)
}

type ExecutionParameters struct {
	ApplicationID         int64          `sql:"primary_key" json:"-"`
	SnapshotPolicy        SnapshotPolicy `json:"snapshot_policy"`
	AdvanceIncCycles      uint64         `json:"advance_inc_cycles"`
	AdvanceMaxCycles      uint64         `json:"advance_max_cycles"`
	InspectIncCycles      uint64         `json:"inspect_inc_cycles"`
	InspectMaxCycles      uint64         `json:"inspect_max_cycles"`
	AdvanceIncDeadline    time.Duration  `json:"advance_inc_deadline"`
	AdvanceMaxDeadline    time.Duration  `json:"advance_max_deadline"`
	InspectIncDeadline    time.Duration  `json:"inspect_inc_deadline"`
	InspectMaxDeadline    time.Duration  `json:"inspect_max_deadline"`
	LoadDeadline          time.Duration  `json:"load_deadline"`
	StoreDeadline         time.Duration  `json:"store_deadline"`
	FastDeadline          time.Duration  `json:"fast_deadline"`
	MaxConcurrentInspects uint32         `json:"max_concurrent_inspects"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

func (e *ExecutionParameters) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias ExecutionParameters
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		AdvanceIncCycles   string `json:"advance_inc_cycles"`
		AdvanceMaxCycles   string `json:"advance_max_cycles"`
		InspectIncCycles   string `json:"inspect_inc_cycles"`
		InspectMaxCycles   string `json:"inspect_max_cycles"`
		AdvanceIncDeadline string `json:"advance_inc_deadline"`
		AdvanceMaxDeadline string `json:"advance_max_deadline"`
		InspectIncDeadline string `json:"inspect_inc_deadline"`
		InspectMaxDeadline string `json:"inspect_max_deadline"`
		LoadDeadline       string `json:"load_deadline"`
		StoreDeadline      string `json:"store_deadline"`
		FastDeadline       string `json:"fast_deadline"`
		*Alias
	}{
		AdvanceIncCycles:   fmt.Sprintf("0x%x", e.AdvanceIncCycles),
		AdvanceMaxCycles:   fmt.Sprintf("0x%x", e.AdvanceMaxCycles),
		InspectIncCycles:   fmt.Sprintf("0x%x", e.InspectIncCycles),
		InspectMaxCycles:   fmt.Sprintf("0x%x", e.InspectMaxCycles),
		AdvanceIncDeadline: fmt.Sprintf("0x%x", uint64(e.AdvanceIncDeadline)),
		AdvanceMaxDeadline: fmt.Sprintf("0x%x", uint64(e.AdvanceMaxDeadline)),
		InspectIncDeadline: fmt.Sprintf("0x%x", uint64(e.InspectIncDeadline)),
		InspectMaxDeadline: fmt.Sprintf("0x%x", uint64(e.InspectMaxDeadline)),
		LoadDeadline:       fmt.Sprintf("0x%x", uint64(e.LoadDeadline)),
		StoreDeadline:      fmt.Sprintf("0x%x", uint64(e.StoreDeadline)),
		FastDeadline:       fmt.Sprintf("0x%x", uint64(e.FastDeadline)),
		Alias:              (*Alias)(e),
	}
	return json.Marshal(aux)
}

func (e *ExecutionParameters) UnmarshalJSON(data []byte) error {
	// Create an alias to avoid infinite recursion in UnmarshalJSON.
	type Alias ExecutionParameters
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		AdvanceIncCycles   string `json:"advance_inc_cycles"`
		AdvanceMaxCycles   string `json:"advance_max_cycles"`
		InspectIncCycles   string `json:"inspect_inc_cycles"`
		InspectMaxCycles   string `json:"inspect_max_cycles"`
		AdvanceIncDeadline string `json:"advance_inc_deadline"`
		AdvanceMaxDeadline string `json:"advance_max_deadline"`
		InspectIncDeadline string `json:"inspect_inc_deadline"`
		InspectMaxDeadline string `json:"inspect_max_deadline"`
		LoadDeadline       string `json:"load_deadline"`
		StoreDeadline      string `json:"store_deadline"`
		FastDeadline       string `json:"fast_deadline"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.AdvanceIncCycles != "" {
		val, err := ParseHexUint64(aux.AdvanceIncCycles)
		if err != nil {
			return fmt.Errorf("invalid advance_inc_cycles: %w", err)
		}
		e.AdvanceIncCycles = val
	}

	if aux.AdvanceMaxCycles != "" {
		val, err := ParseHexUint64(aux.AdvanceMaxCycles)
		if err != nil {
			return fmt.Errorf("invalid advance_max_cycles: %w", err)
		}
		e.AdvanceMaxCycles = val
	}

	if aux.InspectIncCycles != "" {
		val, err := ParseHexUint64(aux.InspectIncCycles)
		if err != nil {
			return fmt.Errorf("invalid inspect_inc_cycles: %w", err)
		}
		e.InspectIncCycles = val
	}

	if aux.InspectMaxCycles != "" {
		val, err := ParseHexUint64(aux.InspectMaxCycles)
		if err != nil {
			return fmt.Errorf("invalid inspect_max_cycles: %w", err)
		}
		e.InspectMaxCycles = val
	}

	if aux.AdvanceIncDeadline != "" {
		val, err := ParseHexDuration(aux.AdvanceIncDeadline)
		if err != nil {
			return fmt.Errorf("invalid advance_inc_deadline: %w", err)
		}
		e.AdvanceIncDeadline = val
	}

	if aux.AdvanceMaxDeadline != "" {
		val, err := ParseHexDuration(aux.AdvanceMaxDeadline)
		if err != nil {
			return fmt.Errorf("invalid advance_max_deadline: %w", err)
		}
		e.AdvanceMaxDeadline = val
	}

	if aux.InspectIncDeadline != "" {
		val, err := ParseHexDuration(aux.InspectIncDeadline)
		if err != nil {
			return fmt.Errorf("invalid inspect_inc_deadline: %w", err)
		}
		e.InspectIncDeadline = val
	}

	if aux.InspectMaxDeadline != "" {
		val, err := ParseHexDuration(aux.InspectMaxDeadline)
		if err != nil {
			return fmt.Errorf("invalid inspect_max_deadline: %w", err)
		}
		e.InspectMaxDeadline = val
	}

	if aux.LoadDeadline != "" {
		val, err := ParseHexDuration(aux.LoadDeadline)
		if err != nil {
			return fmt.Errorf("invalid load_deadline: %w", err)
		}
		e.LoadDeadline = val
	}

	if aux.StoreDeadline != "" {
		val, err := ParseHexDuration(aux.StoreDeadline)
		if err != nil {
			return fmt.Errorf("invalid store_deadline: %w", err)
		}
		e.StoreDeadline = val
	}

	if aux.FastDeadline != "" {
		val, err := ParseHexDuration(aux.FastDeadline)
		if err != nil {
			return fmt.Errorf("invalid fast_deadline: %w", err)
		}
		e.FastDeadline = val
	}

	return nil
}

// validateParameters constants
const maxDuration = 24 * time.Hour
const maxConcurrentInspects = 1000

// validateParameters performs validation on the loaded parameters
func (e *ExecutionParameters) Validate() error {
	// Validate durations are reasonable
	if e.AdvanceIncDeadline < 0 || e.AdvanceIncDeadline > maxDuration {
		return fmt.Errorf("advance_inc_deadline must be between 0 and 24h")
	}

	if e.AdvanceMaxDeadline < 0 || e.AdvanceMaxDeadline > maxDuration {
		return fmt.Errorf("advance_max_deadline must be between 0 and 24h")
	}

	if e.InspectIncDeadline < 0 || e.InspectIncDeadline > maxDuration {
		return fmt.Errorf("inspect_inc_deadline must be between 0 and 24h")
	}

	if e.InspectMaxDeadline < 0 || e.InspectMaxDeadline > maxDuration {
		return fmt.Errorf("inspect_max_deadline must be between 0 and 24h")
	}

	if e.LoadDeadline < 0 || e.LoadDeadline > maxDuration {
		return fmt.Errorf("load_deadline must be between 0 and 24h")
	}

	if e.StoreDeadline < 0 || e.StoreDeadline > maxDuration {
		return fmt.Errorf("store_deadline must be between 0 and 24h")
	}

	if e.FastDeadline < 0 || e.FastDeadline > maxDuration {
		return fmt.Errorf("fast_deadline must be between 0 and 24h")
	}

	// Validate max_concurrent_inspects
	if e.MaxConcurrentInspects > maxConcurrentInspects {
		return fmt.Errorf("max_concurrent_inspects must be between 0 and 1000")
	}

	// Validate snapshot policy
	validPolicy := false
	switch e.SnapshotPolicy {
	case SnapshotPolicy_None, SnapshotPolicy_EveryInput, SnapshotPolicy_EveryEpoch:
		validPolicy = true
	}

	if !validPolicy {
		return fmt.Errorf("invalid snapshot policy: %s. Valid values are: NONE, EVERY_INPUT, EVERY_EPOCH", e.SnapshotPolicy)
	}

	return nil
}

func ParseHexUint64(s string) (uint64, error) {
	if s == "" || len(s) < 3 || (!strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X")) {
		return 0, fmt.Errorf("invalid hex string: %s", s)
	}
	return strconv.ParseUint(s[2:], 16, 64)
}

func ParseHexDuration(s string) (time.Duration, error) {
	ns, err := ParseHexUint64(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(ns), nil
}

type Epoch struct {
	ApplicationID        int64           `sql:"primary_key" json:"-"`
	Index                uint64          `sql:"primary_key" json:"index"`
	FirstBlock           uint64          `json:"first_block"`
	LastBlock            uint64          `json:"last_block"`
	InputIndexLowerBound uint64          `json:"input_index_lower_bound"`
	InputIndexUpperBound uint64          `json:"input_index_upper_bound"`
	MachineHash          *common.Hash    `json:"machine_hash"`
	ClaimHash            *common.Hash    `json:"claim_hash"`
	ClaimTransactionHash *common.Hash    `json:"claim_transaction_hash"`
	TournamentAddress    *common.Address `json:"tournament_address"`
	Status               EpochStatus     `json:"status"`
	VirtualIndex         uint64          `json:"virtual_index"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (e *Epoch) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Epoch
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		Index                string `json:"index"`
		FirstBlock           string `json:"first_block"`
		LastBlock            string `json:"last_block"`
		InputIndexLowerBound string `json:"input_index_lower_bound"`
		InputIndexUpperBound string `json:"input_index_upper_bound"`
		VirtualIndex         string `json:"virtual_index"`
		*Alias
	}{
		Index:                fmt.Sprintf("0x%x", e.Index),
		FirstBlock:           fmt.Sprintf("0x%x", e.FirstBlock),
		LastBlock:            fmt.Sprintf("0x%x", e.LastBlock),
		InputIndexLowerBound: fmt.Sprintf("0x%x", e.InputIndexLowerBound),
		InputIndexUpperBound: fmt.Sprintf("0x%x", e.InputIndexUpperBound),
		VirtualIndex:         fmt.Sprintf("0x%x", e.VirtualIndex),
		Alias:                (*Alias)(e),
	}
	return json.Marshal(aux)
}

func (e *Epoch) UnmarshalJSON(in []byte) error {
	type Alias Epoch
	aux := &struct {
		*Alias

		Index                string `json:"index"`
		FirstBlock           string `json:"first_block"`
		LastBlock            string `json:"last_block"`
		InputIndexLowerBound string `json:"input_index_lower_bound"`
		InputIndexUpperBound string `json:"input_index_upper_bound"`
		VirtualIndex         string `json:"virtual_index"`
	}{}

	var err error

	if err = json.Unmarshal(in, aux); err != nil {
		return err
	}

	*e = Epoch(*aux.Alias)

	// manually decode the following values as hex instead of the default (base64)
	e.Index, err = ParseHexUint64(aux.Index)
	if err != nil {
		return err
	}

	e.FirstBlock, err = ParseHexUint64(aux.FirstBlock)
	if err != nil {
		return err
	}

	e.LastBlock, err = ParseHexUint64(aux.LastBlock)
	if err != nil {
		return err
	}

	e.InputIndexLowerBound, err = ParseHexUint64(aux.InputIndexLowerBound)
	if err != nil {
		return err
	}

	e.InputIndexUpperBound, err = ParseHexUint64(aux.InputIndexUpperBound)
	if err != nil {
		return err
	}

	e.VirtualIndex, err = ParseHexUint64(aux.VirtualIndex)
	if err != nil {
		return err
	}

	return nil
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

func (e *EpochStatus) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for EpochStatus enum. Enum value has to be of type string or []byte")
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
		return errors.New("invalid value '" + enumValue + "' for EpochStatus enum")
	}

	return nil
}

func (e EpochStatus) String() string {
	return string(e)
}

type Input struct {
	EpochApplicationID   int64                 `sql:"primary_key" json:"-"`
	EpochIndex           uint64                `json:"epoch_index"`
	Index                uint64                `sql:"primary_key" json:"index"`
	BlockNumber          uint64                `json:"block_number"`
	RawData              []byte                `json:"raw_data"`
	Status               InputCompletionStatus `json:"status"`
	MachineHash          *common.Hash          `json:"machine_hash"`
	OutputsHash          *common.Hash          `json:"outputs_hash"`
	TransactionReference common.Hash           `json:"transaction_reference"`
	SnapshotURI          *string               `json:"-"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

func (i *Input) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Input
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		Index       string `json:"index"`
		BlockNumber string `json:"block_number"`
		RawData     string `json:"raw_data"`
		*Alias
	}{
		EpochIndex:  fmt.Sprintf("0x%x", i.EpochIndex),
		Index:       fmt.Sprintf("0x%x", i.Index),
		BlockNumber: fmt.Sprintf("0x%x", i.BlockNumber),
		RawData:     "0x" + hex.EncodeToString(i.RawData),
		Alias:       (*Alias)(i),
	}
	return json.Marshal(aux)
}

func (i *Input) UnmarshalJSON(in []byte) error {
	type Alias Input
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		Index       string `json:"index"`
		BlockNumber string `json:"block_number"`
		RawData     string `json:"raw_data"`
		*Alias
	}{}

	var err error
	if err = json.Unmarshal(in, aux); err != nil {
		return err
	}

	*i = Input(*aux.Alias)

	i.EpochIndex, err = ParseHexUint64(aux.EpochIndex)
	if err != nil {
		return fmt.Errorf("error on EpochIndex: %v", err)
	}

	i.Index, err = ParseHexUint64(aux.Index)
	if err != nil {
		return fmt.Errorf("error on Index: %v", err)
	}

	i.BlockNumber, err = ParseHexUint64(aux.BlockNumber)
	if err != nil {
		return fmt.Errorf("error on BlockNumber: %v", err)
	}

	i.RawData, err = hexutil.Decode(aux.RawData)
	if err != nil {
		return fmt.Errorf("error on RawData: %v", err)
	}

	return nil
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

func (e *InputCompletionStatus) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for InputCompletionStatus enum. Enum value has to be of type string or []byte")
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
		return errors.New("invalid value '" + enumValue + "' for InputCompletionStatus enum")
	}

	return nil
}

func (e InputCompletionStatus) String() string {
	return string(e)
}

type Output struct {
	InputEpochApplicationID  int64         `sql:"primary_key" json:"-"`
	EpochIndex               uint64        `json:"epoch_index"`
	InputIndex               uint64        `json:"input_index"`
	Index                    uint64        `sql:"primary_key" json:"index"`
	RawData                  []byte        `json:"raw_data"`
	Hash                     *common.Hash  `json:"hash"`
	OutputHashesSiblings     []common.Hash `json:"output_hashes_siblings"`
	ExecutionTransactionHash *common.Hash  `json:"execution_transaction_hash"`
	CreatedAt                time.Time     `json:"created_at"`
	UpdatedAt                time.Time     `json:"updated_at"`
}

func (i *Output) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Output
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex string `json:"epoch_index"`
		InputIndex string `json:"input_index"`
		Index      string `json:"index"`
		RawData    string `json:"raw_data"`
		*Alias
	}{
		EpochIndex: fmt.Sprintf("0x%x", i.EpochIndex),
		InputIndex: fmt.Sprintf("0x%x", i.InputIndex),
		Index:      fmt.Sprintf("0x%x", i.Index),
		RawData:    "0x" + hex.EncodeToString(i.RawData),
		Alias:      (*Alias)(i),
	}
	return json.Marshal(aux)
}

type Report struct {
	InputEpochApplicationID int64     `sql:"primary_key" json:"-"`
	EpochIndex              uint64    `json:"epoch_index"`
	InputIndex              uint64    `json:"input_index"`
	Index                   uint64    `sql:"primary_key" json:"index"`
	RawData                 []byte    `json:"raw_data"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (r *Report) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Report
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex string `json:"epoch_index"`
		InputIndex string `json:"input_index"`
		Index      string `json:"index"`
		RawData    string `json:"raw_data"`
		*Alias
	}{
		EpochIndex: fmt.Sprintf("0x%x", r.EpochIndex),
		InputIndex: fmt.Sprintf("0x%x", r.InputIndex),
		Index:      fmt.Sprintf("0x%x", r.Index),
		RawData:    "0x" + hex.EncodeToString(r.RawData),
		Alias:      (*Alias)(r),
	}
	return json.Marshal(aux)
}

type NodeConfig[T any] struct {
	Key       string
	Value     T
	CreatedAt time.Time
	UpdatedAt time.Time
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

func (e *DefaultBlock) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for DefaultBlock enum. Enum value has to be of type string or []byte")
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
		return errors.New("invalid value '" + enumValue + "' for DefaultBlock enum")
	}

	return nil
}

func (e DefaultBlock) String() string {
	return string(e)
}

type MonitoredEvent string

const (
	MonitoredEvent_InputAdded     MonitoredEvent = "InputAdded"
	MonitoredEvent_OutputExecuted MonitoredEvent = "OutputExecuted"
	MonitoredEvent_ClaimSubmitted MonitoredEvent = "ClaimSubmitted"
	MonitoredEvent_ClaimAccepted  MonitoredEvent = "ClaimAccepted"
	MonitoredEvent_EpochSealed    MonitoredEvent = "EpochSealed"
)

func (e MonitoredEvent) String() string {
	return string(e)
}

func Pointer[T any](v T) *T {
	return &v
}
