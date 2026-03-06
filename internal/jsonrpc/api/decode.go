// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// ParseOutputType parses a hex-encoded 4-byte output type selector.
func ParseOutputType(s string) ([]byte, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s) != 8 { //nolint: mnd
		return []byte{}, fmt.Errorf("invalid output type: expected exactly 4 bytes")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return []byte{}, err
	}
	return b, nil
}

// EvmAdvance represents decoded EvmAdvance input data.
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

// DecodedInput extends model.Input with ABI-decoded data.
type DecodedInput struct {
	*model.Input
	DecodedData *EvmAdvance `json:"decoded_data"`
}

// DecodeInput ABI-decodes a raw input into a DecodedInput.
func DecodeInput(input *model.Input, parsedAbi *abi.ABI) (*DecodedInput, error) {
	decoded := make(map[string]any)
	if len(input.RawData) < 4 {
		return &DecodedInput{Input: input}, fmt.Errorf("error: input needs at least 4 bytes")
	}

	method, ok := parsedAbi.Methods["EvmAdvance"]
	if !ok {
		return &DecodedInput{Input: input}, fmt.Errorf("EvmAdvance method not found in ABI")
	}

	if !bytes.Equal(input.RawData[:4], method.ID) {
		return &DecodedInput{Input: input},
			fmt.Errorf("input selector %x does not match EvmAdvance", input.RawData[:4])
	}

	err := method.Inputs.UnpackIntoMap(decoded, input.RawData[4:])
	if err != nil {
		return &DecodedInput{Input: input}, err
	}

	chainId, ok1 := decoded["chainId"].(*big.Int)
	appContract, ok2 := decoded["appContract"].(common.Address)
	msgSender, ok3 := decoded["msgSender"].(common.Address)
	blockNumber, ok4 := decoded["blockNumber"].(*big.Int)
	blockTimestamp, ok5 := decoded["blockTimestamp"].(*big.Int)
	prevRandao, ok6 := decoded["prevRandao"].(*big.Int)
	index, ok7 := decoded["index"].(*big.Int)
	payload, ok8 := decoded["payload"].([]byte)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 {
		return &DecodedInput{Input: input}, fmt.Errorf("unable to decode EvmAdvance parameters")
	}

	evmAdvance := EvmAdvance{
		ChainId:        fmt.Sprintf("0x%x", chainId),
		AppContract:    appContract.Hex(),
		MsgSender:      msgSender.Hex(),
		BlockNumber:    fmt.Sprintf("0x%x", blockNumber),
		BlockTimestamp: fmt.Sprintf("0x%x", blockTimestamp),
		PrevRandao:     fmt.Sprintf("0x%x", prevRandao),
		Index:          fmt.Sprintf("0x%x", index),
		Payload:        "0x" + hex.EncodeToString(payload),
	}

	return &DecodedInput{
		Input:       input,
		DecodedData: &evmAdvance,
	}, nil
}

// MarshalJSON produces a flat JSON object merging Input fields with decoded_data.
func (d *DecodedInput) MarshalJSON() ([]byte, error) {
	inputJSON, err := d.Input.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if d.DecodedData == nil {
		return inputJSON, nil
	}
	return mergeJSONField(inputJSON, "decoded_data", d.DecodedData)
}

// DecodedData is the single canonical type for decoded output data, used by
// both the server (JSON-RPC API) and client (CLI) sides. It captures the
// superset of fields across all output types (Notice, Voucher,
// DelegateCallVoucher). Use the Type field to discriminate:
//
//   - "Notice":               Payload is set.
//   - "Voucher":              Destination, Value, and Payload are set.
//   - "DelegateCallVoucher":  Destination and Payload are set.
//   - Any other value:        RawData contains the hex-encoded raw output.
//
// Use the constructor functions (NewNoticeData, NewVoucherData,
// NewDelegateCallVoucherData, NewUnknownData) to create instances with the
// correct fields populated.
type DecodedData struct {
	Type        string `json:"type"`
	Payload     string `json:"payload,omitempty"`
	Destination string `json:"destination,omitempty"`
	Value       string `json:"value,omitempty"`
	RawData     string `json:"raw_data,omitempty"`
}

// NewNoticeData creates a DecodedData for a Notice output.
func NewNoticeData(payload string) *DecodedData {
	return &DecodedData{Type: "Notice", Payload: payload}
}

