// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
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

func TestExtractJSONErrorInfo(t *testing.T) {
	t.Run("NilError", func(t *testing.T) {
		info, ok := ExtractJSONErrorInfo(nil)
		assert.False(t, ok)
		assert.False(t, info.HasCode)
		assert.False(t, info.HasData)
	})

	t.Run("PlainError", func(t *testing.T) {
		_, ok := ExtractJSONErrorInfo(errors.New("plain"))
		assert.False(t, ok)
	})

	t.Run("RPCDataError", func(t *testing.T) {
		err := &rpcDataError{code: 3, msg: "execution reverted", data: "0xdeadbeef"}
		info, ok := ExtractJSONErrorInfo(err)
		assert.True(t, ok)
		assert.True(t, info.HasCode)
		assert.True(t, info.HasData)
		assert.Equal(t, 3, info.Code)
		assert.Equal(t, "0xdeadbeef", info.Data)
	})
}

func TestMatchesSelector(t *testing.T) {
	t.Run("LowercaseHexWithPrefix", func(t *testing.T) {
		assert.True(t, MatchesSelector("0xaabbccdd0000", "0xaabbccdd"))
	})

	t.Run("UppercaseHexWithPrefix", func(t *testing.T) {
		assert.True(t, MatchesSelector("0xAABBCCDD0000", "0xaabbccdd"))
	})

	t.Run("MixedCaseHexWithPrefix", func(t *testing.T) {
		assert.True(t, MatchesSelector("0xAaBbCcDd0000", "0xaabbccdd"))
	})

	t.Run("WithoutPrefix", func(t *testing.T) {
		assert.True(t, MatchesSelector("aabbccdd0000", "0xaabbccdd"))
	})

	t.Run("ExactLength", func(t *testing.T) {
		assert.True(t, MatchesSelector("0xaabbccdd", "0xaabbccdd"))
	})

	t.Run("DifferentSelector", func(t *testing.T) {
		assert.False(t, MatchesSelector("0x11223344", "0xaabbccdd"))
	})

	t.Run("TooShort", func(t *testing.T) {
		assert.False(t, MatchesSelector("0xaabb", "0xaabbccdd"))
	})

	t.Run("EmptyData", func(t *testing.T) {
		assert.False(t, MatchesSelector("", "0xaabbccdd"))
	})

	t.Run("RawBytes", func(t *testing.T) {
		assert.True(t, MatchesSelector([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0x00}, "0xaabbccdd"))
	})

	t.Run("NonStringNonBytes", func(t *testing.T) {
		assert.False(t, MatchesSelector(42, "0xaabbccdd"))
	})

	t.Run("NilData", func(t *testing.T) {
		assert.False(t, MatchesSelector(nil, "0xaabbccdd"))
	})
}

func TestIsCustomError(t *testing.T) {
	t.Run("NilError", func(t *testing.T) {
		assert.False(t, IsCustomError(nil, nil, "Foo"))
	})

	t.Run("NonRPCError", func(t *testing.T) {
		assert.False(t, IsCustomError(errors.New("plain"), nil, "Foo"))
	})

	t.Run("NilMetadata", func(t *testing.T) {
		err := &rpcDataError{code: 3, msg: "revert", data: "0xaabbccdd"}
		assert.False(t, IsCustomError(err, nil, "Foo"))
	})
}
