// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"reflect"
	"regexp"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
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
	Data    any    `json:"data,omitempty"`
}

// writeRPCError sends a generic error response for internal errors.
func writeRPCError(w http.ResponseWriter, id any, code int, message string, data any) {
	// Hide detailed error info for internal errors.
	if code == JSONRPC_INTERNAL_ERROR {
		message = "Internal server error"
		data = nil
	}
	resp := RPCResponse{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeRPCResult(w http.ResponseWriter, id any, result any) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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
// Parameter and Result types (API)
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

// Pagination represents common pagination structure used in responses
type Pagination struct {
	TotalCount uint64 `json:"total_count"`
	Limit      uint64 `json:"limit"`
	Offset     uint64 `json:"offset"`
}

// ListApplicationsParams aligns with the OpenRPC specification
type ListApplicationsParams struct {
	Limit      uint64 `json:"limit"`
	Offset     uint64 `json:"offset"`
	Descending bool   `json:"descending,omitempty"`
}

// GetApplicationParams aligns with the OpenRPC specification
type GetApplicationParams struct {
	Application string `json:"application"`
}

// ListEpochsParams aligns with the OpenRPC specification
type ListEpochsParams struct {
	Application string  `json:"application"`
	Status      *string `json:"status,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
}

// GetEpochParams aligns with the OpenRPC specification
type GetEpochParams struct {
	Application string `json:"application"`
	EpochIndex  string `json:"epoch_index"`
}

// GetLastAcceptedEpochIndexParams with the OpenRPC specification
type GetLastAcceptedEpochIndexParams struct {
	Application string `json:"application"`
}

// ListInputsParams aligns with the OpenRPC specification
type ListInputsParams struct {
	Application string  `json:"application"`
	EpochIndex  *string `json:"epoch_index,omitempty"`
	Sender      *string `json:"sender,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
}

// GetInputParams aligns with the OpenRPC specification
type GetInputParams struct {
	Application string `json:"application"`
	InputIndex  string `json:"input_index"`
}

// GetProcessedInputCountParams aligns with the OpenRPC specification
type GetProcessedInputCountParams struct {
	Application string `json:"application"`
}

// ListOutputsParams aligns with the OpenRPC specification
type ListOutputsParams struct {
	Application    string  `json:"application"`
	EpochIndex     *string `json:"epoch_index,omitempty"`
	InputIndex     *string `json:"input_index,omitempty"`
	OutputType     *string `json:"output_type,omitempty"`
	VoucherAddress *string `json:"voucher_address,omitempty"`
	Limit          uint64  `json:"limit"`
	Offset         uint64  `json:"offset"`
	Descending     bool    `json:"descending,omitempty"`
}

// GetOutputParams aligns with the OpenRPC specification
type GetOutputParams struct {
	Application string `json:"application"`
	OutputIndex string `json:"output_index"`
}

// ListReportsParams aligns with the OpenRPC specification
type ListReportsParams struct {
	Application string  `json:"application"`
	EpochIndex  *string `json:"epoch_index,omitempty"`
	InputIndex  *string `json:"input_index,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
}

// GetReportParams aligns with the OpenRPC specification
type GetReportParams struct {
	Application string `json:"application"`
	ReportIndex string `json:"report_index"`
}

// ListTournamentsParams aligns with the OpenRPC specification
type ListTournamentsParams struct {
	Application string  `json:"application"`
	EpochIndex  *string `json:"epoch_index,omitempty"`
	Level       *string `json:"level,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
}

// GetTournamentParams aligns with the OpenRPC specification
type GetTournamentParams struct {
	Application string `json:"application"`
	Address     string `json:"address"`
}

// -----------------------------------------------------------------------------
// ABI Decoding helpers (provided code)
// -----------------------------------------------------------------------------

type EvmAdvance struct {
	ChainId        string `json:"chain_id"`
	AppContract    string `json:"application_contract"`
	MsgSender      string `json:"sender"`
	BlockNumber    string `json:"block_number"`
	BlockTimestamp string `json:"block_timestamp"`
	PrevRandao     string `json:"prev_randao"`
	Index          string `json:"index"`
	Payload        string `json:"payload"`
}

type DecodedInput struct {
	*model.Input
	DecodedData *EvmAdvance `json:"decoded_data"`
}

func DecodeInput(input *model.Input, parsedAbi *abi.ABI) (*DecodedInput, error) {
	decoded := make(map[string]any)
	err := parsedAbi.Methods["EvmAdvance"].Inputs.UnpackIntoMap(decoded, input.RawData[4:])
	if err != nil {
		return &DecodedInput{Input: input}, err
	}

	evmAdvance := EvmAdvance{
		ChainId:        fmt.Sprintf("0x%x", decoded["chainId"].(*big.Int)),
		AppContract:    decoded["appContract"].(common.Address).Hex(),
		MsgSender:      decoded["msgSender"].(common.Address).Hex(),
		BlockNumber:    fmt.Sprintf("0x%x", decoded["blockNumber"].(*big.Int)),
		BlockTimestamp: fmt.Sprintf("0x%x", decoded["blockTimestamp"].(*big.Int)),
		PrevRandao:     fmt.Sprintf("0x%x", decoded["prevRandao"].(*big.Int)),
		Index:          fmt.Sprintf("0x%x", decoded["index"].(*big.Int)),
		Payload:        "0x" + hex.EncodeToString(decoded["payload"].([]byte)),
	}

	return &DecodedInput{
		Input:       input,
		DecodedData: &evmAdvance,
	}, nil
}

func (d *DecodedInput) MarshalJSON() ([]byte, error) {
	// Marshal the underlying Input using its custom MarshalJSON.
	inputJSON, err := d.Input.MarshalJSON()
	if err != nil {
		return nil, err
	}
	// Ensure inputJSON is a valid JSON object.
	if len(inputJSON) == 0 || inputJSON[len(inputJSON)-1] != '}' {
		return nil, fmt.Errorf("unexpected format from Input.MarshalJSON")
	}

	if d.DecodedData == nil {
		return inputJSON, nil
	}
	// Marshal the DecodedData field.
	decodedDataJSON, err := json.Marshal(d.DecodedData)
	if err != nil {
		return nil, err
	}
	// Use a bytes.Buffer to build the final JSON.
	var buf bytes.Buffer
	buf.Write(bytes.TrimSuffix(inputJSON, []byte("}")))
	buf.WriteString(`,"decoded_data":`)
	buf.Write(decodedDataJSON)
	buf.WriteByte('}')

	return buf.Bytes(), nil
}

type Notice struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type Voucher struct {
	Type        string `json:"type"`
	Destination string `json:"destination"`
	Value       string `json:"value"`
	Payload     string `json:"payload"`
}

type DelegateCallVoucher struct {
	Type        string `json:"type"`
	Destination string `json:"destination"`
	Payload     string `json:"payload"`
}

type DecodedOutput struct {
	*model.Output
	DecodedData any `json:"decoded_data"`
}

func DecodeOutput(output *model.Output, parsedAbi *abi.ABI) (*DecodedOutput, error) {
	decodedOutput := &DecodedOutput{Output: output}
	if len(output.RawData) < 4 {
		return decodedOutput, fmt.Errorf("raw data too short")
	}
	method, err := parsedAbi.MethodById(output.RawData[:4])
	if err != nil {
		return decodedOutput, err
	}
	decoded := make(map[string]any)
	if err := method.Inputs.UnpackIntoMap(decoded, output.RawData[4:]); err != nil {
		return decodedOutput, fmt.Errorf("failed to unpack %s: %w", method.Name, err)
	}
	var result any
	switch method.Name {
	case "Notice":
		payload, ok := decoded["payload"].([]byte)
		if !ok {
			return decodedOutput, fmt.Errorf("unable to decode Notice payload")
		}
		result = Notice{
			Type:    "Notice",
			Payload: "0x" + hex.EncodeToString(payload),
		}
	case "Voucher":
		dest, ok1 := decoded["destination"].(common.Address)
		value, ok2 := decoded["value"].(*big.Int)
		payload, ok3 := decoded["payload"].([]byte)
		if !ok1 || !ok2 || !ok3 {
			return decodedOutput, fmt.Errorf("unable to decode Voucher parameters")
		}
		result = Voucher{
			Type:        "Voucher",
			Destination: dest.Hex(),
			Value:       fmt.Sprintf("0x%x", value),
			Payload:     "0x" + hex.EncodeToString(payload),
		}
	case "DelegateCallVoucher":
		dest, ok1 := decoded["destination"].(common.Address)
		payload, ok2 := decoded["payload"].([]byte)
		if !ok1 || !ok2 {
			return decodedOutput, fmt.Errorf("unable to decode DelegateCallVoucher parameters")
		}
		result = DelegateCallVoucher{
			Type:        "DelegateCallVoucher",
			Destination: dest.Hex(),
			Payload:     "0x" + hex.EncodeToString(payload),
		}
	default:
		result = map[string]any{
			"type":    method.Name,
			"rawData": "0x" + hex.EncodeToString(output.RawData),
		}
	}
	decodedOutput.DecodedData = result
	return decodedOutput, nil
}

func (d *DecodedOutput) MarshalJSON() ([]byte, error) {
	// Marshal the underlying Output using its custom MarshalJSON.
	outputJSON, err := d.Output.MarshalJSON()
	if err != nil {
		return nil, err
	}
	// Ensure outputJSON is a valid JSON object.
	if len(outputJSON) == 0 || outputJSON[len(outputJSON)-1] != '}' {
		return nil, fmt.Errorf("unexpected format from Output.MarshalJSON")
	}

	if d.DecodedData == nil {
		return outputJSON, nil
	}
	// Marshal the DecodedData field.
	decodedDataJSON, err := json.Marshal(d.DecodedData)
	if err != nil {
		return nil, err
	}
	// Use a bytes.Buffer to build the final JSON.
	var buf bytes.Buffer
	buf.Write(bytes.TrimSuffix(outputJSON, []byte("}")))
	buf.WriteString(`,"decoded_data":`)
	buf.Write(decodedDataJSON)
	buf.WriteByte('}')

	return buf.Bytes(), nil
}
