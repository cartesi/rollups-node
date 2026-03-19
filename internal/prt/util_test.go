// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// rpcDataError simulates an RPC error with revert data, as returned by
// eth_estimateGas when a contract reverts.
type rpcDataError struct {
	code int
	msg  string
	data any
}

func (e *rpcDataError) Error() string  { return e.msg }
func (e *rpcDataError) ErrorCode() int { return e.code }
func (e *rpcDataError) ErrorData() any { return e.data }

// buildErrorStringRevert builds the hex-encoded revert data for
// Solidity's Error(string) — the encoding used by require(cond, "reason").
func buildErrorStringRevert(reason string) string {
	// Error(string) selector: 0x08c379a0
	selector := []byte{0x08, 0xc3, 0x79, 0xa0}
	// ABI encoding: offset (0x20) + length + padded string bytes
	offset := make([]byte, 32)
	offset[31] = 0x20
	length := make([]byte, 32)
	length[31] = byte(len(reason))
	// Pad string data to 32-byte boundary.
	data := []byte(reason)
	if rem := len(data) % 32; rem != 0 {
		data = append(data, make([]byte, 32-rem)...)
	}
	var buf []byte
	buf = append(buf, selector...)
	buf = append(buf, offset...)
	buf = append(buf, length...)
	buf = append(buf, data...)
	return "0x" + hex.EncodeToString(buf)
}

func TestIsRevertReason(t *testing.T) {
	t.Run("MatchesExactReason", func(t *testing.T) {
		data := buildErrorStringRevert("clock is initialized")
		err := &rpcDataError{code: 3, msg: "execution reverted", data: data}
		assert.True(t, isRevertReason(err, "clock is initialized"))
	})

	t.Run("DoesNotMatchDifferentReason", func(t *testing.T) {
		data := buildErrorStringRevert("clock is initialized")
		err := &rpcDataError{code: 3, msg: "execution reverted", data: data}
		assert.False(t, isRevertReason(err, "something else"))
	})

	t.Run("DoesNotMatchSubstring", func(t *testing.T) {
		data := buildErrorStringRevert("clock is initialized and running")
		err := &rpcDataError{code: 3, msg: "execution reverted", data: data}
		assert.False(t, isRevertReason(err, "clock is initialized"))
	})

	t.Run("ReturnsFalseForNilError", func(t *testing.T) {
		assert.False(t, isRevertReason(nil, "clock is initialized"))
	})

	t.Run("ReturnsFalseForNonRPCError", func(t *testing.T) {
		err := errors.New("clock is initialized")
		assert.False(t, isRevertReason(err, "clock is initialized"))
	})

	t.Run("ReturnsFalseForCustomErrorSelector", func(t *testing.T) {
		// Custom error selector (not Error(string))
		err := &rpcDataError{code: 3, msg: "execution reverted", data: "0xb3045ef8"}
		assert.False(t, isRevertReason(err, "clock is initialized"))
	})

	t.Run("HandlesWithout0xPrefix", func(t *testing.T) {
		data := buildErrorStringRevert("clock is initialized")
		// Strip the "0x" prefix — some RPC providers omit it.
		err := &rpcDataError{code: 3, msg: "execution reverted", data: data[2:]}
		assert.True(t, isRevertReason(err, "clock is initialized"))
	})
}
