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
	require.Equal(t, []string{
		"NONE",
		"ACCEPTED",
		"REJECTED",
		"EXCEPTION",
		"MACHINE_HALTED",
		"OVERFLOW",
		"UNEXPECTED_YIELD",
	}, completionStatus.Enum)

	var applicationStatus struct {
		Enum []string `json:"enum"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["ApplicationStatus"], &applicationStatus))
	require.Equal(t, []string{
		"OK",
		"FAILED",
		"DIVERGED",
		"CORRUPTED",
		"GUEST_EXCEPTION",
		"MACHINE_HALTED",
		"MCYCLE_OVERFLOW",
		"UNEXPECTED_YIELD",
	}, applicationStatus.Enum)

	var input struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["Input"], &input))
	require.Contains(t, input.Properties, "exception_data")
	require.Contains(t, input.Properties, "tx_buffer_data_block")
	require.NotContains(t, input.Properties, "outputs_hash")

	var epoch struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(spec.Components.Schemas["Epoch"], &epoch))
	for _, field := range []string{
		"tx_buffer_data_block",
		"tx_buffer_proof",
		"iflags_y_data_block",
		"iflags_y_proof",
		"htif_tohost_data_block",
		"htif_tohost_proof",
	} {
		require.Contains(t, epoch.Properties, field)
	}
	require.NotContains(t, epoch.Properties, "outputs_merkle_root")
	require.NotContains(t, epoch.Properties, "outputs_merkle_proof")

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
