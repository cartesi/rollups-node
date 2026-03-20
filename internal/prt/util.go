// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/hex"
	"strings"
	"unsafe"

	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Re-export for backward compatibility within the package.
type JSONRPCInfo = ethutil.JSONRPCInfo

var ExtractJSONErrorInfo = ethutil.ExtractJSONErrorInfo

// errorStringSelector is the 4-byte selector for Solidity's Error(string),
// used by require(condition, "reason") statements. This is keccak256("Error(string)")[:4]
// and is a well-known, stable Solidity ABI constant (0x08c379a0).
var errorStringSelector = [4]byte{0x08, 0xc3, 0x79, 0xa0}

// isRevertReason checks whether an RPC error contains an Error(string) revert
// with the given reason. It extracts the rpc.DataError, decodes the
// ABI-encoded Error(string) payload, and compares the decoded string.
// This is more robust than matching err.Error() because it operates on
// structured revert data, independent of how the RPC provider formats errors.
func isRevertReason(err error, reason string) bool {
	info, ok := ExtractJSONErrorInfo(err)
	if !ok || !info.HasData {
		return false
	}
	dataStr, ok := info.Data.(string)
	if !ok {
		return false
	}
	dataStr = strings.TrimPrefix(dataStr, "0x")
	data, decErr := hex.DecodeString(dataStr)
	if decErr != nil || len(data) < 4 {
		return false
	}
	// Check for the Error(string) selector: 0x08c379a0.
	if [4]byte(data[:4]) != errorStringSelector {
		return false
	}
	// ABI-decode the string argument.
	stringType, typeErr := abi.NewType("string", "", nil)
	if typeErr != nil {
		return false
	}
	args := abi.Arguments{{Type: stringType}}
	values, decErr := args.Unpack(data[4:])
	if decErr != nil || len(values) == 0 {
		return false
	}
	decoded, ok := values[0].(string)
	return ok && decoded == reason
}

// hashSliceToByteSlice converts []common.Hash to [][32]byte without copying.
// This is safe because common.Hash is defined as [32]byte, so the memory layout is identical.
func hashSliceToByteSlice(b []common.Hash) [][32]byte {
	return *(*[][32]byte)(unsafe.Pointer(&b))
}
