// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execution

import (
	"bytes"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

func TestCycleMaximumParameters(t *testing.T) {
	params := &model.ExecutionParameters{}
	require.NoError(t, setParameterValue(params, "advance_max_cycles", "123"))
	require.NoError(t, setParameterValue(params, "inspect_max_cycles", "456"))
	value, err := getParameterValue(params, "advance_max_cycles")
	require.NoError(t, err)
	require.Equal(t, "123", value)
	value, err = getParameterValue(params, "inspect_max_cycles")
	require.NoError(t, err)
	require.Equal(t, "456", value)
}

func TestWriteParametersIncludesCycleMaximums(t *testing.T) {
	params := &model.ExecutionParameters{
		AdvanceIncCycles: 11, AdvanceMaxCycles: 12,
		InspectIncCycles: 21, InspectMaxCycles: 22,
	}
	var output bytes.Buffer
	writeParameters(&output, params)

	require.Contains(t, output.String(), "advance_inc_cycles: 11")
	require.Contains(t, output.String(), "advance_max_cycles: 12")
	require.Contains(t, output.String(), "inspect_inc_cycles: 21")
	require.Contains(t, output.String(), "inspect_max_cycles: 22")
}
