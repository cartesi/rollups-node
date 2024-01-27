// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

// #include "cartesi-machine/htif-defines.h"
import "C"

type HtifYieldType uint8

const (
	YieldAutomatic = C.HTIF_YIELD_AUTOMATIC_DEF
	YieldManual    = C.HTIF_YIELD_MANUAL_DEF
)

type HtifYieldReason uint8

const (
	// request
	YieldReasonProgress    HtifYieldReason = C.HTIF_YIELD_REASON_PROGRESS_DEF
	YieldReasonRxAccepted  HtifYieldReason = C.HTIF_YIELD_REASON_RX_ACCEPTED_DEF
	YieldReasonRxRejected  HtifYieldReason = C.HTIF_YIELD_REASON_RX_REJECTED_DEF
	YieldReasonTxOutput    HtifYieldReason = C.HTIF_YIELD_REASON_TX_OUTPUT_DEF
	YieldReasonTxReport    HtifYieldReason = C.HTIF_YIELD_REASON_TX_REPORT_DEF
	YieldReasonTxException HtifYieldReason = C.HTIF_YIELD_REASON_TX_EXCEPTION_DEF

	// reply
	YieldReasonAdvanceState HtifYieldReason = C.HTIF_YIELD_REASON_ADVANCE_STATE_DEF
	YieldReasonInspectState HtifYieldReason = C.HTIF_YIELD_REASON_INSPECT_STATE_DEF
)
