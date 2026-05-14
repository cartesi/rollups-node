// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
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

// revertSelectorLength is the length of a Solidity custom error selector.
const revertSelectorLength = 4

// revertDataBytes extracts the raw revert payload (selector + ABI-encoded
// arguments) carried by a JSON-RPC error. Returns (nil, false) when err
// carries no revert data or the payload is shorter than a selector.
func revertDataBytes(err error) ([]byte, bool) {
	info, ok := ExtractJSONErrorInfo(err)
	if !ok || !info.HasData {
		return nil, false
	}
	var raw []byte
	switch d := info.Data.(type) {
	case string:
		raw = common.FromHex(d)
	case []byte:
		raw = d
	default:
		return nil, false
	}
	if len(raw) < revertSelectorLength {
		return nil, false
	}
	return raw, true
}

// UnpackRevert checks that err carries revert data for the named custom error
// declared in metadata and ABI-decodes its arguments. It returns (values,
// true) when the selector matches and the data decodes cleanly, and
// (nil, false) otherwise. Using the generated ABI metadata is safer than
// reading fixed byte positions by hand, especially if the ABI changes later.
func UnpackRevert(err error, metadata *bind.MetaData, errorName string) ([]any, bool) {
	raw, ok := revertDataBytes(err)
	if !ok || metadata == nil {
		return nil, false
	}
	parsed, _ := metadata.GetAbi()
	if parsed == nil {
		return nil, false
	}
	abiErr, ok := parsed.Errors[errorName]
	if !ok {
		return nil, false
	}
	if !bytes.Equal(raw[:revertSelectorLength], abiErr.ID[:revertSelectorLength]) {
		return nil, false
	}
	values, unpackErr := abiErr.Inputs.Unpack(raw[revertSelectorLength:])
	if unpackErr != nil {
		return nil, false
	}
	return values, true
}

// DescribeRevert decodes the custom-error revert payload carried by err
// against the given contract metadatas and renders it as
// "Name(param=value, ...)". It returns ("", false) when err carries no revert
// data or none of the metadatas declare an error matching the payload's
// selector. When the selector matches but the arguments do not decode, the
// bare error name is returned. Byte values are hex-encoded and string values
// are quoted — revert payloads can carry contract-controlled data, so they
// are kept inert.
func DescribeRevert(err error, metadatas ...*bind.MetaData) (string, bool) {
	raw, ok := revertDataBytes(err)
	if !ok {
		return "", false
	}
	for _, metadata := range metadatas {
		if metadata == nil {
			continue
		}
		parsed, _ := metadata.GetAbi()
		if parsed == nil {
			continue
		}
		for name, abiErr := range parsed.Errors {
			if !bytes.Equal(raw[:revertSelectorLength], abiErr.ID[:revertSelectorLength]) {
				continue
			}
			values, unpackErr := abiErr.Inputs.Unpack(raw[revertSelectorLength:])
			if unpackErr != nil {
				return name, true
			}
			return formatRevert(name, abiErr.Inputs, values), true
		}
	}
	return "", false
}

func formatRevert(name string, inputs abi.Arguments, values []any) string {
	parts := make([]string, 0, len(values))
	for i, v := range values {
		label := ""
		if i < len(inputs) && inputs[i].Name != "" {
			label = inputs[i].Name + "="
		}
		parts = append(parts, label+formatRevertValue(v))
	}
	return fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))
}

func formatRevertValue(v any) string {
	switch val := v.(type) {
	case []byte:
		return fmt.Sprintf("0x%x", val)
	case [32]byte:
		return fmt.Sprintf("0x%x", val)
	case common.Address:
		return val.Hex()
	case common.Hash:
		return val.Hex()
	case string:
		// Contract-controlled text: quote it so escape sequences cannot
		// reach the terminal or logs unescaped.
		return strconv.Quote(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// IsNonceTooLowError reports whether an error returned by a contract-binding
// broadcast (e.g. SubmitClaim, AcceptClaim, Settle, JoinTournament) is the
// JSON-RPC "nonce too low" rejection. This is a transient broadcast-time
// condition: the chain has already mined a tx with this EOA's nonce N, so a
// new broadcast (also using N because bind.TransactOpts.Nonce is nil and the
// binding fetched it via PendingNonceAt) is rejected. The classic trigger is
// a node restart that straddles an in-flight tx: the pre-restart broadcast
// landed, but the post-restart Tick re-derives the same nonce from
// PendingNonceAt before the chain's pending view catches up.
//
// The check is a case-insensitive substring match because the JSON-RPC error
// from the node arrives as an opaque rpc.Error wrapper around the upstream
// string; go-ethereum's core.ErrNonceTooLow sentinel is not propagated
// through eth_sendRawTransaction. Both anvil and geth produce the literal
// "nonce too low" inside the wrapper.
//
// The recommended response is to treat this as a retry-later condition and
// rely on a pre-flight on-chain read (IsEpochSettled, IsCommitmentJoined,
// getClaim, etc.) at the next tick to reconcile against state.
func IsNonceTooLowError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "nonce too low")
}
