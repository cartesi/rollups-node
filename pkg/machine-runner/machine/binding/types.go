// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package binding

// #include <stdbool.h>
// #include <stdlib.h>
// #include <cartesi-machine/jsonrpc-machine-c-api.h>
import "C"
import (
	"fmt"
)

// ------------------------------
// ErrorCode
// ------------------------------

type ErrorCode uint8

const (
	ErrorOk ErrorCode = iota
	ErrorInvalidArgument
	ErrorDomain
	ErrorLength
	ErrorOutOfRange
	ErrorLogic
	_
	ErrorBadOptionalAccess
	ErrorRuntime
	ErrorRange
	ErrorOverflow
	ErrorUnderflow
	ErrorRegex
	ErrorSystemIosBaseFailure
	ErrorFilesystem
	ErrorAtomicTx
	ErrorNonexistingLocalTime
	ErrorAmbigousLocalTime
	ErrorFormat
	_
	ErrorBadTypeid
	ErrorBadCast
	ErrorBadAnyCast
	ErrorBadWeakPtr
	ErrorBadFunctionCall
	ErrorBadAlloc
	ErrorBadArrayNewLength
	ErrorBadException
	ErrorBadVariantAccesS
	ErrorException
	_
	ErrorUnknown
	invalidError
)

func newErrorCode(inner C.CM_ERROR) ErrorCode {
	machineError := ErrorCode(inner)
	if machineError >= invalidError {
		panic(fmt.Sprintf("invalid MachineError (%d)", machineError))
	}
	return machineError
}

func (code ErrorCode) String() string {
	switch code {
	case ErrorInvalidArgument:
		return "invalid argument"
	case ErrorDomain:
		return "domain"
	case ErrorLength:
		return "length"
	case ErrorOutOfRange:
		return "out Of Range"
	case ErrorLogic:
		return "logic"
	case ErrorBadOptionalAccess:
		return "bad optional access"
	case ErrorRuntime:
		return "runtime"
	case ErrorRange:
		return "range"
	case ErrorOverflow:
		return "overflow"
	case ErrorUnderflow:
		return "underflow"
	case ErrorRegex:
		return "error regex"
	case ErrorSystemIosBaseFailure:
		return "system ios base failure"
	case ErrorFilesystem:
		return "filesystem"
	case ErrorAtomicTx:
		return "atomic tx"
	case ErrorNonexistingLocalTime:
		return "nonexisting local time"
	case ErrorAmbigousLocalTime:
		return "ambigous local time"
	case ErrorFormat:
		return "format"
	case ErrorBadTypeid:
		return "bad type id"
	case ErrorBadCast:
		return "bad cast"
	case ErrorBadAnyCast:
		return "bad any cast"
	case ErrorBadWeakPtr:
		return "bad weak pointer"
	case ErrorBadFunctionCall:
		return "bad function call"
	case ErrorBadAlloc:
		return "bad alloc"
	case ErrorBadArrayNewLength:
		return "bad array new length"
	case ErrorBadException:
		return "bad exception"
	case ErrorBadVariantAccesS:
		return "bad variant accesS"
	case ErrorException:
		return "exception"
	case ErrorUnknown:
		return "unknown"
	default:
		return newErrorCode(C.CM_ERROR(code)).String()
	}
}

// ------------------------------
// BreakReason
// ------------------------------

type BreakReason uint8

const (
	BreakReasonFailed BreakReason = iota
	BreakReasonHalted
	BreakReasonYieldedManually
	BreakReasonYieldedAutomatically
	BreakReasonYieldedSoftly
	BreakReasonReachedTargetCycle
	InvalidBreakReason
)

func newBreakReason(inner C.CM_BREAK_REASON) BreakReason {
	breakReason := BreakReason(inner)
	breakReason.Check()
	return breakReason
}

func (reason BreakReason) Check() BreakReason {
	if reason >= InvalidBreakReason {
		panic(fmt.Sprintf("invalid BreakReason (%d)", reason))
	}
	return reason
}

func (breakReason BreakReason) String() string {
	switch breakReason {
	case BreakReasonFailed:
		return "failed"
	case BreakReasonHalted:
		return "halted"
	case BreakReasonYieldedManually:
		return "yielded manually"
	case BreakReasonYieldedAutomatically:
		return "yielded automatically"
	case BreakReasonYieldedSoftly:
		return "yielded softly"
	case BreakReasonReachedTargetCycle:
		return "reached target cycle"
	default:
		breakReason.Check()
		return "" // TODO
	}
}

// ------------------------------
// YieldReason
// ------------------------------

type YieldReason uint8

// TODO: test
const (
	YieldReasonProgress YieldReason = iota
	YieldReasonRxAccepted
	YieldReasonRxRejected
	YieldReasonTxOutput
	YieldReasonTxReport
	YieldReasonTxException
	InvalidYieldReason
)

func newYieldReason(inner uint64) YieldReason {
	yieldReason := YieldReason(inner)
	if yieldReason >= InvalidYieldReason {
		panic(fmt.Sprintf("invalid YieldReason (%d)", yieldReason))
	}
	return yieldReason
}

func (yieldReason YieldReason) String() string {
	switch yieldReason {
	case YieldReasonProgress:
		return "progress"
	case YieldReasonRxAccepted:
		return "rx accepted"
	case YieldReasonRxRejected:
		return "rx rejected"
	case YieldReasonTxOutput:
		return "tx output"
	case YieldReasonTxReport:
		return "tx report"
	case YieldReasonTxException:
		return "tx exception"
	default:
		return newYieldReason(uint64(yieldReason)).String()
	}
}

// ------------------------------
// RequestType
// ------------------------------

type RequestType uint8

const (
	AdvanceStateRequest RequestType = 0
	InspectStateRequest RequestType = 1
	InvalidRequestType  RequestType = 2
)

func (requestType RequestType) String() string {
	switch requestType {
	case AdvanceStateRequest:
		return "advance state"
	case InspectStateRequest:
		return "inspect state"
	default:
		panic(fmt.Sprintf("invalid RequestType (%d)", requestType))
	}
}

// ------------------------------
// Remote
// ------------------------------

type Remote struct {
	mgr *C.cm_jsonrpc_mg_mgr
}

func NewRemote(address string) (*Remote, error) {
	mgr, err := createJsonRpcMgr(address)
	if err != nil {
		return nil, err
	}
	return &Remote{mgr}, nil
}

func (remote *Remote) Fork() (address string, _ error) {
	return forkJsonrpcMgr(remote.mgr)
}

func (remote *Remote) Delete() {
	deleteJsonrpcMgr(remote.mgr)
}

func (remote *Remote) Shutdown() error {
	return jsonrpcShutdown(remote.mgr)
}

// ------------------------------
// Config
// ------------------------------

type RuntimeConfig struct {
	ConcurrencyUpdateMerkleTree uint64
	HtifNoConsolePutchar        bool
	SkipRootHashCheck           bool // TODO: always false
	SkipVersionCheck            bool // TODO: always false
}

func (config RuntimeConfig) toC() C.cm_machine_runtime_config {
	var inner C.cm_machine_runtime_config
	// TODO
	// inner.concurrency.update_merkle_tree = C.ulong(config.ConcurrencyUpdateMerkleTree)
	// inner.htif.no_console_putchar = C.bool(config.HtifNoConsolePutchar)
	// inner.skip_root_hash_check = C.bool(config.SkipRootHashCheck)
	// inner.skip_version_check = C.bool(config.SkipVersionCheck)
	return inner
}
