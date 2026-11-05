// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/jsonutil"
)

func TestDiscoverySchemaApplicationInputBox(t *testing.T) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)

	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]jsontext.Value `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec, jsonutil.Options))

	properties := spec.Components.Schemas["Application"].Properties
	require.Contains(t, properties, "iinputbox_address")
	require.NotContains(t, properties, "data_availability")
}
