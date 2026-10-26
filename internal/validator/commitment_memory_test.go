//go:build !race

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCommitmentMemoryIsBounded(t *testing.T) {
	const inputs = 4
	rows := uint64(inputs * 250001)
	if os.Getenv("CARTESI_VALIDATOR_LARGE_MEMORY_TEST") == "1" {
		rows = 42*1000000 + inputs
	}
	r, s, app, epoch := newPagedCommitmentFixture(t, rows, inputs)
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	var peak uint64
	r.observePage = func() {
		if r.pageCalls%20 != 0 {
			return
		}
		runtime.GC()
		var current runtime.MemStats
		runtime.ReadMemStats(&current)
		if current.HeapAlloc > baseline.HeapAlloc {
			peak = max(peak, current.HeapAlloc-baseline.HeapAlloc)
		}
	}
	commitment, proof, err := s.buildCommitment(t.Context(), app, epoch)
	require.NoError(t, err)
	require.NotNil(t, commitment)
	require.True(t, proof.VerifyRoot(*commitment))
	t.Logf("peak live heap growth across %d rows: %d bytes", rows, peak)
	require.Less(t, peak, uint64(32<<20))
	runtime.KeepAlive(r)
}
