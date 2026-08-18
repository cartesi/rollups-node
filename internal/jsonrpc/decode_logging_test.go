// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	contractinputs "github.com/cartesi/rollups-node/pkg/contracts/inputs"
	contractoutputs "github.com/cartesi/rollups-node/pkg/contracts/outputs"

	"github.com/stretchr/testify/require"
)

func TestDecodeFailuresAreAggregatedInLogs(t *testing.T) {
	inputABI, err := contractinputs.InputsMetaData.GetAbi()
	require.NoError(t, err)
	outputABI, err := contractoutputs.OutputsMetaData.GetAbi()
	require.NoError(t, err)

	var logs bytes.Buffer
	s := newBatchTestService()
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s.inputABI = inputABI
	s.outputABI = outputABI

	decodedInputs := s.decodeInputs("app", []*model.Input{
		{Index: 7, RawData: nil},
		{Index: 9, RawData: []byte{0x01}},
	})
	decodedOutputs := s.decodeOutputs("app", []*model.Output{
		{Index: 11, RawData: nil},
		{Index: 13, RawData: []byte{0x01}},
	})
	require.Len(t, decodedInputs, 2, "malformed rows must remain in the response")
	require.Len(t, decodedOutputs, 2, "malformed rows must remain in the response")

	debugCount := 0
	warns := map[string]map[string]any{}
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		switch record["level"] {
		case "DEBUG":
			debugCount++
		case "WARN":
			message, _ := record["msg"].(string)
			warns[message] = record
		case "ERROR":
			t.Fatalf("decode failure was logged at Error: %s", line)
		}
	}

	require.Equal(t, 4, debugCount, "each malformed row needs one diagnostic Debug log")
	require.Len(t, warns, 2, "inputs and outputs each need one aggregate warning")
	require.Equal(t, float64(2), warns["Unable to decode Inputs"]["count"])
	require.Equal(t, float64(7), warns["Unable to decode Inputs"]["first_index"])
	require.Equal(t, float64(2), warns["Unable to decode Outputs"]["count"])
	require.Equal(t, float64(11), warns["Unable to decode Outputs"]["first_index"])
}
