// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInputHashCollectionDimensions(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint64(24), Log2MaxAdvanceStatesPerEpoch)
	require.Equal(t, uint64(1)<<24, MaxAdvanceStatesPerEpoch)
	require.Equal(t, uint64(24), Log2InputHashCollectionCapacity)
	require.Equal(t, uint64(1)<<24, InputHashCollectionCapacity)
	require.Equal(t, uint64(48), Log2EpochComputationHashLeafCount)
}

func TestValidateInputHashCollectionSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		hashes  uint64
		padding uint64
		wantErr bool
	}{
		{name: "empty", padding: InputHashCollectionCapacity},
		{name: "partial", hashes: 2, padding: InputHashCollectionCapacity - 2},
		{name: "full", hashes: InputHashCollectionCapacity},
		{name: "short", hashes: 2, padding: InputHashCollectionCapacity - 3, wantErr: true},
		{name: "long", hashes: 2, padding: InputHashCollectionCapacity - 1, wantErr: true},
		{name: "overflow", hashes: InputHashCollectionCapacity + 1, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateInputHashCollectionSpan(test.hashes, test.padding)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
