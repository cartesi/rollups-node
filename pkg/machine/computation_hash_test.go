// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

func TestComputationHashDimensions(t *testing.T) {
	require.Equal(t, uint64(20), Log2MaxUarchCyclesPerMCycle)
	require.Equal(t, uint64(48), Log2MaxMCyclesPerAdvanceState)
	require.Equal(t, uint64(24), Log2MaxAdvanceStatesPerEpoch)
	require.Equal(t, uint64(44), Log2UarchCycleComputationHashPeriod)
	require.Equal(t, uint64(68), Log2UarchCyclesPerAdvanceState)
	require.Equal(t, uint64(1)<<20-1, MaxUarchCycle)
	require.Equal(t, uint64(1)<<48-1, MaxMCycleDeltaPerAdvanceState)
	require.Equal(t, uint64(1)<<24-1, MaxAdvanceStateIndexPerEpoch)

	require.Equal(t, uint64(24), Log2MCycleComputationHashPeriod)
	require.Equal(t, uint64(1)<<24, MCycleComputationHashPeriod)
	require.Equal(t, uint64(24), Log2InputEntryCapacity)
	require.Equal(t, uint64(1)<<24, InputEntryCapacity)
	require.Equal(t, uint64(48), Log2EpochComputationHashLeafCount)
	require.Equal(t, uint64(1)<<48, EpochComputationHashLeafCount)
	require.Equal(t, uint64(1)<<24, MaxAdvanceStatesPerEpoch)
	require.Equal(
		t,
		model.Log2InputHashCollectionCapacity,
		Log2MaxMCyclesPerAdvanceState-Log2MCycleComputationHashPeriod,
		"the execution window and sampling period must derive the protocol input hash collection capacity",
	)
}

func TestComputationHashCollectionChunkMatchesEmulatorCLI(t *testing.T) {
	require.Equal(
		t,
		MCycleComputationHashPeriod<<8,
		mcycleComputationHashChunkSize,
		"cartesi-machine 0.21 targets 2^8 returned hashes per collection call",
	)
}
