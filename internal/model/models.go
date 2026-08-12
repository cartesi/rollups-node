// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Application struct {
	ID                                int64               `sql:"primary_key" json:"-"`
	Name                              string              `json:"name"`
	IApplicationAddress               common.Address      `json:"iapplication_address"`
	IConsensusAddress                 common.Address      `json:"iconsensus_address"`
	IInputBoxAddress                  common.Address      `json:"iinputbox_address"`
	TemplateHash                      common.Hash         `json:"template_hash"`
	TemplateURI                       string              `json:"-"`
	EpochLength                       uint64              `json:"epoch_length"`
	ClaimStagingPeriod                uint64              `json:"claim_staging_period"`
	WithdrawalConfig                  WithdrawalConfig    `json:"withdrawal_config"`
	DataAvailability                  []byte              `json:"data_availability"`
	ConsensusType                     Consensus           `json:"consensus_type"`
	Enabled                           bool                `json:"enabled"`
	Status                            ApplicationStatus   `json:"status"`
	Reason                            *string             `json:"reason"`
	IInputBoxBlock                    uint64              `json:"iinputbox_block"`
	LastEpochCheckBlock               uint64              `json:"last_epoch_check_block"`
	LastInputCheckBlock               uint64              `json:"last_input_check_block"`
	LastOutputCheckBlock              uint64              `json:"last_output_check_block"`
	LastTournamentCheckBlock          uint64              `json:"last_tournament_check_block"`
	LastForecloseCheckBlock           uint64              `json:"last_foreclose_check_block"`
	LastAccountsDriveProvedCheckBlock uint64              `json:"last_accounts_drive_proved_check_block"`
	LastWithdrawalCheckBlock          uint64              `json:"last_withdrawal_check_block"`
	ProcessedInputs                   uint64              `json:"processed_inputs"`
	ForecloseBlock                    uint64              `json:"foreclose_block"`
	ForecloseTransaction              *common.Hash        `json:"foreclose_transaction"`
	AccountsDriveProvedBlock          uint64              `json:"accounts_drive_proved_block"`
	AccountsDriveProvedTransaction    *common.Hash        `json:"accounts_drive_proved_transaction"`
	AccountsDriveMerkleRoot           *common.Hash        `json:"accounts_drive_merkle_root"`
	CreatedAt                         time.Time           `json:"created_at"`
	UpdatedAt                         time.Time           `json:"updated_at"`
	ExecutionParameters               ExecutionParameters `json:"execution_parameters"`
}

// IsForeclosed reports whether the node has observed an on-chain Foreclosure
// event for this application. Block 0 is unreachable for foreclosure (the
// contract is deployed at block >= 1), so 0 is the unambiguous "not observed
// yet" sentinel. Once non-zero it remains so (the chain-level foreclosed
// flag is one-way).
func (a *Application) IsForeclosed() bool {
	return a.ForecloseBlock != 0
}

func (a *Application) CanExecute() bool {
	return a.Enabled && a.Status == ApplicationStatus_OK && !a.IsForeclosed()
}

func (a *Application) NeedsL1Observation() bool {
	return a.Enabled
}

func (a *Application) NeedsForeclosureObservation() bool {
	return a.NeedsL1Observation() && !a.IsForeclosed()
}

func (a *Application) NeedsPostForeclosureObservation() bool {
	return a.NeedsL1Observation() && a.IsForeclosed()
}

// ForeclosureScanCaughtUp reports whether the historical L1 scan has reached
// foreclose_block, so the pre-foreclosure drain queries — which read the
// inputs/epochs already ingested into the DB — can be trusted.
//
// A freshly bootstrapped node can record foreclose_block before it has ingested
// the historical inputs/epochs. Until the scan catches up the drain tables are
// incomplete, and a "nothing left to drain" answer would be premature. Each
// consensus type advances a different cursor: DaveConsensus ingestion is driven
// by EpochSealed scans (last_epoch_check_block), while IConsensus ingestion is
// driven by InputAdded scans (last_input_check_block). This is the single
// definition of drain-readiness shared by the claimer, PRT, and manager.
//
// Only meaningful for a foreclosed app (foreclose_block != 0).
func (a *Application) ForeclosureScanCaughtUp() bool {
	if a.IsDaveConsensus() {
		return a.LastEpochCheckBlock >= a.ForecloseBlock
	}
	return a.LastInputCheckBlock >= a.ForecloseBlock
}

// WithdrawalConfig mirrors the on-chain five-immutable layout from the
// Application contract. Field order matches iapplicationfactory.WithdrawalConfig
// so the two are convertible via a Go type conversion.
type WithdrawalConfig struct {
	Guardian                common.Address `json:"guardian"`
	Log2LeavesPerAccount    uint8          `json:"log2_leaves_per_account"`
	Log2MaxNumOfAccounts    uint8          `json:"log2_max_num_of_accounts"`
	AccountsDriveStartIndex uint64         `json:"accounts_drive_start_index"`
	WithdrawalOutputBuilder common.Address `json:"withdrawal_output_builder"`
}

