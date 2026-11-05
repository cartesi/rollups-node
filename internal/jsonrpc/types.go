// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"regexp"

	"github.com/cartesi/rollups-node/internal/config"
)

// -----------------------------------------------------------------------------
// JSON‑RPC message types
// -----------------------------------------------------------------------------

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      json.RawMessage `json:"id"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return e.Message
}

func newRPCError(code int, message string) error {
	return &RPCError{Code: code, Message: message}
}

// writeRPCError sends a generic error response for internal errors.
func writeRPCError(w io.Writer, id json.RawMessage, code int, message string) error {
	// Hide detailed error info for internal errors.
	if code == JSONRPC_INTERNAL_ERROR {
		message = "Internal server error"
	}
	resp := RPCResponse{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
	return writeRPCResponse(w, resp)
}

func writeRPCResult(w io.Writer, id json.RawMessage, result any) error {
	resp := RPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	return writeRPCResponse(w, resp)
}

// MarshalWrite streams composite values, allowing the response budget to stop
// encoding before the entire result has been serialized. Preserve v1 wire
// semantics and Encode's trailing newline. Response models use MarshalJSONTo
// to stream their fields without constructing intermediate JSON byte slices.
func writeRPCResponse(w io.Writer, resp RPCResponse) error {
	if err := jsonv2.MarshalWrite(w, resp, json.DefaultOptionsV1()); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// -----------------------------------------------------------------------------
// Validation helpers (server-only)
// -----------------------------------------------------------------------------

var hexAddressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var appNameRegex = regexp.MustCompile(`^[a-z0-9_-]{3,}$`)

func validateNameOrAddress(nameOrAddress string) error {
	if hexAddressRegex.MatchString(nameOrAddress) {
		_, err := config.ToAddressFromString(nameOrAddress)
		return err
	}
	if appNameRegex.MatchString(nameOrAddress) {
		return nil
	}
	return fmt.Errorf("invalid application name")
}
