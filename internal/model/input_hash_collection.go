// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import "fmt"

const (
	// Log2MaxAdvanceStatesPerEpoch is the number of input-index bits reserved
	// by the protocol computation-hash tree for one epoch.
	Log2MaxAdvanceStatesPerEpoch uint64 = 24

	// MaxAdvanceStatesPerEpoch is the number of input slots reserved by the
	// protocol computation-hash tree for one epoch.
	MaxAdvanceStatesPerEpoch uint64 = 1 << Log2MaxAdvanceStatesPerEpoch

	// Log2InputHashCollectionCapacity is the number of state-hash index bits
	// reserved for one input in the protocol computation-hash tree.
	Log2InputHashCollectionCapacity uint64 = 24

	// InputHashCollectionCapacity is the number of computation-hash leaves
	// reserved for one input.
	InputHashCollectionCapacity uint64 = 1 << Log2InputHashCollectionCapacity

	// Log2EpochComputationHashLeafCount is the height of the epoch computation-
	// hash tree: 2^24 input slots, each containing 2^24 state-hash entries.
	Log2EpochComputationHashLeafCount uint64 = Log2MaxAdvanceStatesPerEpoch + Log2InputHashCollectionCapacity
)

// ValidateInputHashCollectionSpan verifies that periodic state hashes and the
// final-state padding cover exactly one persisted input hash collection.
func ValidateInputHashCollectionSpan(hashCount, paddingRepetitions uint64) error {
	if hashCount > InputHashCollectionCapacity {
		return fmt.Errorf(
			"collected state hash count %d exceeds input hash collection capacity %d",
			hashCount,
			InputHashCollectionCapacity,
		)
	}
	expectedPaddingRepetitions := InputHashCollectionCapacity - hashCount
	if paddingRepetitions != expectedPaddingRepetitions {
		return fmt.Errorf(
			"collected state hash count %d with padding repetitions %d does not cover input hash collection capacity %d",
			hashCount,
			paddingRepetitions,
			InputHashCollectionCapacity,
		)
	}
	return nil
}
