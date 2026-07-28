// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListOutputsParamsOutputTypeSelectors(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected OutputTypeSelectors
	}{
		"single selector": {
			input:    `{"output_type":"0x237a816f"}`,
			expected: OutputTypeSelectors{"0x237a816f"},
		},
		"selector list": {
			input:    `{"output_type":["0x237a816f","0x10321e8b"]}`,
			expected: OutputTypeSelectors{"0x237a816f", "0x10321e8b"},
		},
		"empty list": {
			input:    `{"output_type":[]}`,
			expected: OutputTypeSelectors{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var params ListOutputsParams
			require.NoError(t, json.Unmarshal([]byte(test.input), &params))
			require.NotNil(t, params.OutputType)
			require.Equal(t, test.expected, *params.OutputType)
		})
	}
}

func TestListOutputsParamsExecutedIsOptional(t *testing.T) {
	var omitted ListOutputsParams
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.Nil(t, omitted.Executed)

	var executed ListOutputsParams
	require.NoError(t, json.Unmarshal([]byte(`{"executed":true}`), &executed))
	require.NotNil(t, executed.Executed)
	require.True(t, *executed.Executed)

	var pending ListOutputsParams
	require.NoError(t, json.Unmarshal([]byte(`{"executed":false}`), &pending))
	require.NotNil(t, pending.Executed)
	require.False(t, *pending.Executed)
}
