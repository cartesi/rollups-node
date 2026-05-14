// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
)

// hashSliceToByteSlice converts []common.Hash to [][32]byte without copying.
// This is safe because common.Hash is defined as [32]byte, so the memory layout is identical.
func hashSliceToByteSlice(b []common.Hash) [][32]byte {
	return *(*[][32]byte)(unsafe.Pointer(&b))
}
