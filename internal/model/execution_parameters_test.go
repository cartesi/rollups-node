// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecutionParametersJSONRoundtrip(t *testing.T) {
	original := ExecutionParameters{
		SnapshotPolicy:        SnapshotPolicy_EveryInput,
		AdvanceIncCycles:      101,
		AdvanceMaxCycles:      111,
		InspectIncCycles:      202,
		InspectMaxCycles:      222,
		AdvanceIncDeadline:    3 * time.Second,
		AdvanceMaxDeadline:    4 * time.Second,
		InspectIncDeadline:    5 * time.Second,
		InspectMaxDeadline:    6 * time.Second,
		LoadDeadline:          7 * time.Second,
		StoreDeadline:         8 * time.Second,
		FastDeadline:          9 * time.Second,
		MaxConcurrentInspects: 10,
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)
	require.Contains(t, string(data), `"advance_inc_cycles":"0x65"`)
	require.Contains(t, string(data), `"advance_max_cycles":"0x6f"`)
	require.Contains(t, string(data), `"inspect_inc_cycles":"0xca"`)
	require.Contains(t, string(data), `"inspect_max_cycles":"0xde"`)

	var decoded ExecutionParameters
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original.SnapshotPolicy, decoded.SnapshotPolicy)
	require.Equal(t, original.AdvanceIncCycles, decoded.AdvanceIncCycles)
	require.Equal(t, original.AdvanceMaxCycles, decoded.AdvanceMaxCycles)
	require.Equal(t, original.InspectIncCycles, decoded.InspectIncCycles)
	require.Equal(t, original.InspectMaxCycles, decoded.InspectMaxCycles)
	require.Equal(t, original.AdvanceIncDeadline, decoded.AdvanceIncDeadline)
	require.Equal(t, original.AdvanceMaxDeadline, decoded.AdvanceMaxDeadline)
	require.Equal(t, original.InspectIncDeadline, decoded.InspectIncDeadline)
	require.Equal(t, original.InspectMaxDeadline, decoded.InspectMaxDeadline)
	require.Equal(t, original.LoadDeadline, decoded.LoadDeadline)
	require.Equal(t, original.StoreDeadline, decoded.StoreDeadline)
	require.Equal(t, original.FastDeadline, decoded.FastDeadline)
	require.Equal(t, original.MaxConcurrentInspects, decoded.MaxConcurrentInspects)
}

func TestExecutionParametersZeroCycleMaximumsRoundtrip(t *testing.T) {
	data, err := json.Marshal(&ExecutionParameters{})
	require.NoError(t, err)
	require.Contains(t, string(data), `"advance_max_cycles":"0x0"`)
	require.Contains(t, string(data), `"inspect_max_cycles":"0x0"`)

	var decoded ExecutionParameters
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Zero(t, decoded.AdvanceMaxCycles)
	require.Zero(t, decoded.InspectMaxCycles)
}

func TestExecutionParametersCycleMaximumBounds(t *testing.T) {
	for _, value := range []uint64{0, 1, MaxExecutionCycleSpan} {
		params := ExecutionParameters{
			SnapshotPolicy:   SnapshotPolicy_None,
			AdvanceIncCycles: 1,
			AdvanceMaxCycles: value,
			InspectIncCycles: 1,
			InspectMaxCycles: value,
		}
		require.NoError(t, params.Validate())
	}

	for _, test := range []struct {
		name   string
		mutate func(*ExecutionParameters)
	}{
		{"advance", func(p *ExecutionParameters) { p.AdvanceMaxCycles = MaxExecutionCycles }},
		{"inspect", func(p *ExecutionParameters) { p.InspectMaxCycles = MaxExecutionCycles }},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := ExecutionParameters{
				SnapshotPolicy:   SnapshotPolicy_None,
				AdvanceIncCycles: 1,
				InspectIncCycles: 1,
			}
			test.mutate(&params)
			require.ErrorContains(t, params.Validate(), "must be between 0 and 281474976710655")
		})
	}
}

