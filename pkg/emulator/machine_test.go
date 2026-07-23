// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mcycleRootHashesResult struct {
	Hashes        []string        `json:"hashes"`
	MCyclePhase   uint64          `json:"mcycle_phase"`
	BreakReason   string          `json:"break_reason"`
	PartialBundle json.RawMessage `json:"partial_bundle,omitempty"`
}

func createLoopMachine(t *testing.T) *Machine {
	t.Helper()

	const ramLength = 4096
	image := make([]byte, ramLength)
	// jal x0, 0 loops forever while allowing mcycle to advance predictably.
	binary.LittleEndian.PutUint32(image, 0x0000006f)
	imagePath := filepath.Join(t.TempDir(), "loop.bin")
	require.NoError(t, os.WriteFile(imagePath, image, 0o600))

	config, err := json.Marshal(map[string]any{
		"ram": map[string]any{
			"length": ramLength,
			"backing_store": map[string]any{
				"data_filename": imagePath,
			},
		},
	})
	require.NoError(t, err)

	machine, err := CreateMachine(string(config), "", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, machine.Destroy())
		machine.Delete()
	})
	return machine
}

func collectMCycleRootHashes(
	t *testing.T,
	machine *Machine,
	target,
	log2Period,
	phase uint64,
	log2Bundle int32,
	partialBundle json.RawMessage,
) mcycleRootHashesResult {
	t.Helper()

	rawResult, err := machine.CollectMCycleRootHashes(target, log2Period, phase, log2Bundle, partialBundle)
	require.NoError(t, err)
	var result mcycleRootHashesResult
	require.NoError(t, json.Unmarshal(rawResult, &result))
	return result
}

func TestCollectMCycleRootHashes_PartitionedBundlingMatchesOneShot(t *testing.T) {
	const (
		mcycleStart = uint64(1)
		mcycleEnd   = uint64(65)
		log2Period  = uint64(2)
		log2Bundle  = int32(2)
	)
	targets := []uint64{2, 7, 19, 34, mcycleEnd}

	oneShotMachine := createLoopMachine(t)
	partitionedMachine := createLoopMachine(t)
	for _, machine := range []*Machine{oneShotMachine, partitionedMachine} {
		breakReason, err := machine.Run(mcycleStart)
		require.NoError(t, err)
		require.Equal(t, BreakReasonReachedTargetMcycle, breakReason)
	}

	oneShot := collectMCycleRootHashes(
		t,
		oneShotMachine,
		mcycleEnd,
		log2Period,
		mcycleStart%(uint64(1)<<log2Period),
		log2Bundle,
		nil,
	)

	partitioned := mcycleRootHashesResult{
		MCyclePhase: mcycleStart % (uint64(1) << log2Period),
	}
	sawPartialBundle := false
	for _, target := range targets {
		result := collectMCycleRootHashes(
			t,
			partitionedMachine,
			target,
			log2Period,
			partitioned.MCyclePhase,
			log2Bundle,
			partitioned.PartialBundle,
		)
		partitioned.Hashes = append(partitioned.Hashes, result.Hashes...)
		partitioned.MCyclePhase = result.MCyclePhase
		partitioned.BreakReason = result.BreakReason
		partitioned.PartialBundle = result.PartialBundle
		sawPartialBundle = sawPartialBundle || len(result.PartialBundle) > 0
	}

	require.True(t, sawPartialBundle, "test partitions must exercise partial-bundle continuation")
	require.Equal(t, oneShot, partitioned)

	oneShotRoot, err := oneShotMachine.GetRootHash()
	require.NoError(t, err)
	partitionedRoot, err := partitionedMachine.GetRootHash()
	require.NoError(t, err)
	require.Equal(t, oneShotRoot, partitionedRoot)
}

func TestCollectUarchCycleRootHashes_AcceptsOpaqueRevertTail(t *testing.T) {
	machine := createLoopMachine(t)
	zeroHash := Hash{}
	encodedZeroHash := base64.StdEncoding.EncodeToString(zeroHash[:])
	tail, err := json.Marshal([]string{encodedZeroHash, encodedZeroHash})
	require.NoError(t, err)

	rawResult, err := machine.CollectUarchCycleRootHashes(1, 0, RevertUarchTail(tail))
	require.NoError(t, err)

	var result struct {
		Hashes      []string `json:"hashes"`
		BreakReason string   `json:"break_reason"`
	}
	require.NoError(t, json.Unmarshal(rawResult, &result))
	require.NotEmpty(t, result.Hashes)
	require.Equal(t, "reached_target_mcycle", result.BreakReason)
}
