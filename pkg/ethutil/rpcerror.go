// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

// JSONRPCInfo contains structured error information extracted from a JSON-RPC error response.
type JSONRPCInfo struct {
	Code    int
	Message string
	Data    any
	HasCode bool
	HasData bool
}

// ExtractJSONErrorInfo attempts to extract JSON-RPC error details from an error.
// It checks for rpc.Error (code + message) and rpc.DataError (error data).
// Returns the extracted info and true if the error contained JSON-RPC details.
func ExtractJSONErrorInfo(err error) (JSONRPCInfo, bool) {
	var out JSONRPCInfo
	if err == nil {
		return out, false
	}

	var e rpc.Error
	if errors.As(err, &e) {
		out.Code = e.ErrorCode()
		out.Message = e.Error()
		out.HasCode = true
	}

	var de rpc.DataError
	if errors.As(err, &de) {
		out.Data = de.ErrorData()
		out.HasData = true
		if !out.HasCode {
			out.Message = de.Error()
		}
	}

	return out, out.HasCode || out.HasData
}

// MatchesSelector checks whether RPC error data starts with the given 4-byte selector.
// Handles varying representations across Ethereum clients: hex strings (with/without
// 0x prefix, any case) and raw []byte.
func MatchesSelector(data any, selector string) bool {
	expected := common.FromHex(selector)
	var got []byte
	switch d := data.(type) {
	case string:
		got = common.FromHex(d)
	case []byte:
		got = d
	default:
		return false
	}
	return len(got) >= len(expected) && bytes.Equal(got[:len(expected)], expected)
}

// IsCustomError checks whether an RPC error is a specific custom Solidity error
// defined in the given contract metadata. It extracts the revert data from the
// error and compares its 4-byte selector against the ABI-derived selector for
// errorName. This is case-insensitive and handles 0x prefix variations.
func IsCustomError(err error, metadata *bind.MetaData, errorName string) bool {
	if metadata == nil {
		return false
	}
	info, ok := ExtractJSONErrorInfo(err)
	if !ok || !info.HasData {
		return false
	}
	parsed, _ := metadata.GetAbi()
	if parsed == nil {
		return false
	}
	abiErr, ok := parsed.Errors[errorName]
	if !ok {
		return false
	}
	selector := fmt.Sprintf("0x%x", abiErr.ID[:4])
	return MatchesSelector(info.Data, selector)
}