// NewVoucherData creates a DecodedData for a Voucher output.
func NewVoucherData(destination, value, payload string) *DecodedData {
	return &DecodedData{
		Type:        "Voucher",
		Destination: destination,
		Value:       value,
		Payload:     payload,
	}
}

// NewDelegateCallVoucherData creates a DecodedData for a DelegateCallVoucher output.
func NewDelegateCallVoucherData(destination, payload string) *DecodedData {
	return &DecodedData{
		Type:        "DelegateCallVoucher",
		Destination: destination,
		Payload:     payload,
	}
}

// NewUnknownData creates a DecodedData for an unrecognized output type.
func NewUnknownData(typeName, rawData string) *DecodedData {
	return &DecodedData{Type: typeName, RawData: rawData}
}

// DecodedOutput extends model.Output with ABI-decoded data.
type DecodedOutput struct {
	*model.Output
	DecodedData *DecodedData `json:"decoded_data"`
}

// DecodeOutput ABI-decodes a raw output into a DecodedOutput.
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
	var result *DecodedData
	switch method.Name {
	case "Notice":
		payload, ok := decoded["payload"].([]byte)
		if !ok {
			return decodedOutput, fmt.Errorf("unable to decode Notice payload")
		}
		result = NewNoticeData("0x" + hex.EncodeToString(payload))
	case "Voucher":
		dest, ok1 := decoded["destination"].(common.Address)
		value, ok2 := decoded["value"].(*big.Int)
		payload, ok3 := decoded["payload"].([]byte)
		if !ok1 || !ok2 || !ok3 {
			return decodedOutput, fmt.Errorf("unable to decode Voucher parameters")
		}
		result = NewVoucherData(
			dest.Hex(),
			fmt.Sprintf("0x%x", value),
			"0x"+hex.EncodeToString(payload),
		)
	case "DelegateCallVoucher":
		dest, ok1 := decoded["destination"].(common.Address)
		payload, ok2 := decoded["payload"].([]byte)
		if !ok1 || !ok2 {
			return decodedOutput, fmt.Errorf("unable to decode DelegateCallVoucher parameters")
		}
		result = NewDelegateCallVoucherData(
			dest.Hex(),
			"0x"+hex.EncodeToString(payload),
		)
	default:
		result = NewUnknownData(method.Name, "0x"+hex.EncodeToString(output.RawData))
	}
	decodedOutput.DecodedData = result
	return decodedOutput, nil
}

// UnmarshalJSON deserializes a DecodedOutput: it delegates base fields to
// model.Output.UnmarshalJSON, then extracts and parses the decoded_data field.
func (d *DecodedOutput) UnmarshalJSON(data []byte) error {
	if d.Output == nil {
		d.Output = new(model.Output)
	}
	if err := d.Output.UnmarshalJSON(data); err != nil {
		return err
	}
	var raw struct {
		DecodedData json.RawMessage `json:"decoded_data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.DecodedData) > 0 && string(raw.DecodedData) != "null" {
		d.DecodedData = new(DecodedData)
		if err := json.Unmarshal(raw.DecodedData, d.DecodedData); err != nil {
			return err
		}
	}
	return nil
}

// MarshalJSON produces a flat JSON object merging Output fields with decoded_data.
func (d *DecodedOutput) MarshalJSON() ([]byte, error) {
	outputJSON, err := d.Output.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if d.DecodedData == nil {
		return outputJSON, nil
	}
	return mergeJSONField(outputJSON, "decoded_data", d.DecodedData)
}

// mergeJSONField appends a key-value pair to an existing JSON object.
// The base must be a valid JSON object (ending with '}'). The value is
// marshaled and appended as a new field.
func mergeJSONField(base []byte, key string, value any) ([]byte, error) {
	if len(base) == 0 || base[len(base)-1] != '}' {
		return nil, fmt.Errorf("mergeJSONField: base is not a valid JSON object")
	}
	fieldJSON, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSuffix(base, []byte("}"))
	var buf bytes.Buffer
	buf.Write(trimmed)
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] != '{' {
		buf.WriteByte(',')
	}
	buf.WriteByte('"')
	buf.WriteString(key)
	buf.WriteString(`":`)
	buf.Write(fieldJSON)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
