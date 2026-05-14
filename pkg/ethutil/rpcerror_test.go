// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestIsNonceTooLowError pins the substring-match contract used by both the
// claimer and PRT broadcast paths to short-circuit on the JSON-RPC
// "nonce too low" rejection. The classifier must catch the literal anvil/
// geth wording, the wrapped form (`[nonce too low]` produced when a top-level
// formatter renders a []error), and arbitrary case; it must not match
// unrelated errors.
func TestIsNonceTooLowError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "Nil", err: nil, want: false},
		{name: "LiteralLowercase", err: errors.New("nonce too low"), want: true},
		{name: "MixedCase", err: errors.New("Nonce Too Low"), want: true},
		{name: "BracketWrapped", err: errors.New("[nonce too low]"), want: true},
		{
			name: "WrappedWithFmt",
			err:  fmt.Errorf("send transaction: %w", errors.New("nonce too low")),
			want: true,
		},
		{name: "UnrelatedError", err: errors.New("connection refused"), want: false},
		{name: "RevertedError", err: errors.New("execution reverted"), want: false},
		{
			name: "NonceTooHigh",
			err:  errors.New("nonce too high"),
			want: false, // intentional — different condition, not handled here
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsNonceTooLowError(tc.err))
		})
	}
}

// describeTestMetaData declares a small error ABI to exercise DescribeRevert
// without depending on generated contract bindings.
var describeTestMetaData = &bind.MetaData{ABI: `[
	{"type":"error","name":"InsufficientFunds","inputs":[
		{"name":"value","type":"uint256"},{"name":"balance","type":"uint256"}]},
	{"type":"error","name":"AppReverted","inputs":[
		{"name":"data","type":"bytes"}]},
	{"type":"error","name":"Rejected","inputs":[
		{"name":"reason","type":"string"}]},
	{"type":"error","name":"Closed","inputs":[]}
]`}

func describeTestRevert(t *testing.T, name string, args ...any) error {
	t.Helper()
	parsed, err := describeTestMetaData.GetAbi()
	require.NoError(t, err)
	abiErr := parsed.Errors[name]
	packed, err := abiErr.Inputs.Pack(args...)
	require.NoError(t, err)
	payload := append(append([]byte{}, abiErr.ID[:4]...), packed...)
	return &rpcDataError{code: 3, msg: "execution reverted", data: fmt.Sprintf("0x%x", payload)}
}

func TestDescribeRevert(t *testing.T) {
	t.Run("NumericArgs", func(t *testing.T) {
		err := describeTestRevert(t, "InsufficientFunds", big.NewInt(5), big.NewInt(3))
		desc, ok := DescribeRevert(err, describeTestMetaData)
		assert.True(t, ok)
		assert.Equal(t, "InsufficientFunds(value=5, balance=3)", desc)
	})

	t.Run("BytesArgsHexEncoded", func(t *testing.T) {
		err := describeTestRevert(t, "AppReverted", []byte{0xde, 0xad})
		desc, ok := DescribeRevert(err, describeTestMetaData)
		assert.True(t, ok)
		assert.Equal(t, "AppReverted(data=0xdead)", desc)
	})

	t.Run("StringArgsQuoted", func(t *testing.T) {
		// Contract-controlled text must reach the output escaped, never raw —
		// otherwise a malicious contract could inject terminal escape codes.
		err := describeTestRevert(t, "Rejected", "evil\x1b[2Jstring")
		desc, ok := DescribeRevert(err, describeTestMetaData)
		assert.True(t, ok)
		assert.Equal(t, `Rejected(reason="evil\x1b[2Jstring")`, desc)
	})

	t.Run("NoArgs", func(t *testing.T) {
		err := describeTestRevert(t, "Closed")
		desc, ok := DescribeRevert(err, describeTestMetaData)
		assert.True(t, ok)
		assert.Equal(t, "Closed()", desc)
	})

	t.Run("SelectorMatchedButArgsUndecodable", func(t *testing.T) {
		parsed, err := describeTestMetaData.GetAbi()
		require.NoError(t, err)
		// Selector of a parameterized error with a truncated body.
		id := parsed.Errors["InsufficientFunds"].ID
		payload := append([]byte{}, id[:4]...)
		e := &rpcDataError{code: 3, msg: "execution reverted", data: fmt.Sprintf("0x%x", payload)}
		desc, ok := DescribeRevert(e, describeTestMetaData)
		assert.True(t, ok)
		assert.Equal(t, "InsufficientFunds", desc, "bare name when args do not decode")
	})

	t.Run("UnknownSelector", func(t *testing.T) {
		e := &rpcDataError{code: 3, msg: "execution reverted", data: "0xdeadbeef"}
		_, ok := DescribeRevert(e, describeTestMetaData)
		assert.False(t, ok)
	})

	t.Run("NoRevertData", func(t *testing.T) {
		_, ok := DescribeRevert(errors.New("plain"), describeTestMetaData)
		assert.False(t, ok)
	})

	t.Run("NilMetadata", func(t *testing.T) {
		err := describeTestRevert(t, "Closed")
		_, ok := DescribeRevert(err, nil)
		assert.False(t, ok)
	})
}
