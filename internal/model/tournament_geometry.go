// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import "fmt"

// TournamentLevelCount is the number of levels in the canonical table.
const TournamentLevelCount uint64 = 2

// TournamentLevelGeometry returns the stride and height of a canonical level.
func TournamentLevelGeometry(level uint64) (stride, height uint64, err error) {
	switch level {
	case 0:
		return Log2RootTournamentStride, Log2EpochComputationHashLeafCount, nil
	case 1:
		return 0, Log2RootTournamentStride, nil
	default:
		return 0, 0, fmt.Errorf("tournament level %d is outside %d levels", level, TournamentLevelCount)
	}
}

// IsCanonicalTournamentLevel compares a declared level with the canonical geometry.
func IsCanonicalTournamentLevel(level, height, stride uint64) bool {
	expectedStride, expectedHeight, err := TournamentLevelGeometry(level)
	return err == nil && height == expectedHeight && stride == expectedStride
}
