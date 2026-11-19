// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type inputJSONAlias Input

// InputJSON is the flattened JSON wire representation shared with decoded
// JSON-RPC responses. Obtain it through Input.JSONFields.
type InputJSON struct {
	EpochIndex    string  `json:"epoch_index"`
	Index         string  `json:"index"`
	BlockNumber   string  `json:"block_number"`
	RawData       string  `json:"raw_data"`
	ExceptionData *string `json:"exception_data"`
	LogIndex      string  `json:"log_index"`
	*inputJSONAlias
}

// JSONFields constructs the wire view without serializing it.
func (i *Input) JSONFields() *InputJSON {
	var exceptionData *string
	if i.ExceptionData != nil {
		encoded := hexutil.Encode(i.ExceptionData)
		exceptionData = &encoded
	}
	aux := &InputJSON{
		EpochIndex:     fmt.Sprintf("0x%x", i.EpochIndex),
		Index:          fmt.Sprintf("0x%x", i.Index),
		BlockNumber:    fmt.Sprintf("0x%x", i.BlockNumber),
		RawData:        "0x" + hex.EncodeToString(i.RawData),
		ExceptionData:  exceptionData,
		LogIndex:       fmt.Sprintf("0x%x", i.LogIndex),
		inputJSONAlias: (*inputJSONAlias)(i),
	}
	return aux
}

type outputJSONAlias Output

// OutputJSON is the flattened JSON wire representation shared with decoded
// JSON-RPC responses. Obtain it through Output.JSONFields.
type OutputJSON struct {
	EpochIndex string `json:"epoch_index"`
	InputIndex string `json:"input_index"`
	Index      string `json:"index"`
	RawData    string `json:"raw_data"`
	*outputJSONAlias
}

// JSONFields constructs the wire view without serializing it.
func (o *Output) JSONFields() *OutputJSON {
	aux := &OutputJSON{
		EpochIndex:      fmt.Sprintf("0x%x", o.EpochIndex),
		InputIndex:      fmt.Sprintf("0x%x", o.InputIndex),
		Index:           fmt.Sprintf("0x%x", o.Index),
		RawData:         "0x" + hex.EncodeToString(o.RawData),
		outputJSONAlias: (*outputJSONAlias)(o),
	}
	return aux
}