func (w WithdrawalConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Guardian                common.Address `json:"guardian"`
		Log2LeavesPerAccount    string         `json:"log2_leaves_per_account"`
		Log2MaxNumOfAccounts    string         `json:"log2_max_num_of_accounts"`
		AccountsDriveStartIndex string         `json:"accounts_drive_start_index"`
		WithdrawalOutputBuilder common.Address `json:"withdrawal_output_builder"`
	}{
		Guardian:                w.Guardian,
		Log2LeavesPerAccount:    fmt.Sprintf("0x%x", w.Log2LeavesPerAccount),
		Log2MaxNumOfAccounts:    fmt.Sprintf("0x%x", w.Log2MaxNumOfAccounts),
		AccountsDriveStartIndex: fmt.Sprintf("0x%x", w.AccountsDriveStartIndex),
		WithdrawalOutputBuilder: w.WithdrawalOutputBuilder,
	})
}

func (w *WithdrawalConfig) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Guardian                common.Address `json:"guardian"`
		Log2LeavesPerAccount    string         `json:"log2_leaves_per_account"`
		Log2MaxNumOfAccounts    string         `json:"log2_max_num_of_accounts"`
		AccountsDriveStartIndex string         `json:"accounts_drive_start_index"`
		WithdrawalOutputBuilder common.Address `json:"withdrawal_output_builder"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	w.Guardian = aux.Guardian
	w.WithdrawalOutputBuilder = aux.WithdrawalOutputBuilder
	if aux.Log2LeavesPerAccount != "" {
		v, err := ParseHexUint64(aux.Log2LeavesPerAccount)
		if err != nil {
			return fmt.Errorf("invalid log2_leaves_per_account: %w", err)
		}
		if v > math.MaxUint8 {
			return fmt.Errorf("log2_leaves_per_account out of range for uint8: %d", v)
		}
		w.Log2LeavesPerAccount = uint8(v)
	}
	if aux.Log2MaxNumOfAccounts != "" {
		v, err := ParseHexUint64(aux.Log2MaxNumOfAccounts)
		if err != nil {
			return fmt.Errorf("invalid log2_max_num_of_accounts: %w", err)
		}
		if v > math.MaxUint8 {
			return fmt.Errorf("log2_max_num_of_accounts out of range for uint8: %d", v)
		}
		w.Log2MaxNumOfAccounts = uint8(v)
	}
	if aux.AccountsDriveStartIndex != "" {
		v, err := ParseHexUint64(aux.AccountsDriveStartIndex)
		if err != nil {
			return fmt.Errorf("invalid accounts_drive_start_index: %w", err)
		}
		w.AccountsDriveStartIndex = v
	}
	return nil
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
		DataAvailability                  string `json:"data_availability"`
		IInputBoxBlock                    string `json:"iinputbox_block"`
		LastEpochCheckBlock               string `json:"last_epoch_check_block"`
		LastInputCheckBlock               string `json:"last_input_check_block"`
		LastOutputCheckBlock              string `json:"last_output_check_block"`
		LastTournamentCheckBlock          string `json:"last_tournament_check_block"`
		LastForecloseCheckBlock           string `json:"last_foreclose_check_block"`
		LastAccountsDriveProvedCheckBlock string `json:"last_accounts_drive_proved_check_block"`
		LastWithdrawalCheckBlock          string `json:"last_withdrawal_check_block"`
		EpochLength                       string `json:"epoch_length"`
		ClaimStagingPeriod                string `json:"claim_staging_period"`
		ProcessedInputs                   string `json:"processed_inputs"`
		ForecloseBlock                    string `json:"foreclose_block"`
		AccountsDriveProvedBlock          string `json:"accounts_drive_proved_block"`
	}{
		Alias:                             (*Alias)(a),
		DataAvailability:                  "0x" + hex.EncodeToString(a.DataAvailability),
		IInputBoxBlock:                    fmt.Sprintf("0x%x", a.IInputBoxBlock),
		LastEpochCheckBlock:               fmt.Sprintf("0x%x", a.LastEpochCheckBlock),
		LastInputCheckBlock:               fmt.Sprintf("0x%x", a.LastInputCheckBlock),
		LastOutputCheckBlock:              fmt.Sprintf("0x%x", a.LastOutputCheckBlock),
		LastTournamentCheckBlock:          fmt.Sprintf("0x%x", a.LastTournamentCheckBlock),
		LastForecloseCheckBlock:           fmt.Sprintf("0x%x", a.LastForecloseCheckBlock),
		LastAccountsDriveProvedCheckBlock: fmt.Sprintf("0x%x", a.LastAccountsDriveProvedCheckBlock),
		LastWithdrawalCheckBlock:          fmt.Sprintf("0x%x", a.LastWithdrawalCheckBlock),
		EpochLength:                       fmt.Sprintf("0x%x", a.EpochLength),
		ClaimStagingPeriod:                fmt.Sprintf("0x%x", a.ClaimStagingPeriod),
		ProcessedInputs:                   fmt.Sprintf("0x%x", a.ProcessedInputs),
		ForecloseBlock:                    fmt.Sprintf("0x%x", a.ForecloseBlock),
		AccountsDriveProvedBlock:          fmt.Sprintf("0x%x", a.AccountsDriveProvedBlock),
	}
	return json.Marshal(aux)
}

func (a *Application) UnmarshalJSON(in []byte) error {
	type Alias Application
	aux := &struct {
		*Alias

		DataAvailability                  string `json:"data_availability"`
		IInputBoxBlock                    string `json:"iinputbox_block"`
		LastInputCheckBlock               string `json:"last_input_check_block"`
		LastOutputCheckBlock              string `json:"last_output_check_block"`
		LastEpochCheckBlock               string `json:"last_epoch_check_block"`
		LastTournamentCheckBlock          string `json:"last_tournament_check_block"`
		LastForecloseCheckBlock           string `json:"last_foreclose_check_block"`
		LastAccountsDriveProvedCheckBlock string `json:"last_accounts_drive_proved_check_block"`
		LastWithdrawalCheckBlock          string `json:"last_withdrawal_check_block"`
		EpochLength                       string `json:"epoch_length"`
		ClaimStagingPeriod                string `json:"claim_staging_period"`
		ProcessedInputs                   string `json:"processed_inputs"`
		ForecloseBlock                    string `json:"foreclose_block"`
		AccountsDriveProvedBlock          string `json:"accounts_drive_proved_block"`
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

	a.LastTournamentCheckBlock, err = ParseHexUint64(aux.LastTournamentCheckBlock)
	if err != nil {
		return err
	}

	a.LastForecloseCheckBlock, err = ParseHexUint64(aux.LastForecloseCheckBlock)
	if err != nil {
		return err
	}

	if aux.LastAccountsDriveProvedCheckBlock != "" {
		a.LastAccountsDriveProvedCheckBlock, err = ParseHexUint64(aux.LastAccountsDriveProvedCheckBlock)
		if err != nil {
			return err
		}
	}

	if aux.LastWithdrawalCheckBlock != "" {
		a.LastWithdrawalCheckBlock, err = ParseHexUint64(aux.LastWithdrawalCheckBlock)
		if err != nil {
			return err
		}
	}

	a.EpochLength, err = ParseHexUint64(aux.EpochLength)
	if err != nil {
		return err
	}

	if aux.ClaimStagingPeriod != "" {
		a.ClaimStagingPeriod, err = ParseHexUint64(aux.ClaimStagingPeriod)
		if err != nil {
			return err
		}
	}

	a.ProcessedInputs, err = ParseHexUint64(aux.ProcessedInputs)
	if err != nil {
		return err
	}

	if aux.ForecloseBlock != "" {
		a.ForecloseBlock, err = ParseHexUint64(aux.ForecloseBlock)
		if err != nil {
			return err
		}
	}

	if aux.AccountsDriveProvedBlock != "" {
		a.AccountsDriveProvedBlock, err = ParseHexUint64(aux.AccountsDriveProvedBlock)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Application) IsDaveConsensus() bool {
	return a.ConsensusType == Consensus_PRT
}

// ApplicationStatus is the node's processing-integrity health for an
// application. It is independent of lifecycle (foreclosure lives in the
// foreclose_block column) and of operator intent (the enabled flag).
//
// Transitions (enforced by DB trigger):
//
//	OK ⇄ FAILED              (FAILED is recoverable by the operator)
//	OK, FAILED → DIVERGED    (terminal)
//	OK, FAILED → CORRUPTED   (terminal)
//
// DIVERGED means the node's computed claim disagrees with what the chain
// accepted; CORRUPTED means local state is missing or inconsistent. Both are
// terminal and carry a reason. Foreclosure is orthogonal and may coexist with
// any health value.
type ApplicationStatus string

const (
	ApplicationStatus_OK        ApplicationStatus = "OK"        // healthy; eligible for work when enabled and not foreclosed
	ApplicationStatus_Failed    ApplicationStatus = "FAILED"    // recoverable failure (e.g., OOM, process crash)
	ApplicationStatus_Diverged  ApplicationStatus = "DIVERGED"  // computed claim disagrees with the chain (terminal)
	ApplicationStatus_Corrupted ApplicationStatus = "CORRUPTED" // local state missing or inconsistent (terminal)
)

var ApplicationStatusAllValues = []ApplicationStatus{
	ApplicationStatus_OK,
	ApplicationStatus_Failed,
	ApplicationStatus_Diverged,
	ApplicationStatus_Corrupted,
}

func (e *ApplicationStatus) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for ApplicationStatus enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "OK":
		*e = ApplicationStatus_OK
	case "FAILED":
		*e = ApplicationStatus_Failed
	case "DIVERGED":
		*e = ApplicationStatus_Diverged
	case "CORRUPTED":
		*e = ApplicationStatus_Corrupted
	default:
		return errors.New("invalid value '" + enumValue + "' for ApplicationStatus enum")
	}

	return nil
}

func (e ApplicationStatus) String() string {
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

// Log2MaxExecutionCycles is the single node-side definition of the protocol
// execution window size. pkg/machine aliases it as Log2MaxMCyclesPerAdvanceState.
const Log2MaxExecutionCycles uint64 = 48

// MaxExecutionCycles is the number of mcycles in one machine-enforced window.
const MaxExecutionCycles uint64 = 1 << Log2MaxExecutionCycles

// MaxExecutionCycleSpan is the largest endpoint delta in that window.
const MaxExecutionCycleSpan uint64 = MaxExecutionCycles - 1

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

func ParseHexInt64(s string) (int64, error) {
	if s == "" || len(s) < 3 || (!strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X")) {
		return 0, fmt.Errorf("invalid hex string: %s", s)
	}
	return strconv.ParseInt(s[2:], 16, 64)
}

func ParseHexDuration(s string) (time.Duration, error) {
	ns, err := ParseHexInt64(s)
	if err != nil {
		return 0, err
	}
	if ns < 0 {
		return 0, fmt.Errorf("duration cannot be negative: %s", s)
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
	OutputsMerkleRoot    *common.Hash    `json:"outputs_merkle_root"`
	OutputsMerkleProof   []common.Hash   `json:"outputs_merkle_proof,omitempty"`
	ClaimTransactionHash *common.Hash    `json:"claim_transaction_hash"`
	Commitment           *common.Hash    `json:"commitment"`
	CommitmentProof      []common.Hash   `json:"commitment_proof,omitempty"`
	TournamentAddress    *common.Address `json:"tournament_address"`
	Status               EpochStatus     `json:"status"`
	StagedAtBlock        *uint64         `json:"staged_at_block"`
	VirtualIndex         uint64          `json:"virtual_index"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (e *Epoch) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Epoch
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		Index                string  `json:"index"`
		FirstBlock           string  `json:"first_block"`
		LastBlock            string  `json:"last_block"`
		InputIndexLowerBound string  `json:"input_index_lower_bound"`
		InputIndexUpperBound string  `json:"input_index_upper_bound"`
		StagedAtBlock        *string `json:"staged_at_block"`
		VirtualIndex         string  `json:"virtual_index"`
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
	if e.StagedAtBlock != nil {
		s := fmt.Sprintf("0x%x", *e.StagedAtBlock)
		aux.StagedAtBlock = &s
	}
	return json.Marshal(aux)
}

