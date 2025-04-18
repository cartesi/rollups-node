// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package is a binding to the emulator's C API.
// Refer to the machine-c files in the emulator's repository for documentation
// (mainly machine-c-api.h and jsonrpc-machine-c-api.h).
package emulator

// #include <stdlib.h>
// #include "cartesi-machine/jsonrpc-machine-c-api.h"
import "C"
import "fmt"

// -----------------------------------------------------------------------------
// Error + Helpers
// -----------------------------------------------------------------------------

type ErrorCode int32

const (
	ErrCodeOk                ErrorCode = C.CM_ERROR_OK
	ErrCodeInvalidArgument   ErrorCode = C.CM_ERROR_INVALID_ARGUMENT
	ErrCodeDomainError       ErrorCode = C.CM_ERROR_DOMAIN_ERROR
	ErrCodeLengthError       ErrorCode = C.CM_ERROR_LENGTH_ERROR
	ErrCodeOutOfRange        ErrorCode = C.CM_ERROR_OUT_OF_RANGE
	ErrCodeLogicError        ErrorCode = C.CM_ERROR_LOGIC_ERROR
	ErrCodeRuntimeError      ErrorCode = C.CM_ERROR_RUNTIME_ERROR
	ErrCodeRangeError        ErrorCode = C.CM_ERROR_RANGE_ERROR
	ErrCodeOverflowError     ErrorCode = C.CM_ERROR_OVERFLOW_ERROR
	ErrCodeUnderflowError    ErrorCode = C.CM_ERROR_UNDERFLOW_ERROR
	ErrCodeRegexError        ErrorCode = C.CM_ERROR_REGEX_ERROR
	ErrCodeSystemError       ErrorCode = C.CM_ERROR_SYSTEM_ERROR
	ErrCodeBadTypeid         ErrorCode = C.CM_ERROR_BAD_TYPEID
	ErrCodeBadCast           ErrorCode = C.CM_ERROR_BAD_CAST
	ErrCodeBadAnyCast        ErrorCode = C.CM_ERROR_BAD_ANY_CAST
	ErrCodeBadOptionalAccess ErrorCode = C.CM_ERROR_BAD_OPTIONAL_ACCESS
	ErrCodeBadWeakPtr        ErrorCode = C.CM_ERROR_BAD_WEAK_PTR
	ErrCodeBadFunctionCall   ErrorCode = C.CM_ERROR_BAD_FUNCTION_CALL
	ErrCodeBadAlloc          ErrorCode = C.CM_ERROR_BAD_ALLOC
	ErrCodeBadArrayNewLength ErrorCode = C.CM_ERROR_BAD_ARRAY_NEW_LENGTH
	ErrCodeBadException      ErrorCode = C.CM_ERROR_BAD_EXCEPTION
	ErrCodeBadVariantAccess  ErrorCode = C.CM_ERROR_BAD_VARIANT_ACCESS
	ErrCodeException         ErrorCode = C.CM_ERROR_EXCEPTION
	ErrCodeUnknown           ErrorCode = C.CM_ERROR_UNKNOWN
)

type Error struct {
	Code ErrorCode
	Msg  string
}

func (e Error) Error() string {
	return fmt.Sprintf("Cartesi JSONRPC error %d: %s", e.Code, e.Msg)
}

func newError(code C.cm_error) error {
	if code == C.CM_ERROR_OK {
		return nil
	}
	msg := C.GoString(C.cm_get_last_error_message())
	return Error{Code: ErrorCode(code), Msg: msg}
}

// -----------------------------------------------------------------------------
// Enums + Constants from machine-c-api.h & jsonrpc-machine-c-api.h
// -----------------------------------------------------------------------------

// Cleanup calls
const (
	CM_JSONRPC_NOTHING  = C.CM_JSONRPC_NOTHING
	CM_JSONRPC_DESTROY  = C.CM_JSONRPC_DESTROY
	CM_JSONRPC_SHUTDOWN = C.CM_JSONRPC_SHUTDOWN
)

