// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestEpochOutputsRootPresenceAndComparison(t *testing.T) {
	zero := common.Hash{}
	other := common.HexToHash("0xcafe")
	for _, test := range []struct {
		name       string
		declared   *common.Hash
		calculated *common.Hash
		matching   bool
		invalid    bool
	}{
		{name: "neither present"},
		{name: "declared absent", calculated: &zero},
		{name: "calculated absent", declared: &zero},
		{name: "present zero roots", declared: &zero, calculated: &zero, matching: true},
		{name: "matching roots", declared: &other, calculated: &other, matching: true},
		{name: "unequal roots", declared: &other, calculated: &zero, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			epoch := &Epoch{TxBufferDataBlock: test.declared, CalculatedOutputsRoot: test.calculated}
			require.Equal(t, test.matching, epoch.HasMatchingOutputsRoot())
			require.Equal(t, test.invalid, epoch.HasInvalidOutputsRoot())
		})
	}
}

func TestEpochJSONPreservesAbsentCalculatedOutputsRoot(t *testing.T) {
	original := Epoch{}
	encoded, err := json.Marshal(&original)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Contains(t, fields, "calculated_outputs_root")
	require.JSONEq(t, "null", string(fields["calculated_outputs_root"]))
	var decoded Epoch
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Nil(t, decoded.CalculatedOutputsRoot)
	require.False(t, decoded.HasMatchingOutputsRoot())
	require.False(t, decoded.HasInvalidOutputsRoot())
}