func (e *Epoch) UnmarshalJSON(in []byte) error {
	type Alias Epoch
	aux := &struct {
		*Alias

		Index                string  `json:"index"`
		FirstBlock           string  `json:"first_block"`
		LastBlock            string  `json:"last_block"`
		InputIndexLowerBound string  `json:"input_index_lower_bound"`
		InputIndexUpperBound string  `json:"input_index_upper_bound"`
		StagedAtBlock        *string `json:"staged_at_block"`
		VirtualIndex         string  `json:"virtual_index"`
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

	if aux.StagedAtBlock != nil {
		v, err := ParseHexUint64(*aux.StagedAtBlock)
		if err != nil {
			return err
		}
		e.StagedAtBlock = &v
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
	EpochStatus_ClaimStaged     EpochStatus = "CLAIM_STAGED"
	EpochStatus_ClaimAccepted   EpochStatus = "CLAIM_ACCEPTED"
	EpochStatus_ClaimRejected   EpochStatus = "CLAIM_REJECTED"
	EpochStatus_ClaimForeclosed EpochStatus = "CLAIM_FORECLOSED"
)

var EpochStatusAllValues = []EpochStatus{
	EpochStatus_Open,
	EpochStatus_Closed,
	EpochStatus_InputsProcessed,
	EpochStatus_ClaimComputed,
	EpochStatus_ClaimSubmitted,
	EpochStatus_ClaimStaged,
	EpochStatus_ClaimAccepted,
	EpochStatus_ClaimRejected,
	EpochStatus_ClaimForeclosed,
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
	case "CLAIM_STAGED":
		*e = EpochStatus_ClaimStaged
	case "CLAIM_ACCEPTED":
		*e = EpochStatus_ClaimAccepted
	case "CLAIM_REJECTED":
		*e = EpochStatus_ClaimRejected
	case "CLAIM_FORECLOSED":
		*e = EpochStatus_ClaimForeclosed
	default:
		return errors.New("invalid value '" + enumValue + "' for EpochStatus enum")
	}

	return nil
}

func (e EpochStatus) String() string {
	return string(e)
}

type Input struct {
	EpochApplicationID int64                 `sql:"primary_key" json:"-"`
	EpochIndex         uint64                `json:"epoch_index"`
	Index              uint64                `sql:"primary_key" json:"index"`
	BlockNumber        uint64                `json:"block_number"`
	RawData            []byte                `json:"raw_data"`
	Status             InputCompletionStatus `json:"status"`
	MachineHash        *common.Hash          `json:"machine_hash"`
	OutputsHash        *common.Hash          `json:"outputs_hash"`
	TransactionHash    common.Hash           `json:"transaction_hash"`
	LogIndex           uint64                `json:"log_index"`
	SnapshotURI        *string               `json:"-"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
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
		LogIndex    string `json:"log_index"`
		*Alias
	}{
		EpochIndex:  fmt.Sprintf("0x%x", i.EpochIndex),
		Index:       fmt.Sprintf("0x%x", i.Index),
		BlockNumber: fmt.Sprintf("0x%x", i.BlockNumber),
		RawData:     "0x" + hex.EncodeToString(i.RawData),
		LogIndex:    fmt.Sprintf("0x%x", i.LogIndex),
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
		LogIndex    string `json:"log_index"`
		*Alias
	}{Alias: (*Alias)(i)}

	var err error
	if err = json.Unmarshal(in, aux); err != nil {
		return err
	}

	i.EpochIndex, err = ParseHexUint64(aux.EpochIndex)
	if err != nil {
		return fmt.Errorf("error on EpochIndex: %w", err)
	}

	i.Index, err = ParseHexUint64(aux.Index)
	if err != nil {
		return fmt.Errorf("error on Index: %w", err)
	}

	i.BlockNumber, err = ParseHexUint64(aux.BlockNumber)
	if err != nil {
		return fmt.Errorf("error on BlockNumber: %w", err)
	}

	i.RawData, err = hexutil.Decode(aux.RawData)
	if err != nil {
		return fmt.Errorf("error on RawData: %w", err)
	}

	i.LogIndex, err = ParseHexUint64(aux.LogIndex)
	if err != nil {
		return fmt.Errorf("error on LogIndex: %w", err)
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
	InputCompletionStatus_ReportsLimitExceeded       InputCompletionStatus = "REPORTS_LIMIT_EXCEEDED"
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
	InputCompletionStatus_OutputsLimitExceeded,
	InputCompletionStatus_ReportsLimitExceeded,
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
	case "OUTPUTS_LIMIT_EXCEEDED":
		*e = InputCompletionStatus_OutputsLimitExceeded
	case "REPORTS_LIMIT_EXCEEDED":
		*e = InputCompletionStatus_ReportsLimitExceeded
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

func (o *Output) UnmarshalJSON(data []byte) error {
	type Alias Output
	aux := &struct {
		EpochIndex string `json:"epoch_index"`
		InputIndex string `json:"input_index"`
		Index      string `json:"index"`
		RawData    string `json:"raw_data"`
		*Alias
	}{Alias: (*Alias)(o)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*o = Output(*aux.Alias)

	var err error
	o.EpochIndex, err = ParseHexUint64(aux.EpochIndex)
	if err != nil {
		return fmt.Errorf("error on EpochIndex: %w", err)
	}
	o.InputIndex, err = ParseHexUint64(aux.InputIndex)
	if err != nil {
		return fmt.Errorf("error on InputIndex: %w", err)
	}
	o.Index, err = ParseHexUint64(aux.Index)
	if err != nil {
		return fmt.Errorf("error on Index: %w", err)
	}
	o.RawData, err = hexutil.Decode(aux.RawData)
	if err != nil {
		return fmt.Errorf("error on RawData: %w", err)
	}
	return nil
}

// Withdrawal records a Withdrawal(uint64 accountIndex, bytes account, bytes output)
// event emitted by an IApplication after the accounts drive has been proved.
// The node observes these only for applications with a non-zero ForecloseBlock
// and AccountsDriveProvedBlock; evmreader uses a FindTransitions scan on the
// on-chain getNumberOfWithdrawals counter to detect them. The contract marks
// each accountIndex as withdrawn, so the event fires at most once per slot.
//
// Account and Output are stored as raw bytes — the recipient encoding inside
// Account is defined by the per-app WithdrawalOutputBuilder and is opaque to
// the node. LogIndex is preserved (despite not being part of the primary key)
// so audits can locate the exact log on chain without re-querying.
type Withdrawal struct {
	ApplicationID   int64       `sql:"primary_key" json:"-"`
	AccountIndex    uint64      `sql:"primary_key" json:"account_index"`
	Account         []byte      `json:"account"`
	Output          []byte      `json:"output"`
	BlockNumber     uint64      `json:"block_number"`
	TransactionHash common.Hash `json:"transaction_hash"`
	LogIndex        uint        `json:"log_index"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

func (w *Withdrawal) MarshalJSON() ([]byte, error) {
	type Alias Withdrawal
	aux := &struct {
		AccountIndex string `json:"account_index"`
		Account      string `json:"account"`
		Output       string `json:"output"`
		BlockNumber  string `json:"block_number"`
		LogIndex     string `json:"log_index"`
		*Alias
	}{
		AccountIndex: fmt.Sprintf("0x%x", w.AccountIndex),
		Account:      "0x" + hex.EncodeToString(w.Account),
		Output:       "0x" + hex.EncodeToString(w.Output),
		BlockNumber:  fmt.Sprintf("0x%x", w.BlockNumber),
		LogIndex:     fmt.Sprintf("0x%x", w.LogIndex),
		Alias:        (*Alias)(w),
	}
	return json.Marshal(aux)
}

func (w *Withdrawal) UnmarshalJSON(data []byte) error {
	type Alias Withdrawal
	aux := &struct {
		AccountIndex string `json:"account_index"`
		Account      string `json:"account"`
		Output       string `json:"output"`
		BlockNumber  string `json:"block_number"`
		LogIndex     string `json:"log_index"`
		*Alias
	}{Alias: (*Alias)(w)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*w = Withdrawal(*aux.Alias)

	var err error
	w.AccountIndex, err = ParseHexUint64(aux.AccountIndex)
	if err != nil {
		return fmt.Errorf("error on AccountIndex: %w", err)
	}
	w.Account, err = hexutil.Decode(aux.Account)
	if err != nil {
		return fmt.Errorf("error on Account: %w", err)
	}
	w.Output, err = hexutil.Decode(aux.Output)
	if err != nil {
		return fmt.Errorf("error on Output: %w", err)
	}
	w.BlockNumber, err = ParseHexUint64(aux.BlockNumber)
	if err != nil {
		return fmt.Errorf("error on BlockNumber: %w", err)
	}
	logIndex, err := ParseHexUint64(aux.LogIndex)
	if err != nil {
		return fmt.Errorf("error on LogIndex: %w", err)
	}
	w.LogIndex = uint(logIndex)
	return nil
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

func (r *Report) UnmarshalJSON(data []byte) error {
	type Alias Report
	aux := &struct {
		EpochIndex string `json:"epoch_index"`
		InputIndex string `json:"input_index"`
		Index      string `json:"index"`
		RawData    string `json:"raw_data"`
		*Alias
	}{Alias: (*Alias)(r)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*r = Report(*aux.Alias)

	var err error
	r.EpochIndex, err = ParseHexUint64(aux.EpochIndex)
	if err != nil {
		return fmt.Errorf("error on EpochIndex: %w", err)
	}
	r.InputIndex, err = ParseHexUint64(aux.InputIndex)
	if err != nil {
		return fmt.Errorf("error on InputIndex: %w", err)
	}
	r.Index, err = ParseHexUint64(aux.Index)
	if err != nil {
		return fmt.Errorf("error on Index: %w", err)
	}
	r.RawData, err = hexutil.Decode(aux.RawData)
	if err != nil {
		return fmt.Errorf("error on RawData: %w", err)
	}
	return nil
}

type NodeConfig[T any] struct {
	Key       string
	Value     T
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OutputsProof struct {
	OutputsHash      common.Hash
	OutputsHashProof [][32]byte
	MachineHash      common.Hash
}

type AdvanceResult struct {
	OutputsProof
	EpochIndex          uint64
	InputIndex          uint64
	Status              InputCompletionStatus
	Outputs             [][]byte
	Reports             [][]byte
	ExceptionData       []byte
	PeriodicStateHashes [][32]byte
	PaddingRepetitions  uint64
	IsDaveConsensus     bool
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
	MonitoredEvent_InputAdded                    MonitoredEvent = "InputAdded"
	MonitoredEvent_OutputExecuted                MonitoredEvent = "OutputExecuted"
	MonitoredEvent_Foreclosure                   MonitoredEvent = "Foreclosure"
	MonitoredEvent_Withdrawal                    MonitoredEvent = "Withdrawal"
	MonitoredEvent_AccountsDriveMerkleRootProved MonitoredEvent = "AccountsDriveMerkleRootProved"
	MonitoredEvent_ClaimSubmitted                MonitoredEvent = "ClaimSubmitted"
	MonitoredEvent_ClaimAccepted                 MonitoredEvent = "ClaimAccepted"
	MonitoredEvent_EpochSealed                   MonitoredEvent = "EpochSealed"
	MonitoredEvent_CommitmentJoined              MonitoredEvent = "CommitmentJoined"
	MonitoredEvent_MatchAdvanced                 MonitoredEvent = "MatchAdvanced"
	MonitoredEvent_MatchCreated                  MonitoredEvent = "MatchCreated"
	MonitoredEvent_MatchDeleted                  MonitoredEvent = "MatchDeleted"
	MonitoredEvent_NewInnerTournament            MonitoredEvent = "NewInnerTournament"
)

func (e MonitoredEvent) String() string {
	return string(e)
}

type Tournament struct {
	ApplicationID           int64           `sql:"primary_key" json:"-"`
	EpochIndex              uint64          `sql:"primary_key" json:"epoch_index"`
	Address                 common.Address  `sql:"primary_key" json:"address"`
	ParentTournamentAddress *common.Address `json:"parent_tournament_address"`
	ParentMatchIDHash       *common.Hash    `json:"parent_match_id_hash"`
	MaxLevel                uint64          `json:"max_level"`
	Level                   uint64          `json:"level"`
	Log2Step                uint64          `json:"log2step"`
	Height                  uint64          `json:"height"`
	WinnerCommitment        *common.Hash    `json:"winner_commitment"`
	FinalStateHash          *common.Hash    `json:"final_state_hash"`
	FinishedAtBlock         uint64          `json:"finished_at_block"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

func (t *Tournament) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Tournament
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex      string `json:"epoch_index"`
		MaxLevel        string `json:"max_level"`
		Level           string `json:"level"`
		Log2Step        string `json:"log2step"`
		Height          string `json:"height"`
		FinishedAtBlock string `json:"finished_at_block"`
		*Alias
	}{
		Alias:           (*Alias)(t),
		EpochIndex:      fmt.Sprintf("0x%x", t.EpochIndex),
		MaxLevel:        fmt.Sprintf("0x%x", t.MaxLevel),
		Level:           fmt.Sprintf("0x%x", t.Level),
		Log2Step:        fmt.Sprintf("0x%x", t.Log2Step),
		Height:          fmt.Sprintf("0x%x", t.Height),
		FinishedAtBlock: fmt.Sprintf("0x%x", t.FinishedAtBlock),
	}
	return json.Marshal(aux)
}

func (t *Tournament) UnmarshalJSON(data []byte) error {
	type Alias Tournament
	aux := &struct {
		EpochIndex      string `json:"epoch_index"`
		MaxLevel        string `json:"max_level"`
		Level           string `json:"level"`
		Log2Step        string `json:"log2step"`
		Height          string `json:"height"`
		FinishedAtBlock string `json:"finished_at_block"`
		*Alias
	}{Alias: (*Alias)(t)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*t = Tournament(*aux.Alias)

	var err error
	t.EpochIndex, err = ParseHexUint64(aux.EpochIndex)
	if err != nil {
		return fmt.Errorf("error on EpochIndex: %w", err)
	}
	t.MaxLevel, err = ParseHexUint64(aux.MaxLevel)
	if err != nil {
		return fmt.Errorf("error on MaxLevel: %w", err)
	}
	t.Level, err = ParseHexUint64(aux.Level)
	if err != nil {
		return fmt.Errorf("error on Level: %w", err)
	}
	t.Log2Step, err = ParseHexUint64(aux.Log2Step)
	if err != nil {
		return fmt.Errorf("error on Log2Step: %w", err)
	}
	t.Height, err = ParseHexUint64(aux.Height)
	if err != nil {
		return fmt.Errorf("error on Height: %w", err)
	}
	t.FinishedAtBlock, err = ParseHexUint64(aux.FinishedAtBlock)
	if err != nil {
		return fmt.Errorf("error on FinishedAtBlock: %w", err)
	}
	return nil
}

type Commitment struct {
	ApplicationID     int64          `sql:"primary_key" json:"-"`
	EpochIndex        uint64         `sql:"primary_key" json:"epoch_index"`
	TournamentAddress common.Address `sql:"primary_key" json:"tournament_address"`
	Commitment        common.Hash    `sql:"primary_key" json:"commitment"`
	FinalStateHash    common.Hash    `json:"final_state_hash"`
	SubmitterAddress  common.Address `json:"submitter_address"`
	BlockNumber       uint64         `json:"block_number"`
	TxHash            common.Hash    `json:"tx_hash"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (c *Commitment) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Commitment
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		BlockNumber string `json:"block_number"`
		*Alias
	}{
		EpochIndex:  fmt.Sprintf("0x%x", c.EpochIndex),
		BlockNumber: fmt.Sprintf("0x%x", c.BlockNumber),
		Alias:       (*Alias)(c),
	}
	return json.Marshal(aux)
}

func (c *Commitment) UnmarshalJSON(data []byte) error {
	type Alias Commitment
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		BlockNumber string `json:"block_number"`
		*Alias
	}{Alias: (*Alias)(c)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*c = Commitment(*aux.Alias)

	var err error
	c.EpochIndex, err = ParseHexUint64(aux.EpochIndex)
	if err != nil {
		return fmt.Errorf("error on EpochIndex: %w", err)
	}
	c.BlockNumber, err = ParseHexUint64(aux.BlockNumber)
	if err != nil {
		return fmt.Errorf("error on BlockNumber: %w", err)
	}
	return nil
}

type Match struct {
	ApplicationID       int64               `sql:"primary_key" json:"-"`
	EpochIndex          uint64              `sql:"primary_key" json:"epoch_index"`
	TournamentAddress   common.Address      `sql:"primary_key" json:"tournament_address"`
	IDHash              common.Hash         `sql:"primary_key" json:"id_hash"`
	CommitmentOne       common.Hash         `json:"commitment_one"`
	CommitmentTwo       common.Hash         `json:"commitment_two"`
	LeftOfTwo           common.Hash         `json:"left_of_two"`
	BlockNumber         uint64              `json:"block_number"`
	TxHash              common.Hash         `json:"tx_hash"`
	Winner              WinnerCommitment    `json:"winner_commitment"`
	DeletionReason      MatchDeletionReason `json:"deletion_reason"`
	DeletionBlockNumber uint64              `json:"deletion_block_number"`
	DeletionTxHash      common.Hash         `json:"deletion_tx_hash"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

func (m *Match) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Match
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex          string `json:"epoch_index"`
		BlockNumber         string `json:"block_number"`
		DeletionBlockNumber string `json:"deletion_block_number"`
		*Alias
	}{
		EpochIndex:          fmt.Sprintf("0x%x", m.EpochIndex),
		BlockNumber:         fmt.Sprintf("0x%x", m.BlockNumber),
		DeletionBlockNumber: fmt.Sprintf("0x%x", m.DeletionBlockNumber),
		Alias:               (*Alias)(m),
	}
	return json.Marshal(aux)
}

type MatchAdvanced struct {
	ApplicationID     int64          `sql:"primary_key" json:"-"`
	EpochIndex        uint64         `sql:"primary_key" json:"epoch_index"`
	TournamentAddress common.Address `sql:"primary_key" json:"tournament_address"`
	IDHash            common.Hash    `sql:"primary_key" json:"id_hash"`
	OtherParent       common.Hash    `json:"other_parent"`
	LeftNode          common.Hash    `json:"left_node"`
	BlockNumber       uint64         `json:"block_number"`
	TxHash            common.Hash    `json:"tx_hash"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (m *MatchAdvanced) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias MatchAdvanced
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		BlockNumber string `json:"block_number"`
		*Alias
	}{
		EpochIndex:  fmt.Sprintf("0x%x", m.EpochIndex),
		BlockNumber: fmt.Sprintf("0x%x", m.BlockNumber),
		Alias:       (*Alias)(m),
	}
	return json.Marshal(aux)
}

// MatchDeletionReason represents the reason why a match was deleted
type MatchDeletionReason string

const (
	MatchDeletionReason_STEP             MatchDeletionReason = "STEP"
	MatchDeletionReason_TIMEOUT          MatchDeletionReason = "TIMEOUT"
	MatchDeletionReason_CHILD_TOURNAMENT MatchDeletionReason = "CHILD_TOURNAMENT"
	MatchDeletionReason_NOT_DELETED      MatchDeletionReason = "NOT_DELETED"
)

var MatchDeletionReasonAllValues = []MatchDeletionReason{
	MatchDeletionReason_STEP,
	MatchDeletionReason_TIMEOUT,
	MatchDeletionReason_CHILD_TOURNAMENT,
	MatchDeletionReason_NOT_DELETED,
}

func (e *MatchDeletionReason) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for MatchDeletionReason enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "STEP":
		*e = MatchDeletionReason_STEP
	case "TIMEOUT":
		*e = MatchDeletionReason_TIMEOUT
	case "CHILD_TOURNAMENT":
		*e = MatchDeletionReason_CHILD_TOURNAMENT
	case "NOT_DELETED":
		*e = MatchDeletionReason_NOT_DELETED
	default:
		return errors.New("invalid value '" + enumValue + "' for MatchDeletionReason enum")
	}

	return nil
}

func (e MatchDeletionReason) String() string {
	return string(e)
}

func MatchDeletionReasonFromUint8(v uint8) MatchDeletionReason {
	switch v {
	case 0:
		return MatchDeletionReason_STEP
	case 1:
		return MatchDeletionReason_TIMEOUT
	case 2: //nolint: mnd
		return MatchDeletionReason_CHILD_TOURNAMENT
	case 0xff: //nolint: mnd
		return MatchDeletionReason_NOT_DELETED
	default:
		return MatchDeletionReason_STEP // default to STEP for unknown values
	}
}

// WinnerCommitment represents the winner commitment of a match
type WinnerCommitment string

const (
	WinnerCommitment_NONE WinnerCommitment = "NONE"
	WinnerCommitment_ONE  WinnerCommitment = "ONE"
	WinnerCommitment_TWO  WinnerCommitment = "TWO"
)

var WinnerCommitmentAllValues = []WinnerCommitment{
	WinnerCommitment_NONE,
	WinnerCommitment_ONE,
	WinnerCommitment_TWO,
}

func (e *WinnerCommitment) Scan(value any) error {
	var enumValue string
	switch val := value.(type) {
	case string:
		enumValue = val
	case []byte:
		enumValue = string(val)
	default:
		return errors.New("invalid value for WinnerCommitment enum. Enum value has to be of type string or []byte")
	}

	switch enumValue {
	case "NONE":
		*e = WinnerCommitment_NONE
	case "ONE":
		*e = WinnerCommitment_ONE
	case "TWO":
		*e = WinnerCommitment_TWO
	default:
		return errors.New("invalid value '" + enumValue + "' for WinnerCommitment enum")
	}

	return nil
}

func (e WinnerCommitment) String() string {
	return string(e)
}

func WinnerCommitmentFromUint8(v uint8) WinnerCommitment {
	switch v {
	case 0:
		return WinnerCommitment_NONE
	case 1:
		return WinnerCommitment_ONE
	case 2: //nolint: mnd
		return WinnerCommitment_TWO
	default:
		return WinnerCommitment_NONE // default to NONE for unknown values
	}
}

type StateHash struct {
	InputEpochApplicationID int64       `sql:"primary_key" json:"-"`
	EpochIndex              uint64      `json:"epoch_index"`
	InputIndex              uint64      `json:"input_index"`
	Index                   uint64      `sql:"primary_key" json:"index"`
	MachineHash             common.Hash `json:"machine_hash"`
	Repetitions             uint64      `json:"repetitions"`
	CreatedAt               time.Time   `json:"created_at"`
	UpdatedAt               time.Time   `json:"updated_at"`
}

func (s *StateHash) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias StateHash
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		InputIndex  string `json:"input_index"`
		Index       string `json:"index"`
		Repetitions string `json:"repetitions"`
		*Alias
	}{
		EpochIndex:  fmt.Sprintf("0x%x", s.EpochIndex),
		InputIndex:  fmt.Sprintf("0x%x", s.InputIndex),
		Index:       fmt.Sprintf("0x%x", s.Index),
		Repetitions: fmt.Sprintf("0x%x", s.Repetitions),
		Alias:       (*Alias)(s),
	}
	return json.Marshal(aux)
}

func (s *StateHash) UnmarshalJSON(data []byte) error {
	// Create an alias to avoid infinite recursion in UnmarshalJSON.
	type Alias StateHash
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		EpochIndex  string `json:"epoch_index"`
		InputIndex  string `json:"input_index"`
		Index       string `json:"index"`
		Repetitions string `json:"repetitions"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.EpochIndex != "" {
		val, err := ParseHexUint64(aux.EpochIndex)
		if err != nil {
			return fmt.Errorf("invalid epoch_index: %w", err)
		}
		s.EpochIndex = val
	}

	if aux.InputIndex != "" {
		val, err := ParseHexUint64(aux.InputIndex)
		if err != nil {
			return fmt.Errorf("invalid input_index: %w", err)
		}
		s.InputIndex = val
	}

	if aux.Index != "" {
		val, err := ParseHexUint64(aux.Index)
		if err != nil {
			return fmt.Errorf("invalid index: %w", err)
		}
		s.Index = val
	}

	if aux.Repetitions != "" {
		val, err := ParseHexUint64(aux.Repetitions)
		if err != nil {
			return fmt.Errorf("invalid repetitions: %w", err)
		}
		s.Repetitions = val
	}

	return nil
}

func Pointer[T any](v T) *T {
	return &v
}