// BreakReason
type BreakReason int32

const (
	BreakReasonFailed               BreakReason = C.CM_BREAK_REASON_FAILED
	BreakReasonHalted               BreakReason = C.CM_BREAK_REASON_HALTED
	BreakReasonYieldedManually      BreakReason = C.CM_BREAK_REASON_YIELDED_MANUALLY
	BreakReasonYieldedAutomatically BreakReason = C.CM_BREAK_REASON_YIELDED_AUTOMATICALLY
	BreakReasonYieldedSoftly        BreakReason = C.CM_BREAK_REASON_YIELDED_SOFTLY
	BreakReasonReachedTargetMcycle  BreakReason = C.CM_BREAK_REASON_REACHED_TARGET_MCYCLE
)

func (reason BreakReason) String() (s string) {
	switch reason {
	case BreakReasonFailed:
		s = "failed"
	case BreakReasonHalted:
		s = "halted"
	case BreakReasonYieldedManually:
		s = "yielded manually"
	case BreakReasonYieldedAutomatically:
		s = "yielded automatically"
	case BreakReasonYieldedSoftly:
		s = "yielded softly"
	case BreakReasonReachedTargetMcycle:
		s = "reached target mcycle"
	default:
		return "invalid break reason"
	}
	return "break reason: " + s

}

type (
	CmioYieldCommand uint8
	CmioYieldReason  uint16
)

const (
	// type
	YieldAutomatic CmioYieldCommand = C.CM_CMIO_YIELD_COMMAND_AUTOMATIC
	YieldManual    CmioYieldCommand = C.CM_CMIO_YIELD_COMMAND_MANUAL

	// NOTE: these values do not form an enum (e.g., automatic-progress == manual-accepted).

	// reason - request
	AutomaticYieldReasonProgress CmioYieldReason = C.CM_CMIO_YIELD_AUTOMATIC_REASON_PROGRESS
	AutomaticYieldReasonOutput   CmioYieldReason = C.CM_CMIO_YIELD_AUTOMATIC_REASON_TX_OUTPUT
	AutomaticYieldReasonReport   CmioYieldReason = C.CM_CMIO_YIELD_AUTOMATIC_REASON_TX_REPORT

	ManualYieldReasonAccepted  CmioYieldReason = C.CM_CMIO_YIELD_MANUAL_REASON_RX_ACCEPTED
	ManualYieldReasonRejected  CmioYieldReason = C.CM_CMIO_YIELD_MANUAL_REASON_RX_REJECTED
	ManualYieldReasonException CmioYieldReason = C.CM_CMIO_YIELD_MANUAL_REASON_TX_EXCEPTION

	// reason - reply
	YieldReasonAdvanceState CmioYieldReason = C.CM_CMIO_YIELD_REASON_ADVANCE_STATE
	YieldReasonInspectState CmioYieldReason = C.CM_CMIO_YIELD_REASON_INSPECT_STATE
)

// cm_reg addresses/CSRs
type RegID = int32 // from cm_reg enum

