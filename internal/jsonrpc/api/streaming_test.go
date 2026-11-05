// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/jsonutil"
	"github.com/cartesi/rollups-node/internal/model"
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
			base, err := json.Marshal(tc.base, jsonutil.Options)
			require.NoError(t, err)
			expected := base
			if tc.decoded != nil {
				decoded, err := json.Marshal(tc.decoded, jsonutil.Options)
				require.NoError(t, err)
				expected = append(expected[:len(expected)-1], []byte(`,"decoded_data":`)...)
				expected = append(expected, decoded...)
				expected = append(expected, '}')
			}
			var got bytes.Buffer
			require.NoError(t, json.MarshalWrite(&got, tc.value, jsonutil.Options))
			require.Equal(t, string(expected), got.String())
			legacy, err := json.Marshal(tc.value, jsonutil.Options)
			require.NoError(t, err)
			require.Equal(t, got.String(), string(legacy))
			require.Implements(t, (*json.MarshalerTo)(nil), tc.value)
		})
	}
}

func BenchmarkDecodedOutputStreaming(b *testing.B) {
	output := &DecodedOutput{Output: &model.Output{RawData: []byte("notice"), OutputHashesSiblings: make([]common.Hash, 63)}, DecodedData: NewNoticeData("0x1234")}
	b.ReportAllocs()
	for b.Loop() {
		if err := json.MarshalWrite(io.Discard, output, jsonutil.Options); err != nil {
			b.Fatal(err)
		}
	}
}
