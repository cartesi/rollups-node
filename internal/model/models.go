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

func (a *Application) NeedsL1Observation() bool {
	return a.Enabled
}

// ForeclosureScanCaughtUp reports whether the historical L1 scan has reached
// foreclose_block, so the pre-foreclosure drain queries — which read the
// inputs/epochs already ingested into the DB — can be trusted.
//
// A freshly bootstrapped node can record foreclose_block before it has ingested
// the historical inputs/epochs. Until the scan catches up the drain tables are
// incomplete, and a "nothing left to drain" answer would be premature. Each
// IConsensus ingestion is driven by InputAdded scans (last_input_check_block).
// DaveConsensus has two independent scans: sealed epochs advance
// last_epoch_check_block, while inputs in the current open epoch advance
// last_input_check_block. Both must reach the foreclosure boundary before its
// historical state is complete. This is the single definition of
// drain-readiness shared by the claimer, PRT, and manager.
//
// Only meaningful for a foreclosed app (foreclose_block != 0).
func (a *Application) ForeclosureScanCaughtUp() bool {
	if a.IsDaveConsensus() {
		return a.LastEpochCheckBlock >= a.ForecloseBlock &&
			a.LastInputCheckBlock >= a.ForecloseBlock
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

func (a *Application) MarshalJSON() ([]byte, error) {
	// Create an alias to avoid infinite recursion in MarshalJSON.
	type Alias Application
	// Define a new structure that embeds the alias but overrides the hex fields.
	aux := &struct {
		*Alias
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

// ApplicationStatus records why the node may no longer execute an application
// or produce claims for it. It combines recoverable operational failures,
// integrity failures, and deterministic terminal machine outcomes. It is
// independent of lifecycle (foreclosure lives in the foreclose_block column)
// and of operator intent (the enabled flag).
//
// Transitions (enforced by DB trigger):
//
//	OK ⇄ FAILED                                 (FAILED is recoverable)
//	OK, FAILED → DIVERGED, CORRUPTED              (integrity terminal)
//	OK         → GUEST_EXCEPTION, MACHINE_HALTED  (execution terminal)
//	             MCYCLE_OVERFLOW, UNEXPECTED_YIELD
//	execution terminal → CORRUPTED                 (integrity escalation)
//
// FAILED is recoverable and suspends execution, but enabled applications in
// every status continue L1 observation. Consequently, later evidence may
// supersede FAILED with DIVERGED or CORRUPTED. DIVERGED means the node's
// computed claim disagrees with what the chain accepted; CORRUPTED means local
// state or its relationship with L1 history is missing or inconsistent.
//
// GUEST_EXCEPTION, MACHINE_HALTED, MCYCLE_OVERFLOW, and UNEXPECTED_YIELD record
// deterministic machine outcomes that stop execution and must not be retried.
// Later L1 observation may supersede one with CORRUPTED when it establishes
// that local history is untrustworthy. The input retains its original
// completion status and terminal state proof. Foreclosure is orthogonal and
// may coexist with any application status.
type ApplicationStatus string

//nolint:revive // Public enum names preserve the generated/API naming convention.
const (
	ApplicationStatus_OK              ApplicationStatus = "OK"        // healthy; eligible for work when enabled and not foreclosed
	ApplicationStatus_Failed          ApplicationStatus = "FAILED"    // recoverable failure (e.g., OOM, process crash)
	ApplicationStatus_Diverged        ApplicationStatus = "DIVERGED"  // computed claim disagrees with the chain (terminal)
	ApplicationStatus_Corrupted       ApplicationStatus = "CORRUPTED" // local state missing or inconsistent (terminal)
	ApplicationStatus_GuestException  ApplicationStatus = "GUEST_EXCEPTION"
	ApplicationStatus_MachineHalted   ApplicationStatus = "MACHINE_HALTED"
	ApplicationStatus_McycleOverflow  ApplicationStatus = "MCYCLE_OVERFLOW"
	ApplicationStatus_UnexpectedYield ApplicationStatus = "UNEXPECTED_YIELD"
)

var ApplicationStatusAllValues = []ApplicationStatus{
	ApplicationStatus_OK,
	ApplicationStatus_Failed,
	ApplicationStatus_Diverged,
	ApplicationStatus_Corrupted,
	ApplicationStatus_GuestException,
	ApplicationStatus_MachineHalted,
	ApplicationStatus_McycleOverflow,
	ApplicationStatus_UnexpectedYield,
}

func (e ApplicationStatus) IsTerminal() bool {
	switch e {
	case ApplicationStatus_Diverged,
		ApplicationStatus_Corrupted,
		ApplicationStatus_GuestException,
		ApplicationStatus_MachineHalted,
		ApplicationStatus_McycleOverflow,
		ApplicationStatus_UnexpectedYield:
		return true
	case ApplicationStatus_OK, ApplicationStatus_Failed:
		return false
	}
	return false
}

// IsExecutionTerminal reports whether machine execution ended deterministically
// and must not be retried. Unlike DIVERGED and CORRUPTED, these states may still
// escalate to CORRUPTED when later L1 observation disproves local history.
func (e ApplicationStatus) IsExecutionTerminal() bool {
	switch e {
	case ApplicationStatus_GuestException,
		ApplicationStatus_MachineHalted,
		ApplicationStatus_McycleOverflow,
		ApplicationStatus_UnexpectedYield:
		return true
	case ApplicationStatus_OK,
		ApplicationStatus_Failed,
		ApplicationStatus_Diverged,
		ApplicationStatus_Corrupted:
		return false
	}
	return false
}

func (e *ApplicationStatus) Scan(value any) error {
	return scanEnum(e, value, ApplicationStatusAllValues, "ApplicationStatus")
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
	return scanEnum(e, value, ConsensusAllValues, "Consensus")
}

func (e Consensus) String() string {
	return string(e)
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
	return scanEnum(e, value, SnapshotPolicyAllValues, "SnapshotPolicy")
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
// execution window size. pkg/machine aliases it as Log2MaxMCyclesPerAdvanceState,
// so the execution ceiling and computation-hash dimensions cannot diverge. The
// emulator exposes the same value as
// CM_ROLLUP_LOG2_MAX_MCYCLES_PER_ADVANCE_STATE.
const Log2MaxExecutionCycles uint64 = 48

// MaxExecutionCycles is the number of cycles in one machine-enforced execution
// window. It is the mcycle window covered by one input hash collection.
const MaxExecutionCycles uint64 = 1 << Log2MaxExecutionCycles

// MaxExecutionCycleSpan is the largest configurable distance between the
// starting mcycle and the execution endpoint. The endpoint itself is included
// in the MaxExecutionCycles-wide window, hence the subtraction by one. A
// configured maximum of zero means no operator-imposed cap; the machine's
// MaxExecutionCycleSpan ceiling applies instead.
const MaxExecutionCycleSpan uint64 = MaxExecutionCycles - 1

// validateParameters constants
const maxDuration = 24 * time.Hour
const maxConcurrentInspects = 1000

// validateParameters performs validation on the loaded parameters
func (e *ExecutionParameters) Validate() error {
	if e.AdvanceIncCycles == 0 {
		return errors.New("advance_inc_cycles must be greater than 0")
	}
	if e.AdvanceMaxCycles > MaxExecutionCycleSpan {
		return fmt.Errorf("advance_max_cycles must be between 0 and %d", MaxExecutionCycleSpan)
	}
	if e.InspectIncCycles == 0 {
		return errors.New("inspect_inc_cycles must be greater than 0")
	}
	if e.InspectMaxCycles > MaxExecutionCycleSpan {
		return fmt.Errorf("inspect_max_cycles must be between 0 and %d", MaxExecutionCycleSpan)
	}

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
	TxBufferDataBlock    *common.Hash    `json:"tx_buffer_data_block"`
	TxBufferProof        []common.Hash   `json:"tx_buffer_proof,omitempty"`
	IflagsYDataBlock     *common.Hash    `json:"iflags_y_data_block"`
	IflagsYProof         []common.Hash   `json:"iflags_y_proof,omitempty"`
	HtifTohostDataBlock  *common.Hash    `json:"htif_tohost_data_block"`
	HtifTohostProof      []common.Hash   `json:"htif_tohost_proof,omitempty"`
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

// NonTerminalEpochStatuses returns the states that still require epoch or claim
// work. Each caller owns the returned slice.
// Keep this set equal to the epoch_unreconciled_idx predicate in the initial
// PostgreSQL migration (000001_create_initial_schema.up.sql).
func NonTerminalEpochStatuses() []EpochStatus {
	return []EpochStatus{
		EpochStatus_Open,
		EpochStatus_Closed,
		EpochStatus_InputsProcessed,
		EpochStatus_ClaimComputed,
		EpochStatus_ClaimSubmitted,
		EpochStatus_ClaimStaged,
	}
}

func (e *EpochStatus) Scan(value any) error {
	return scanEnum(e, value, EpochStatusAllValues, "EpochStatus")
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
	ExceptionData      []byte                `json:"-"`
	MachineHash        *common.Hash          `json:"machine_hash"`
	TxBufferDataBlock  *common.Hash          `json:"tx_buffer_data_block"`
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
	var exceptionData *string
	if i.ExceptionData != nil {
		encoded := hexutil.Encode(i.ExceptionData)
		exceptionData = &encoded
	}
	aux := &struct {
		EpochIndex    string  `json:"epoch_index"`
		Index         string  `json:"index"`
		BlockNumber   string  `json:"block_number"`
		RawData       string  `json:"raw_data"`
		ExceptionData *string `json:"exception_data"`
		LogIndex      string  `json:"log_index"`
		*Alias
	}{
		EpochIndex:    fmt.Sprintf("0x%x", i.EpochIndex),
		Index:         fmt.Sprintf("0x%x", i.Index),
		BlockNumber:   fmt.Sprintf("0x%x", i.BlockNumber),
		RawData:       "0x" + hex.EncodeToString(i.RawData),
		ExceptionData: exceptionData,
		LogIndex:      fmt.Sprintf("0x%x", i.LogIndex),
		Alias:         (*Alias)(i),
	}
	return json.Marshal(aux)
}

func (i *Input) UnmarshalJSON(in []byte) error {
	type Alias Input
	aux := &struct {
		EpochIndex    string  `json:"epoch_index"`
		Index         string  `json:"index"`
		BlockNumber   string  `json:"block_number"`
		RawData       string  `json:"raw_data"`
		ExceptionData *string `json:"exception_data"`
		LogIndex      string  `json:"log_index"`
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
	if aux.ExceptionData == nil {
		i.ExceptionData = nil
	} else {
		i.ExceptionData, err = hexutil.Decode(*aux.ExceptionData)
		if err != nil {
			return fmt.Errorf("error on ExceptionData: %w", err)
		}
	}

	i.LogIndex, err = ParseHexUint64(aux.LogIndex)
	if err != nil {
		return fmt.Errorf("error on LogIndex: %w", err)
	}

	return nil
}

type InputCompletionStatus string

//nolint:revive // Public enum names preserve the generated/API naming convention.
const (
	InputCompletionStatus_None            InputCompletionStatus = "NONE"
	InputCompletionStatus_Accepted        InputCompletionStatus = "ACCEPTED"
	InputCompletionStatus_Rejected        InputCompletionStatus = "REJECTED"
	InputCompletionStatus_Exception       InputCompletionStatus = "EXCEPTION"
	InputCompletionStatus_MachineHalted   InputCompletionStatus = "MACHINE_HALTED"
	InputCompletionStatus_Overflow        InputCompletionStatus = "OVERFLOW"
	InputCompletionStatus_UnexpectedYield InputCompletionStatus = "UNEXPECTED_YIELD"
)

var InputCompletionStatusAllValues = []InputCompletionStatus{
	InputCompletionStatus_None,
	InputCompletionStatus_Accepted,
	InputCompletionStatus_Rejected,
	InputCompletionStatus_Exception,
	InputCompletionStatus_MachineHalted,
	InputCompletionStatus_Overflow,
	InputCompletionStatus_UnexpectedYield,
}

// IsCompleted reports whether the status is a deterministic completed result
// of an advance execution. NONE represents an input that has not completed.
func (e InputCompletionStatus) IsCompleted() bool {
	switch e {
	case InputCompletionStatus_Accepted,
		InputCompletionStatus_Rejected,
		InputCompletionStatus_Exception,
		InputCompletionStatus_MachineHalted,
		InputCompletionStatus_Overflow,
		InputCompletionStatus_UnexpectedYield:
		return true
	case InputCompletionStatus_None:
		return false
	default:
		return false
	}
}

// IsTerminal reports whether a completed input leaves the canonical machine
// unable to process another advance. Rejection is completed but not terminal.
func (e InputCompletionStatus) IsTerminal() bool {
	_, terminal := e.TerminalApplicationStatus()
	return terminal
}

// TerminalApplicationStatus maps an execution terminal to the durable status
// exposed for the application. The boolean is false for pending, accepted, and
// rejected inputs.
func (e InputCompletionStatus) TerminalApplicationStatus() (ApplicationStatus, bool) {
	switch e {
	case InputCompletionStatus_Exception:
		return ApplicationStatus_GuestException, true
	case InputCompletionStatus_MachineHalted:
		return ApplicationStatus_MachineHalted, true
	case InputCompletionStatus_Overflow:
		return ApplicationStatus_McycleOverflow, true
	case InputCompletionStatus_UnexpectedYield:
		return ApplicationStatus_UnexpectedYield, true
	case InputCompletionStatus_None,
		InputCompletionStatus_Accepted,
		InputCompletionStatus_Rejected:
		return "", false
	default:
		return "", false
	}
}

func (e *InputCompletionStatus) Scan(value any) error {
	return scanEnum(e, value, InputCompletionStatusAllValues, "InputCompletionStatus")
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

func (o *Output) MarshalJSON() ([]byte, error) {
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
		EpochIndex: fmt.Sprintf("0x%x", o.EpochIndex),
		InputIndex: fmt.Sprintf("0x%x", o.InputIndex),
		Index:      fmt.Sprintf("0x%x", o.Index),
		RawData:    "0x" + hex.EncodeToString(o.RawData),
		Alias:      (*Alias)(o),
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

type StateProof struct {
	TxBufferDataBlock   common.Hash
	TxBufferProof       [][32]byte
	MachineHash         common.Hash
	IflagsYDataBlock    common.Hash
	IflagsYProof        [][32]byte
	HtifTohostDataBlock common.Hash
	HtifTohostProof     [][32]byte
}

// ErrIncompleteStateProof means an epoch does not contain every persisted
// component required to reconstruct its machine state proof.
var ErrIncompleteStateProof = errors.New("epoch state proof is incomplete")

// StateProofSiblingCount is the height of the canonical machine
// memory tree above a 32-byte data block (64 - 5).
const StateProofSiblingCount = 59

// IsComplete reports whether all three state leaves have the canonical sibling
// depth. The machine validates proof contents when it collects them. The
// contracts validate the persisted proof when the node submits it.
func (p *StateProof) IsComplete() bool {
	return p != nil &&
		len(p.TxBufferProof) == StateProofSiblingCount &&
		len(p.IflagsYProof) == StateProofSiblingCount &&
		len(p.HtifTohostProof) == StateProofSiblingCount
}

// HasCompleteStateProof reports whether an epoch contains every persisted
// component of the machine state proof.
func (e *Epoch) HasCompleteStateProof() bool {
	return e != nil &&
		e.MachineHash != nil &&
		e.TxBufferDataBlock != nil &&
		e.IflagsYDataBlock != nil &&
		e.HtifTohostDataBlock != nil &&
		len(e.TxBufferProof) == StateProofSiblingCount &&
		len(e.IflagsYProof) == StateProofSiblingCount &&
		len(e.HtifTohostProof) == StateProofSiblingCount
}

// StateProof reconstructs the persisted machine state proof owned by an
// epoch. The returned value owns its sibling slices.
func (e *Epoch) StateProof() (StateProof, error) {
	if !e.HasCompleteStateProof() {
		return StateProof{}, ErrIncompleteStateProof
	}

	return StateProof{
		TxBufferDataBlock:   *e.TxBufferDataBlock,
		TxBufferProof:       copyStateProofSiblings(e.TxBufferProof),
		MachineHash:         *e.MachineHash,
		IflagsYDataBlock:    *e.IflagsYDataBlock,
		IflagsYProof:        copyStateProofSiblings(e.IflagsYProof),
		HtifTohostDataBlock: *e.HtifTohostDataBlock,
		HtifTohostProof:     copyStateProofSiblings(e.HtifTohostProof),
	}, nil
}

func copyStateProofSiblings(siblings []common.Hash) [][32]byte {
	result := make([][32]byte, len(siblings))
	for i := range siblings {
		result[i] = siblings[i]
	}
	return result
}

type AdvanceResult struct {
	StateProof
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

// ReplaySummary identifies the immutable completed-input prefix selected for
// one machine replay.
type ReplaySummary struct {
	ApplicationID   int64
	ProcessedInputs uint64
	Consensus       Consensus
}

// ReplayInput is the canonical input evidence needed to verify one replayed
// machine execution. It is deliberately narrower than Input: L1 metadata,
// timestamps, and snapshot location do not participate in the comparison.
type ReplayInput struct {
	ApplicationID     int64
	EpochIndex        uint64
	InputIndex        uint64
	RawData           []byte
	Status            InputCompletionStatus
	ExceptionData     []byte
	MachineHash       *common.Hash
	TxBufferDataBlock *common.Hash
}

// ReplayStateHash is one persisted row of a PRT input hash collection. Keeping
// this projection narrow matters because one input may contain millions of
// rows.
type ReplayStateHash struct {
	Index       uint64
	MachineHash common.Hash
	Repetitions uint64
}

// ReplayRecord contains one completed input and its requested verification
// evidence. Canonical records leave Outputs, Reports, and StateHashes empty.
type ReplayRecord struct {
	Input       ReplayInput
	Outputs     [][]byte
	Reports     [][]byte
	StateHashes []ReplayStateHash
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
	return scanEnum(e, value, DefaultBlockAllValues, "DefaultBlock")
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
	MonitoredEvent_LeafMatchSealed               MonitoredEvent = "LeafMatchSealed"
	MonitoredEvent_PartialBondRefund             MonitoredEvent = "PartialBondRefund"
	MonitoredEvent_BondRecovered                 MonitoredEvent = "BondRecovered"
)

func (e MonitoredEvent) String() string {
	return string(e)
}

type Tournament struct {
	ApplicationID           int64                    `sql:"primary_key" json:"-"`
	EpochIndex              uint64                   `sql:"primary_key" json:"epoch_index"`
	Address                 common.Address           `sql:"primary_key" json:"address"`
	ParentTournamentAddress *common.Address          `json:"parent_tournament_address"`
	ParentMatchIDHash       *common.Hash             `json:"parent_match_id_hash"`
	MaxLevel                uint64                   `json:"max_level"`
	Level                   uint64                   `json:"level"`
	Log2Step                uint64                   `json:"log2step"`
	Height                  uint64                   `json:"height"`
	InitialHash             common.Hash              `json:"initial_hash"`
	BaseCycle               Uint256                  `json:"base_cycle"`
	Kind                    TournamentKind           `json:"kind"`
	StartInstant            uint64                   `json:"start_instant"`
	Allowance               uint64                   `json:"allowance"`
	CreationEvent           *TournamentCreationEvent `json:"creation_event"`
	Snapshot                TournamentSnapshot       `json:"snapshot"`
	CreatedAt               time.Time                `json:"created_at"`
	UpdatedAt               time.Time                `json:"updated_at"`
}

func (value Tournament) MarshalJSON() ([]byte, error) {
	type Alias Tournament
	return json.Marshal(struct {
		*Alias
		EpochIndex   hexutil.Uint64 `json:"epoch_index"`
		MaxLevel     hexutil.Uint64 `json:"max_level"`
		Level        hexutil.Uint64 `json:"level"`
		Log2Step     hexutil.Uint64 `json:"log2step"`
		Height       hexutil.Uint64 `json:"height"`
		StartInstant hexutil.Uint64 `json:"start_instant"`
		Allowance    hexutil.Uint64 `json:"allowance"`
	}{
		Alias:        (*Alias)(&value),
		EpochIndex:   hexutil.Uint64(value.EpochIndex),
		MaxLevel:     hexutil.Uint64(value.MaxLevel),
		Level:        hexutil.Uint64(value.Level),
		Log2Step:     hexutil.Uint64(value.Log2Step),
		Height:       hexutil.Uint64(value.Height),
		StartInstant: hexutil.Uint64(value.StartInstant),
		Allowance:    hexutil.Uint64(value.Allowance),
	})
}

func (value *Tournament) UnmarshalJSON(data []byte) error {
	type Alias Tournament
	var decoded Tournament
	aux := struct {
		*Alias
		EpochIndex   hexutil.Uint64 `json:"epoch_index"`
		MaxLevel     hexutil.Uint64 `json:"max_level"`
		Level        hexutil.Uint64 `json:"level"`
		Log2Step     hexutil.Uint64 `json:"log2step"`
		Height       hexutil.Uint64 `json:"height"`
		StartInstant hexutil.Uint64 `json:"start_instant"`
		Allowance    hexutil.Uint64 `json:"allowance"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.EpochIndex = uint64(aux.EpochIndex)
	decoded.MaxLevel = uint64(aux.MaxLevel)
	decoded.Level = uint64(aux.Level)
	decoded.Log2Step = uint64(aux.Log2Step)
	decoded.Height = uint64(aux.Height)
	decoded.StartInstant = uint64(aux.StartInstant)
	decoded.Allowance = uint64(aux.Allowance)
	*value = decoded
	return nil
}

type Commitment struct {
	ApplicationID     int64              `sql:"primary_key" json:"-"`
	EpochIndex        uint64             `sql:"primary_key" json:"epoch_index"`
	TournamentAddress common.Address     `sql:"primary_key" json:"tournament_address"`
	Commitment        common.Hash        `sql:"primary_key" json:"commitment"`
	FinalStateHash    common.Hash        `json:"final_state_hash"`
	SubmitterAddress  common.Address     `json:"submitter_address"`
	BlockNumber       uint64             `json:"block_number"`
	TxHash            common.Hash        `json:"tx_hash"`
	LogIndex          uint64             `json:"log_index"`
	Snapshot          CommitmentSnapshot `json:"snapshot"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

func (value Commitment) MarshalJSON() ([]byte, error) {
	type Alias Commitment
	return json.Marshal(struct {
		*Alias
		EpochIndex  hexutil.Uint64 `json:"epoch_index"`
		BlockNumber hexutil.Uint64 `json:"block_number"`
		LogIndex    hexutil.Uint64 `json:"log_index"`
	}{
		Alias:       (*Alias)(&value),
		EpochIndex:  hexutil.Uint64(value.EpochIndex),
		BlockNumber: hexutil.Uint64(value.BlockNumber),
		LogIndex:    hexutil.Uint64(value.LogIndex),
	})
}

func (value *Commitment) UnmarshalJSON(data []byte) error {
	type Alias Commitment
	var decoded Commitment
	aux := struct {
		*Alias
		EpochIndex  hexutil.Uint64 `json:"epoch_index"`
		BlockNumber hexutil.Uint64 `json:"block_number"`
		LogIndex    hexutil.Uint64 `json:"log_index"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.EpochIndex = uint64(aux.EpochIndex)
	decoded.BlockNumber = uint64(aux.BlockNumber)
	decoded.LogIndex = uint64(aux.LogIndex)
	*value = decoded
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
	LogIndex            uint64              `json:"log_index"`
	EliminableAt        uint64              `json:"eliminable_at"`
	LeafSeal            *LeafMatchSeal      `json:"leaf_seal"`
	Winner              WinnerCommitment    `json:"winner_commitment"`
	DeletionReason      MatchDeletionReason `json:"deletion_reason"`
	DeletionBlockNumber uint64              `json:"deletion_block_number"`
	DeletionTxHash      *common.Hash        `json:"deletion_tx_hash"`
	DeletionLogIndex    *uint64             `json:"deletion_log_index"`
	Snapshot            MatchSnapshot       `json:"snapshot"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

func (value Match) MarshalJSON() ([]byte, error) {
	type Alias Match
	return json.Marshal(struct {
		*Alias
		EpochIndex          hexutil.Uint64  `json:"epoch_index"`
		BlockNumber         hexutil.Uint64  `json:"block_number"`
		LogIndex            hexutil.Uint64  `json:"log_index"`
		EliminableAt        hexutil.Uint64  `json:"eliminable_at"`
		DeletionBlockNumber hexutil.Uint64  `json:"deletion_block_number"`
		DeletionLogIndex    *hexutil.Uint64 `json:"deletion_log_index"`
	}{
		Alias:               (*Alias)(&value),
		EpochIndex:          hexutil.Uint64(value.EpochIndex),
		BlockNumber:         hexutil.Uint64(value.BlockNumber),
		LogIndex:            hexutil.Uint64(value.LogIndex),
		EliminableAt:        hexutil.Uint64(value.EliminableAt),
		DeletionBlockNumber: hexutil.Uint64(value.DeletionBlockNumber),
		DeletionLogIndex:    (*hexutil.Uint64)(value.DeletionLogIndex),
	})
}

func (value *Match) UnmarshalJSON(data []byte) error {
	type Alias Match
	var decoded Match
	aux := struct {
		*Alias
		EpochIndex          hexutil.Uint64  `json:"epoch_index"`
		BlockNumber         hexutil.Uint64  `json:"block_number"`
		LogIndex            hexutil.Uint64  `json:"log_index"`
		EliminableAt        hexutil.Uint64  `json:"eliminable_at"`
		DeletionBlockNumber hexutil.Uint64  `json:"deletion_block_number"`
		DeletionLogIndex    *hexutil.Uint64 `json:"deletion_log_index"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.EpochIndex = uint64(aux.EpochIndex)
	decoded.BlockNumber = uint64(aux.BlockNumber)
	decoded.LogIndex = uint64(aux.LogIndex)
	decoded.EliminableAt = uint64(aux.EliminableAt)
	decoded.DeletionBlockNumber = uint64(aux.DeletionBlockNumber)
	decoded.DeletionLogIndex = (*uint64)(aux.DeletionLogIndex)
	*value = decoded
	return nil
}

type MatchAdvanced struct {
	ApplicationID        int64          `sql:"primary_key" json:"-"`
	EpochIndex           uint64         `json:"epoch_index"`
	TournamentAddress    common.Address `json:"tournament_address"`
	IDHash               common.Hash    `json:"id_hash"`
	OtherParent          common.Hash    `json:"other_parent"`
	LeftNode             common.Hash    `json:"left_node"`
	SegmentStartPosition Uint256        `json:"segment_start_position"`
	EliminableAt         uint64         `json:"eliminable_at"`
	BlockNumber          uint64         `json:"block_number"`
	TxHash               common.Hash    `sql:"primary_key" json:"tx_hash"`
	LogIndex             uint64         `sql:"primary_key" json:"log_index"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

func (value MatchAdvanced) MarshalJSON() ([]byte, error) {
	type Alias MatchAdvanced
	return json.Marshal(struct {
		*Alias
		EpochIndex   hexutil.Uint64 `json:"epoch_index"`
		EliminableAt hexutil.Uint64 `json:"eliminable_at"`
		BlockNumber  hexutil.Uint64 `json:"block_number"`
		LogIndex     hexutil.Uint64 `json:"log_index"`
	}{
		Alias:        (*Alias)(&value),
		EpochIndex:   hexutil.Uint64(value.EpochIndex),
		EliminableAt: hexutil.Uint64(value.EliminableAt),
		BlockNumber:  hexutil.Uint64(value.BlockNumber),
		LogIndex:     hexutil.Uint64(value.LogIndex),
	})
}

func (value *MatchAdvanced) UnmarshalJSON(data []byte) error {
	type Alias MatchAdvanced
	var decoded MatchAdvanced
	aux := struct {
		*Alias
		EpochIndex   hexutil.Uint64 `json:"epoch_index"`
		EliminableAt hexutil.Uint64 `json:"eliminable_at"`
		BlockNumber  hexutil.Uint64 `json:"block_number"`
		LogIndex     hexutil.Uint64 `json:"log_index"`
	}{Alias: (*Alias)(&decoded)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	decoded.EpochIndex = uint64(aux.EpochIndex)
	decoded.EliminableAt = uint64(aux.EliminableAt)
	decoded.BlockNumber = uint64(aux.BlockNumber)
	decoded.LogIndex = uint64(aux.LogIndex)
	*value = decoded
	return nil
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
	return scanEnum(e, value, MatchDeletionReasonAllValues, "MatchDeletionReason")
}

func (e MatchDeletionReason) String() string {
	return string(e)
}

func MatchDeletionReasonFromUint8(v uint8) (MatchDeletionReason, error) {
	switch v {
	case 0:
		return MatchDeletionReason_STEP, nil
	case 1:
		return MatchDeletionReason_TIMEOUT, nil
	case 2: //nolint: mnd
		return MatchDeletionReason_CHILD_TOURNAMENT, nil
	default:
		return "", fmt.Errorf("unmodelled MatchDeletionReason %d from contract", v)
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
	return scanEnum(e, value, WinnerCommitmentAllValues, "WinnerCommitment")
}

func (e WinnerCommitment) String() string {
	return string(e)
}

func WinnerCommitmentFromUint8(v uint8) (WinnerCommitment, error) {
	switch v {
	case 0:
		return WinnerCommitment_NONE, nil
	case 1:
		return WinnerCommitment_ONE, nil
	case 2: //nolint: mnd
		return WinnerCommitment_TWO, nil
	default:
		return "", fmt.Errorf("unmodelled WinnerCommitment %d from contract", v)
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
