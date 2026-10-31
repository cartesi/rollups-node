// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTournamentGeometry(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint64(2), TournamentLevelCount)
	for _, test := range []struct {
		level, height, stride uint64
	}{
		{level: 0, height: 54, stride: 38},
		{level: 1, height: 38, stride: 0},
	} {
		stride, height, err := TournamentLevelGeometry(test.level)
		require.NoError(t, err)
		require.Equal(t, test.stride, stride)
		require.Equal(t, test.height, height)
		require.True(t, IsCanonicalTournamentLevel(test.level, test.height, test.stride))
		require.False(t, IsCanonicalTournamentLevel(test.level, test.height+1, test.stride))
		require.False(t, IsCanonicalTournamentLevel(test.level, test.height, test.stride+1))
	}
	for _, level := range []uint64{2, math.MaxUint64} {
		_, _, err := TournamentLevelGeometry(level)
		require.Error(t, err)
		require.False(t, IsCanonicalTournamentLevel(level, 54, 38))
	}
	require.False(t, IsCanonicalTournamentLevel(0, 48, 44))
}