func TestExecutionParametersRejectZeroCycleIncrements(t *testing.T) {
	for _, test := range []struct {
		name   string
		field  string
		mutate func(*ExecutionParameters)
	}{
		{
			name:   "advance",
			field:  "advance_inc_cycles",
			mutate: func(p *ExecutionParameters) { p.AdvanceIncCycles = 0 },
		},
		{
			name:   "inspect",
			field:  "inspect_inc_cycles",
			mutate: func(p *ExecutionParameters) { p.InspectIncCycles = 0 },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := ExecutionParameters{
				SnapshotPolicy:   SnapshotPolicy_None,
				AdvanceIncCycles: 1,
				InspectIncCycles: 1,
			}
			test.mutate(&params)

			require.ErrorContains(t, params.Validate(), test.field)
		})
	}
}

func TestInputCompletionStatusContract(t *testing.T) {
	expected := []InputCompletionStatus{
		InputCompletionStatus_None,
		InputCompletionStatus_Accepted,
		InputCompletionStatus_Rejected,
		InputCompletionStatus_Exception,
		InputCompletionStatus_MachineHalted,
		InputCompletionStatus_Overflow,
		InputCompletionStatus_UnexpectedYield,
	}
	require.Equal(t, expected, InputCompletionStatusAllValues)

	for _, value := range expected {
		t.Run(value.String(), func(t *testing.T) {
			var fromString InputCompletionStatus
			require.NoError(t, fromString.Scan(value.String()))
			require.Equal(t, value, fromString)

			var fromBytes InputCompletionStatus
			require.NoError(t, fromBytes.Scan([]byte(value.String())))
			require.Equal(t, value, fromBytes)
			require.Equal(t, value != InputCompletionStatus_None, value.IsCompleted())
			require.Equal(t,
				value == InputCompletionStatus_Exception ||
					value == InputCompletionStatus_MachineHalted ||
					value == InputCompletionStatus_Overflow ||
					value == InputCompletionStatus_UnexpectedYield,
				value.IsTerminal(),
			)
		})
	}

	removedOrInvalid := []string{
		"OUTPUTS_LIMIT_EXCEEDED",
		"REPORTS_LIMIT_EXCEEDED",
		"CYCLE_LIMIT_EXCEEDED",
		"TIME_LIMIT_EXCEEDED",
		"PAYLOAD_LENGTH_LIMIT_EXCEEDED",
		"INVALID",
	}
	for _, value := range removedOrInvalid {
		t.Run(value, func(t *testing.T) {
			var status InputCompletionStatus
			require.Error(t, status.Scan(value))
			require.False(t, InputCompletionStatus(value).IsCompleted())
			require.False(t, InputCompletionStatus(value).IsTerminal())
		})
	}
}

func TestApplicationStatusContract(t *testing.T) {
	expected := []ApplicationStatus{
		ApplicationStatus_OK,
		ApplicationStatus_Failed,
		ApplicationStatus_Diverged,
		ApplicationStatus_Corrupted,
		ApplicationStatus_GuestException,
		ApplicationStatus_MachineHalted,
		ApplicationStatus_McycleOverflow,
		ApplicationStatus_UnexpectedYield,
	}
	require.Equal(t, expected, ApplicationStatusAllValues)

	for _, value := range expected {
		t.Run(value.String(), func(t *testing.T) {
			var scanned ApplicationStatus
			require.NoError(t, scanned.Scan(value.String()))
			require.Equal(t, value, scanned)
			require.Equal(t,
				value != ApplicationStatus_OK && value != ApplicationStatus_Failed,
				value.IsTerminal(),
			)
		})
	}
}

func TestTerminalApplicationStatus(t *testing.T) {
	expected := map[InputCompletionStatus]ApplicationStatus{
		InputCompletionStatus_Exception:       ApplicationStatus_GuestException,
		InputCompletionStatus_MachineHalted:   ApplicationStatus_MachineHalted,
		InputCompletionStatus_Overflow:        ApplicationStatus_McycleOverflow,
		InputCompletionStatus_UnexpectedYield: ApplicationStatus_UnexpectedYield,
	}
	for inputStatus, applicationStatus := range expected {
		got, terminal := inputStatus.TerminalApplicationStatus()
		require.True(t, terminal)
		require.Equal(t, applicationStatus, got)
	}
	for _, inputStatus := range []InputCompletionStatus{
		InputCompletionStatus_None,
		InputCompletionStatus_Accepted,
		InputCompletionStatus_Rejected,
	} {
		got, terminal := inputStatus.TerminalApplicationStatus()
		require.False(t, terminal)
		require.Empty(t, got)
	}
}
