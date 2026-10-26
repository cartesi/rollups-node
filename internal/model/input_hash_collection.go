// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/keccak"
)

// PRT collections store roots of consecutive sample bundles as entries.
// Each sampled entry has repetition one. Its last bundle may contain final-state padding.
// Remaining entries repeat FinalStateEntry of the input's final machine hash.
// Entry repetitions total InputHashCollectionCapacity; a full sampled span has no padding row.
const (
	// Log2MaxAdvanceStatesPerEpoch is the number of input-index bits reserved
	// by the protocol computation-hash tree for one epoch.
	Log2MaxAdvanceStatesPerEpoch uint64 = 24

	// MaxAdvanceStatesPerEpoch is the number of input slots reserved by the
	// protocol computation-hash tree for one epoch.
	MaxAdvanceStatesPerEpoch uint64 = 1 << Log2MaxAdvanceStatesPerEpoch

	// Log2MaxUarchCyclesPerMCycle is the emulator's maximum uarch-cycle span
	// exponent for one mcycle.
	Log2MaxUarchCyclesPerMCycle uint64 = 20

	// Log2RootTournamentStride is ArbitrationConstants.log2step(0), the root
	// tournament sampling period expressed in uarch cycles.
	Log2RootTournamentStride uint64 = 44

	// Log2MCycleComputationHashPeriod is the sampling period in mcycles.
	Log2MCycleComputationHashPeriod uint64 = Log2RootTournamentStride - Log2MaxUarchCyclesPerMCycle

	// Log2MCyclesPerStateHashEntry fixes the storage density in mcycles per entry.
	Log2MCyclesPerStateHashEntry uint64 = 24

	// Log2StateHashesPerEntry is the height of each entry's sample subtree.
	Log2StateHashesPerEntry uint64 = Log2MCyclesPerStateHashEntry - Log2MCycleComputationHashPeriod

	// Log2InputHashCollectionCapacity is the number of entry-index bits
	// reserved for one input in the protocol computation-hash tree.
	Log2InputHashCollectionCapacity uint64 = Log2MaxExecutionCycles - Log2MCyclesPerStateHashEntry

	// InputHashCollectionCapacity is the number of entries reserved for one input.
	InputHashCollectionCapacity uint64 = 1 << Log2InputHashCollectionCapacity

	// Log2InputComputationHashLeafCount is the number of sample-index bits per input.
	Log2InputComputationHashLeafCount uint64 = Log2InputHashCollectionCapacity + Log2StateHashesPerEntry

	// Log2EpochComputationHashLeafCount is the height of the epoch tree in leaves.
	Log2EpochComputationHashLeafCount uint64 = Log2MaxAdvanceStatesPerEpoch + Log2InputComputationHashLeafCount
)

// RepeatedStateHashRoot returns the root of 2^log2Count copies of h.
func RepeatedStateHashRoot(h common.Hash, log2Count uint64) common.Hash {
	hasher := keccak.NewLegacyKeccak256()
	for range log2Count {
		hasher.Reset()
		_, _ = hasher.Write(h[:])
		_, _ = hasher.Write(h[:])
		hasher.Sum(h[:0])
	}
	return h
}

// FinalStateEntry returns the entry root used to pad an input's final state.
func FinalStateEntry(h common.Hash) common.Hash {
	return RepeatedStateHashRoot(h, Log2StateHashesPerEntry)
}

// canonicalInputHashCollectionCounts maps a completed input's cycle delta to
// the emulator's raw count, the retained entry count, and final-state padding.
func canonicalInputHashCollectionCounts(deltaMCycles uint64) (
	rawCount, keptCount, padding uint64, err error,
) {
	var periodicSamples uint64
	if deltaMCycles != 0 {
		periodicSamples = (deltaMCycles - 1) >> Log2MCycleComputationHashPeriod
	}
	rawCount = (periodicSamples >> Log2StateHashesPerEntry) + 1
	if Log2StateHashesPerEntry > 0 {
		rawCount++
	}
	keptCount = periodicSamples >> Log2StateHashesPerEntry
	if periodicSamples&((uint64(1)<<Log2StateHashesPerEntry)-1) != 0 {
		keptCount++
	}
	if keptCount > InputHashCollectionCapacity {
		return 0, 0, 0, ValidateInputHashCollectionSpan(keptCount, 0)
	}
	return rawCount, keptCount, InputHashCollectionCapacity - keptCount, nil
}

// CanonicalInputHashCollection retains entries containing periodic samples.
// It folds discarded final-state-only entries into padding and returns their root.
func CanonicalInputHashCollection(collected [][32]byte, deltaMCycles uint64) (
	kept [][32]byte, padding uint64, final [32]byte, err error,
) {
	rawCount, keptCount, padding, err := canonicalInputHashCollectionCounts(deltaMCycles)
	if err != nil {
		return nil, 0, [32]byte{}, err
	}
	if uint64(len(collected)) != rawCount {
		return nil, 0, [32]byte{}, fmt.Errorf("collected %d state hash entries, expected %d", len(collected), rawCount)
	}
	final = collected[len(collected)-1]
	for _, entry := range collected[keptCount : len(collected)-1] {
		if entry != final {
			return nil, 0, [32]byte{}, errors.New("discarded state hash entry differs from final-state entry")
		}
	}
	return collected[:keptCount], padding, final, nil
}

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
