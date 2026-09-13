// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinnedCallOptsOwnsBlockNumber(t *testing.T) {
	ctx := t.Context()
	first := pinnedCallOpts(ctx, math.MaxUint64)
	second := pinnedCallOpts(ctx, math.MaxUint64)
	require.Same(t, ctx, first.Context)
	require.Same(t, ctx, second.Context)
	require.NotSame(t, first, second)
	require.NotSame(t, first.BlockNumber, second.BlockNumber)
	require.Equal(t, uint64(math.MaxUint64), first.BlockNumber.Uint64())
	first.BlockNumber.SetUint64(0)
	require.Equal(t, uint64(math.MaxUint64), second.BlockNumber.Uint64())
}
