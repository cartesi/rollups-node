//go:build !race

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"encoding/binary"
	"os"
	"runtime"
	"testing"

	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const outputMemoryMiB = uint64(1 << 20)

func liveOutputHeap() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func TestOutputsPathMemoryIsBounded(t *testing.T) {
	count := uint64(240_000)
	if os.Getenv("CARTESI_VALIDATOR_LARGE_OUTPUT_MEMORY_TEST") == "1" {
		count = 4_000_000
	}
	service, repo, _ := newOutputsFixture(t, 0, count)
	repo.keepProofs = false
	repo.enforceBatchLimit = false
	baseline := liveOutputHeap()
	var maximum uint64
	var rowReadings [2]int
	batches := 0
	sample := func() {
		heap := liveOutputHeap()
		if heap > baseline {
			maximum = max(maximum, heap-baseline)
		}
	}
	repo.onRow = func(second bool, index uint64, raw []byte) {
		pass := 0
		if second {
			pass = 1
		}
		current := repo.scans[pass][len(repo.scans[pass])-1]
		if index == current.first {
			sample()
			rowReadings[pass]++
		}
		runtime.KeepAlive(raw)
	}
	repo.onBatch = func(proofs []repository.OutputProof) {
		sample()
		// The matrix, leaves and batch are still live at every measurement.
		runtime.KeepAlive(proofs)
		batches++
	}
	_, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
	require.NoError(t, err)
	require.NoError(t, repo.StoreClaimAndProofs(t.Context(), repo.epoch, write))
	final := liveOutputHeap()
	finalGrowth := uint64(0)
	if final > baseline {
		finalGrowth = final - baseline
	}
	runtime.KeepAlive(write)
	runtime.KeepAlive(service)
	expected := len(repo.scans[0])
	require.Equal(t, [2]int{expected, expected}, rowReadings)
	require.Equal(t, expected, batches)
	t.Logf("outputs=%d active_growth=%d final_growth=%d batches=%d readings=%v",
		count, maximum, finalGrowth, batches, rowReadings)
	// Pure fake measurement: 4,372,216 bytes; ceil(1.5 times growth) is 7 MiB.
	assert.LessOrEqual(t, maximum, 7*outputMemoryMiB, "live output batch exceeds its heap bound")
	assert.LessOrEqual(t, finalGrowth, 2*outputMemoryMiB, "completed pass retains epoch-sized data")
}

func TestOutputsPathMemoryLargeOutputs(t *testing.T) {
	service, repo, _ := newOutputsFixture(t, 2048-32, 64)
	repo.keepProofs = false
	repo.raw = func(index uint64) []byte {
		raw := make([]byte, 2*outputMemoryMiB)
		binary.BigEndian.PutUint64(raw, index)
		return raw
	}
	baseline := liveOutputHeap()
	maximum := uint64(0)
	readings := [2]int{}
	repo.onRow = func(second bool, _ uint64, raw []byte) {
		heap := liveOutputHeap()
		if heap > baseline {
			maximum = max(maximum, heap-baseline)
		}
		pass := 0
		if second {
			pass = 1
		}
		readings[pass]++
		runtime.KeepAlive(raw)
	}
	_, write, err := service.computeMerkleTreeAndProofs(t.Context(), repo.app, repo.epoch)
	require.NoError(t, err)
	require.NoError(t, repo.StoreClaimAndProofs(t.Context(), repo.epoch, write))
	require.Equal(t, [2]int{64, 64}, readings)
	require.Len(t, repo.scans[0], 2)
	require.Len(t, repo.scans[1], 2)
	t.Logf("large_outputs active_growth=%d readings=%v", maximum, readings)
	require.LessOrEqual(t, maximum, 16*outputMemoryMiB, "borrowed output rows were retained")
	runtime.KeepAlive(write)
}
