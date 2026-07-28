// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
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
	ID      any             `json:"id"`
}

type RPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
	ID      any       `json:"id"`
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
func writeRPCError(w io.Writer, id any, code int, message string) error {
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
	return json.NewEncoder(w).Encode(resp)
}

func writeRPCResult(w io.Writer, id any, result any) error {
	resp := RPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	return json.NewEncoder(w).Encode(resp)
}

// UnmarshalParams supports both by-name (object) and by-position (array) parameter structures.
// If params is an object, it simply does json.Unmarshal; if it's an array, it will attempt
// to unmarshal each positional parameter into the target struct field in declaration order.
func UnmarshalParams(data json.RawMessage, target any) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		// Unmarshal positional parameters into a slice of json.RawMessage.
		var rawParams []json.RawMessage
		if err := json.Unmarshal(data, &rawParams); err != nil {
			return err
		}
		// Use reflection to set values in the target struct in the order they appear.
		val := reflect.ValueOf(target)
		if val.Kind() != reflect.Pointer || val.IsNil() {
			return fmt.Errorf("error unmarshalling positional parameters target must be a non-nil pointer to a struct")
		}
		val = val.Elem()
		if val.Kind() != reflect.Struct {
			return fmt.Errorf("error unmarshalling positional parameters target must point to a struct")
		}
		typ := val.Type()
		if len(rawParams) > typ.NumField() {
			return fmt.Errorf("error unmarshalling positional parameters, expected %d params, got %d",
				typ.NumField(), len(rawParams))
		}
		// For each field in the struct, if a positional parameter exists, unmarshal that parameter.
		for i := 0; i < typ.NumField() && i < len(rawParams); i++ {
			sf := typ.Field(i)
			if sf.Tag.Get("json") == "-" {
				continue
			}
			field := val.Field(i)
			if !field.CanSet() {
				return fmt.Errorf("error unmarshalling positional parameter field %q is not settable", typ.Field(i).Name)
			}
			// Unmarshal the corresponding raw parameter into the field.
			if err := json.Unmarshal(rawParams[i], field.Addr().Interface()); err != nil {
				return fmt.Errorf("error unmarshalling positional parameter %d for field %s: %w", i, typ.Field(i).Name, err)
			}
		}
		return nil
	}
	// Otherwise, assume by-name structure.
	return json.Unmarshal(data, target)
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
