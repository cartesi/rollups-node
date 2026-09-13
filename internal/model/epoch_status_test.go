// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNonTerminalEpochStatusesPartitionAllValues(t *testing.T) {
	partition := NonTerminalEpochStatuses()
	terminal := []EpochStatus{EpochStatus_ClaimAccepted, EpochStatus_ClaimRejected, EpochStatus_ClaimForeclosed}
	partition = append(partition, terminal...)
	require.ElementsMatch(t, EpochStatusAllValues, partition,
		"classify every epoch status exactly once and keep the epoch_unreconciled_idx predicate in sync")
}

func TestNonTerminalEpochStatusesReturnsOwnedSlice(t *testing.T) {
	first, second := NonTerminalEpochStatuses(), NonTerminalEpochStatuses()
	require.NotEmpty(t, first)
	first[0] = EpochStatus_ClaimAccepted
	require.NotContains(t, second, EpochStatus_ClaimAccepted)
	require.Equal(t, second, NonTerminalEpochStatuses())
}