const (
	// Processor x registers
	REG_X0  RegID = C.CM_REG_X0
	REG_X1  RegID = C.CM_REG_X1
	REG_X2  RegID = C.CM_REG_X2
	REG_X3  RegID = C.CM_REG_X3
	REG_X4  RegID = C.CM_REG_X4
	REG_X5  RegID = C.CM_REG_X5
	REG_X6  RegID = C.CM_REG_X6
	REG_X7  RegID = C.CM_REG_X7
	REG_X8  RegID = C.CM_REG_X8
	REG_X9  RegID = C.CM_REG_X9
	REG_X10 RegID = C.CM_REG_X10
	REG_X11 RegID = C.CM_REG_X11
	REG_X12 RegID = C.CM_REG_X12
	REG_X13 RegID = C.CM_REG_X13
	REG_X14 RegID = C.CM_REG_X14
	REG_X15 RegID = C.CM_REG_X15
	REG_X16 RegID = C.CM_REG_X16
	REG_X17 RegID = C.CM_REG_X17
	REG_X18 RegID = C.CM_REG_X18
	REG_X19 RegID = C.CM_REG_X19
	REG_X20 RegID = C.CM_REG_X20
	REG_X21 RegID = C.CM_REG_X21
	REG_X22 RegID = C.CM_REG_X22
	REG_X23 RegID = C.CM_REG_X23
	REG_X24 RegID = C.CM_REG_X24
	REG_X25 RegID = C.CM_REG_X25
	REG_X26 RegID = C.CM_REG_X26
	REG_X27 RegID = C.CM_REG_X27
	REG_X28 RegID = C.CM_REG_X28
	REG_X29 RegID = C.CM_REG_X29
	REG_X30 RegID = C.CM_REG_X30
	REG_X31 RegID = C.CM_REG_X31
	// Processor f registers
	REG_F0  RegID = C.CM_REG_F0
	REG_F1  RegID = C.CM_REG_F1
	REG_F2  RegID = C.CM_REG_F2
	REG_F3  RegID = C.CM_REG_F3
	REG_F4  RegID = C.CM_REG_F4
	REG_F5  RegID = C.CM_REG_F5
	REG_F6  RegID = C.CM_REG_F6
	REG_F7  RegID = C.CM_REG_F7
	REG_F8  RegID = C.CM_REG_F8
	REG_F9  RegID = C.CM_REG_F9
	REG_F10 RegID = C.CM_REG_F10
	REG_F11 RegID = C.CM_REG_F11
	REG_F12 RegID = C.CM_REG_F12
	REG_F13 RegID = C.CM_REG_F13
	REG_F14 RegID = C.CM_REG_F14
	REG_F15 RegID = C.CM_REG_F15
	REG_F16 RegID = C.CM_REG_F16
	REG_F17 RegID = C.CM_REG_F17
	REG_F18 RegID = C.CM_REG_F18
	REG_F19 RegID = C.CM_REG_F19
	REG_F20 RegID = C.CM_REG_F20
	REG_F21 RegID = C.CM_REG_F21
	REG_F22 RegID = C.CM_REG_F22
	REG_F23 RegID = C.CM_REG_F23
	REG_F24 RegID = C.CM_REG_F24
	REG_F25 RegID = C.CM_REG_F25
	REG_F26 RegID = C.CM_REG_F26
	REG_F27 RegID = C.CM_REG_F27
	REG_F28 RegID = C.CM_REG_F28
	REG_F29 RegID = C.CM_REG_F29
	REG_F30 RegID = C.CM_REG_F30
	REG_F31 RegID = C.CM_REG_F31
	// Processor CSRs
	REG_PC            RegID = C.CM_REG_PC
	REG_FCSR          RegID = C.CM_REG_FCSR
	REG_MVENDORID     RegID = C.CM_REG_MVENDORID
	REG_MARCHID       RegID = C.CM_REG_MARCHID
	REG_MIMPID        RegID = C.CM_REG_MIMPID
	REG_MCYCLE        RegID = C.CM_REG_MCYCLE
	REG_ICYCLEINSTRET RegID = C.CM_REG_ICYCLEINSTRET
	REG_MSTATUS       RegID = C.CM_REG_MSTATUS
	REG_MTVEC         RegID = C.CM_REG_MTVEC
	REG_MSCRATCH      RegID = C.CM_REG_MSCRATCH
	REG_MEPC          RegID = C.CM_REG_MEPC
	REG_MCAUSE        RegID = C.CM_REG_MCAUSE
	REG_MTVAL         RegID = C.CM_REG_MTVAL
	REG_MISA          RegID = C.CM_REG_MISA
	REG_MIE           RegID = C.CM_REG_MIE
	REG_MIP           RegID = C.CM_REG_MIP
	REG_MEDELEG       RegID = C.CM_REG_MEDELEG
	REG_MIDELEG       RegID = C.CM_REG_MIDELEG
	REG_MCOUNTEREN    RegID = C.CM_REG_MCOUNTEREN
	REG_MENVCFG       RegID = C.CM_REG_MENVCFG
	REG_STVEC         RegID = C.CM_REG_STVEC
	REG_SSCRATCH      RegID = C.CM_REG_SSCRATCH
	REG_SEPC          RegID = C.CM_REG_SEPC
	REG_SCAUSE        RegID = C.CM_REG_SCAUSE
	REG_STVAL         RegID = C.CM_REG_STVAL
	REG_SATP          RegID = C.CM_REG_SATP
	REG_SCOUNTEREN    RegID = C.CM_REG_SCOUNTEREN
	REG_SENVCFG       RegID = C.CM_REG_SENVCFG
	REG_ILRSC         RegID = C.CM_REG_ILRSC
	REG_IPRV          RegID = C.CM_REG_IPRV
	REG_IFLAGS_X      RegID = C.CM_REG_IFLAGS_X
	REG_IFLAGS_Y      RegID = C.CM_REG_IFLAGS_Y
	REG_IFLAGS_H      RegID = C.CM_REG_IFLAGS_H
	REG_IUNREP        RegID = C.CM_REG_IUNREP
	// Device registers
	REG_CLINT_MTIMECMP RegID = C.CM_REG_CLINT_MTIMECMP
	REG_PLIC_GIRQPEND  RegID = C.CM_REG_PLIC_GIRQPEND
	REG_PLIC_GIRQSRVD  RegID = C.CM_REG_PLIC_GIRQSRVD
	REG_HTIF_TOHOST    RegID = C.CM_REG_HTIF_TOHOST
	REG_HTIF_FROMHOST  RegID = C.CM_REG_HTIF_FROMHOST
	REG_HTIF_IHALT     RegID = C.CM_REG_HTIF_IHALT
	REG_HTIF_ICONSOLE  RegID = C.CM_REG_HTIF_ICONSOLE
	REG_HTIF_IYIELD    RegID = C.CM_REG_HTIF_IYIELD
	// Microarchitecture registers
	REG_UARCH_X0        RegID = C.CM_REG_UARCH_X0
	REG_UARCH_X1        RegID = C.CM_REG_UARCH_X1
	REG_UARCH_X2        RegID = C.CM_REG_UARCH_X2
	REG_UARCH_X3        RegID = C.CM_REG_UARCH_X3
	REG_UARCH_X4        RegID = C.CM_REG_UARCH_X4
	REG_UARCH_X5        RegID = C.CM_REG_UARCH_X5
	REG_UARCH_X6        RegID = C.CM_REG_UARCH_X6
	REG_UARCH_X7        RegID = C.CM_REG_UARCH_X7
	REG_UARCH_X8        RegID = C.CM_REG_UARCH_X8
	REG_UARCH_X9        RegID = C.CM_REG_UARCH_X9
	REG_UARCH_X10       RegID = C.CM_REG_UARCH_X10
	REG_UARCH_X11       RegID = C.CM_REG_UARCH_X11
	REG_UARCH_X12       RegID = C.CM_REG_UARCH_X12
	REG_UARCH_X13       RegID = C.CM_REG_UARCH_X13
	REG_UARCH_X14       RegID = C.CM_REG_UARCH_X14
	REG_UARCH_X15       RegID = C.CM_REG_UARCH_X15
	REG_UARCH_X16       RegID = C.CM_REG_UARCH_X16
	REG_UARCH_X17       RegID = C.CM_REG_UARCH_X17
	REG_UARCH_X18       RegID = C.CM_REG_UARCH_X18
	REG_UARCH_X19       RegID = C.CM_REG_UARCH_X19
	REG_UARCH_X20       RegID = C.CM_REG_UARCH_X20
	REG_UARCH_X21       RegID = C.CM_REG_UARCH_X21
	REG_UARCH_X22       RegID = C.CM_REG_UARCH_X22
	REG_UARCH_X23       RegID = C.CM_REG_UARCH_X23
	REG_UARCH_X24       RegID = C.CM_REG_UARCH_X24
	REG_UARCH_X25       RegID = C.CM_REG_UARCH_X25
	REG_UARCH_X26       RegID = C.CM_REG_UARCH_X26
	REG_UARCH_X27       RegID = C.CM_REG_UARCH_X27
	REG_UARCH_X28       RegID = C.CM_REG_UARCH_X28
	REG_UARCH_X29       RegID = C.CM_REG_UARCH_X29
	REG_UARCH_X30       RegID = C.CM_REG_UARCH_X30
	REG_UARCH_X31       RegID = C.CM_REG_UARCH_X31
	REG_UARCH_PC        RegID = C.CM_REG_UARCH_PC
	REG_UARCH_CYCLE     RegID = C.CM_REG_UARCH_CYCLE
	REG_UARCH_HALT_FLAG RegID = C.CM_REG_UARCH_HALT_FLAG
	// Views of registers
	REG_HTIF_TOHOST_DEV      RegID = C.CM_REG_HTIF_TOHOST_DEV
	REG_HTIF_TOHOST_CMD      RegID = C.CM_REG_HTIF_TOHOST_CMD
	REG_HTIF_TOHOST_REASON   RegID = C.CM_REG_HTIF_TOHOST_REASON
	REG_HTIF_TOHOST_DATA     RegID = C.CM_REG_HTIF_TOHOST_DATA
	REG_HTIF_FROMHOST_DEV    RegID = C.CM_REG_HTIF_FROMHOST_DEV
	REG_HTIF_FROMHOST_CMD    RegID = C.CM_REG_HTIF_FROMHOST_CMD
	REG_HTIF_FROMHOST_REASON RegID = C.CM_REG_HTIF_FROMHOST_REASON
	REG_HTIF_FROMHOST_DATA   RegID = C.CM_REG_HTIF_FROMHOST_DATA
	// Enumeration helpers
	REG_UNKNOWN RegID = C.CM_REG_UNKNOWN_
	REG_FIRST   RegID = C.CM_REG_FIRST_
	REG_LAST    RegID = C.CM_REG_LAST_
)

