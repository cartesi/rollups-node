// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

// #include "cartesi-machine/pma-defines.h"
import "C"

const (
	CmioRxBufferStart uint64 = C.PMA_CMIO_RX_BUFFER_START_DEF
	CmioTxBufferStart uint64 = C.PMA_CMIO_TX_BUFFER_START_DEF
)
