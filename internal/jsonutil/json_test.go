// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonutil

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWireOptions(t *testing.T) {
	data, err := json.Marshal(map[string]any{"z": "<>&", "m": map[string]int(nil), "a": []int(nil)}, Options)
	require.NoError(t, err)
	require.Equal(t, `{"a":null,"m":null,"z":"\u003c\u003e\u0026"}`, string(data))
	var target struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"COUNT":17}`), &target, Options))
	require.Equal(t, 17, target.Count)
	var out bytes.Buffer
	require.NoError(t, json.MarshalEncode(jsontext.NewEncoder(&out), target, Options))
	require.Equal(t, "{\"count\":17}\n", out.String())
}

func TestIndentPreservesInputAndDoesNotAppendNewline(t *testing.T) {
	src := []byte(`{"a":[1,2],"b":"<text>"}`)
	before := bytes.Clone(src)
	var out bytes.Buffer
	require.NoError(t, Indent(&out, src, "", "  "))
	require.Equal(t, "{\n  \"a\": [\n    1,\n    2\n  ],\n  \"b\": \"<text>\"\n}", out.String())
	require.Equal(t, before, src)
}
