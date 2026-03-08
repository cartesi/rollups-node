// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBlockRange(t *testing.T) {
	cc := &chainClient{blockNum: 1000}

	// Ensure globals are reset even if a subtest panics.
	t.Cleanup(func() {
		epochFromBlock = -1
		epochToBlock = -1
	})

	t.Run("defaults to deploy and pinned block", func(t *testing.T) {
		epochFromBlock = -1
		epochToBlock = -1
		from, to, err := cc.resolveBlockRange(500)
		require.NoError(t, err)
		assert.Equal(t, uint64(500), from)
		assert.Equal(t, uint64(1000), to)
	})

	t.Run("respects from-block and to-block flags", func(t *testing.T) {
		epochFromBlock = 600
		epochToBlock = 800
		from, to, err := cc.resolveBlockRange(500)
		require.NoError(t, err)
		assert.Equal(t, uint64(600), from)
		assert.Equal(t, uint64(800), to)
	})

	t.Run("from-block exceeds pinned block", func(t *testing.T) {
		epochFromBlock = 2000
		epochToBlock = -1
		_, _, err := cc.resolveBlockRange(500)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--from-block 2000 exceeds pinned block 1000")
	})

	t.Run("from-block exceeds to-block", func(t *testing.T) {
		epochFromBlock = 800
		epochToBlock = 700
		_, _, err := cc.resolveBlockRange(500)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--from-block 800 exceeds --to-block 700")
	})
}

func TestMergeClaimEvents(t *testing.T) {
	t.Run("both empty", func(t *testing.T) {
		result := mergeClaimEvents(nil, nil)
		assert.Empty(t, result)
	})

	t.Run("a empty", func(t *testing.T) {
		b := []ClaimEvent{
			{EventType: "ClaimAccepted", BlockNumber: 100},
			{EventType: "ClaimAccepted", BlockNumber: 200},
		}
		result := mergeClaimEvents(nil, b)
		assert.Equal(t, b, result)
	})

	t.Run("b empty", func(t *testing.T) {
		a := []ClaimEvent{
			{EventType: "ClaimSubmitted", BlockNumber: 50},
		}
		result := mergeClaimEvents(a, nil)
		assert.Equal(t, a, result)
	})

	t.Run("interleaved", func(t *testing.T) {
		a := []ClaimEvent{
			{EventType: "ClaimSubmitted", BlockNumber: 10},
			{EventType: "ClaimSubmitted", BlockNumber: 30},
			{EventType: "ClaimSubmitted", BlockNumber: 50},
		}
		b := []ClaimEvent{
			{EventType: "ClaimAccepted", BlockNumber: 20},
			{EventType: "ClaimAccepted", BlockNumber: 40},
		}
		result := mergeClaimEvents(a, b)
		assert.Len(t, result, 5)

		// Verify sorted order.
		for i := 1; i < len(result); i++ {
			assert.LessOrEqual(t, result[i-1].BlockNumber, result[i].BlockNumber)
		}

		// Verify types in expected order.
		assert.Equal(t, "ClaimSubmitted", result[0].EventType)
		assert.Equal(t, "ClaimAccepted", result[1].EventType)
		assert.Equal(t, "ClaimSubmitted", result[2].EventType)
		assert.Equal(t, "ClaimAccepted", result[3].EventType)
		assert.Equal(t, "ClaimSubmitted", result[4].EventType)
	})

	t.Run("same block number preserves order", func(t *testing.T) {
		a := []ClaimEvent{
			{EventType: "ClaimSubmitted", BlockNumber: 100},
		}
		b := []ClaimEvent{
			{EventType: "ClaimAccepted", BlockNumber: 100},
		}
		result := mergeClaimEvents(a, b)
		assert.Len(t, result, 2)
		// a comes first when blocks are equal (a[i].BlockNumber <= b[j].BlockNumber).
		assert.Equal(t, "ClaimSubmitted", result[0].EventType)
		assert.Equal(t, "ClaimAccepted", result[1].EventType)
	})
}