const (
	CmioRxBufferStart    uint64 = C.CM_PMA_CMIO_RX_BUFFER_START
	CmioRxBufferLog2Size uint64 = C.CM_PMA_CMIO_RX_BUFFER_LOG2_SIZE

	CmioTxBufferStart    uint64 = C.CM_PMA_CMIO_TX_BUFFER_START
	CmioTxBufferLog2Size uint64 = C.CM_PMA_CMIO_TX_BUFFER_LOG2_SIZE
)

type MachineRuntimeConfig struct {
	Concurrency       *ConcurrencyRuntimeConfig `json:"concurrency,omitempty"`
	Htif              *HtifRuntimeConfig        `json:"htif,omitempty"`
	SkipRootHashCheck bool                      `json:"skip_root_hash_check"`
	SkipRootHashStore bool                      `json:"skip_root_hash_store"`
	SkipVersionCheck  bool                      `json:"skip_version_check"`
	SoftYield         bool                      `json:"soft_yield"`
}

type HtifRuntimeConfig struct {
	NoConsolePutchar bool `json:"no_console_putchar"`
}

type ConcurrencyRuntimeConfig struct {
	UpdateMerkleTree uint64 `json:"update_merkle_tree"`
}

func NewMachineRuntimeConfig() *MachineRuntimeConfig {
	return &MachineRuntimeConfig{
		Concurrency: &ConcurrencyRuntimeConfig{
			UpdateMerkleTree: 0,
		},
		Htif: &HtifRuntimeConfig{
			NoConsolePutchar: false,
		},
		SkipRootHashCheck: false,
		SkipRootHashStore: false,
		SkipVersionCheck:  false,
		SoftYield:         false,
	}
}
