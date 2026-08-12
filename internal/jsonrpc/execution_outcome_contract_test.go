// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverySchemaExecutionOutcomeContract(t *testing.T) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)

	var spec struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))

	var completionStatus struct {
		Enum []string `json:"enum"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["InputCompletionStatus"], &completionStatus))
	require.Equal(t, []string{"NONE", "ACCEPTED", "REJECTED", "EXCEPTION", "MACHINE_HALTED"}, completionStatus.Enum)

	var input struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["Input"], &input))
	require.Contains(t, input.Properties, "exception_data")

	var executionParameters struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["ExecutionParameters"], &executionParameters))
	require.Contains(t, executionParameters.Properties, "advance_inc_cycles")
	require.Contains(t, executionParameters.Properties, "inspect_inc_cycles")
	require.Contains(t, executionParameters.Properties, "advance_max_cycles")
	require.Contains(t, executionParameters.Properties, "inspect_max_cycles")
	for field, operation := range map[string]string{
		"advance_max_cycles": "advance",
		"inspect_max_cycles": "inspect",
	} {
		var property struct {
			Description string `json:"description"`
		}
		require.NoError(t, json.Unmarshal(executionParameters.Properties[field], &property))
		require.Equal(t,
			"Optional "+operation+
				" execution delta from the starting mcycle in range 0..2^48-1; "+
				"0 imposes no operator cap and the machine's imcyclemax governs",
			property.Description,
		)
	}
}
