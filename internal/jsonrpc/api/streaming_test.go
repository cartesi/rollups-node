// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"io"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestDecodedStreamingWireFormat(t *testing.T) {
	input := &model.Input{Index: 17, RawData: []byte("<input>"), ExceptionData: []byte("exception")}
	output := &model.Output{Index: 23, RawData: []byte("<output>"), OutputHashesSiblings: []common.Hash{common.HexToHash("0x01")}}
	for _, tc := range []struct {
		name    string
		base    any
		decoded any
		value   any
	}{
		{"input", input, &EvmAdvance{Payload: "<>&雪"}, &DecodedInput{Input: input, DecodedData: &EvmAdvance{Payload: "<>&雪"}}},
		{"input without decoded data", input, nil, &DecodedInput{Input: input}},
		{"output", output, NewNoticeData("0x1234"), &DecodedOutput{Output: output, DecodedData: NewNoticeData("0x1234")}},
		{"output without decoded data", output, nil, &DecodedOutput{Output: output}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := json.Marshal(tc.base)
			require.NoError(t, err)
			expected := base
			if tc.decoded != nil {
				decoded, err := json.Marshal(tc.decoded)
				require.NoError(t, err)
				expected = append(expected[:len(expected)-1], []byte(`,"decoded_data":`)...)
				expected = append(expected, decoded...)
				expected = append(expected, '}')
			}
			var got bytes.Buffer
			require.NoError(t, jsonv2.MarshalWrite(&got, tc.value, json.DefaultOptionsV1()))
			require.Equal(t, string(expected), got.String())
			legacy, err := json.Marshal(tc.value)
			require.NoError(t, err)
			require.Equal(t, got.String(), string(legacy))
			require.Implements(t, (*jsonv2.MarshalerTo)(nil), tc.value)
		})
	}
}

func BenchmarkDecodedOutputStreaming(b *testing.B) {
	output := &DecodedOutput{Output: &model.Output{RawData: []byte("notice"), OutputHashesSiblings: make([]common.Hash, 63)}, DecodedData: NewNoticeData("0x1234")}
	b.ReportAllocs()
	for b.Loop() {
		if err := jsonv2.MarshalWrite(io.Discard, output, json.DefaultOptionsV1()); err != nil {
			b.Fatal(err)
		}
	}
}
