// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

// #include "cartesi-machine/htif-defines.h"
import "C"

type (
	HtifYieldType   uint8
	HtifYieldReason uint8
)

const (
	// type
	YieldAutomatic = C.HTIF_YIELD_CMD_AUTOMATIC_DEF
	YieldManual    = C.HTIF_YIELD_CMD_MANUAL_DEF

	// NOTE: these do not form an enum (e.g., automatic-progress == manual-accepted).

	// reason - request
	AutomaticYieldReasonProgress HtifYieldReason = C.HTIF_YIELD_AUTOMATIC_REASON_PROGRESS_DEF
	AutomaticYieldReasonOutput   HtifYieldReason = C.HTIF_YIELD_AUTOMATIC_REASON_TX_OUTPUT_DEF
	AutomaticYieldReasonReport   HtifYieldReason = C.HTIF_YIELD_AUTOMATIC_REASON_TX_REPORT_DEF

	ManualYieldReasonAccepted  HtifYieldReason = C.HTIF_YIELD_MANUAL_REASON_RX_ACCEPTED_DEF
	ManualYieldReasonRejected  HtifYieldReason = C.HTIF_YIELD_MANUAL_REASON_RX_REJECTED_DEF
	ManualYieldReasonException HtifYieldReason = C.HTIF_YIELD_MANUAL_REASON_TX_EXCEPTION_DEF

	// reason - reply
	YieldReasonAdvanceState HtifYieldReason = C.HTIF_YIELD_REASON_ADVANCE_STATE_DEF
	YieldReasonInspectState HtifYieldReason = C.HTIF_YIELD_REASON_INSPECT_STATE_DEF
)
