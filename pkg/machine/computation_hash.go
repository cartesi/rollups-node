// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
)

const (
	// Log2MaxUarchCyclesPerMCycle is emulator 0.21's
	// CM_ROLLUP_LOG2_MAX_UARCH_CYCLES_PER_MCYCLE; the Go name omits the C
	// ROLLUP_ prefix. The DAVE 3.0-alpha node calls the same dimension
	// LOG2_UARCH_SPAN_TO_BARCH.
	Log2MaxUarchCyclesPerMCycle uint64 = 20

	// Log2MaxMCyclesPerAdvanceState is emulator 0.21's
	// CM_ROLLUP_LOG2_MAX_MCYCLES_PER_ADVANCE_STATE; the Go name omits the C
	// ROLLUP_ prefix. The DAVE 3.0-alpha node calls the same dimension
	// LOG2_BARCH_SPAN_TO_INPUT. Keep it aliased to the model definition so
	// the machine endpoint and computation-hash dimensions cannot diverge.
	Log2MaxMCyclesPerAdvanceState uint64 = model.Log2MaxExecutionCycles

	// Log2MaxAdvanceStatesPerEpoch is emulator 0.21's
	// CM_ROLLUP_LOG2_MAX_ADVANCE_STATES_PER_EPOCH; the Go name omits the C
	// ROLLUP_ prefix. The DAVE 3.0-alpha node calls the same dimension
	// LOG2_INPUT_SPAN_TO_EPOCH.
	Log2MaxAdvanceStatesPerEpoch uint64 = model.Log2MaxAdvanceStatesPerEpoch

	// Log2UarchCycleComputationHashPeriod is the computation-hash sampling
	// period expressed in uarch cycles. The DAVE 3.0-alpha node calls it
	// LOG2_STRIDE, and ArbitrationConstants.log2step(0) fixes it at 44.
	Log2UarchCycleComputationHashPeriod uint64 = 44

	// Log2UarchCyclesPerAdvanceState is the full advance-state window expressed
	// in uarch cycles. The DAVE 3.0-alpha node calls it
	// LOG2_UARCH_SPAN_TO_INPUT.
	Log2UarchCyclesPerAdvanceState uint64 = Log2MaxMCyclesPerAdvanceState + Log2MaxUarchCyclesPerMCycle // 68

	// MaxUarchCycle is emulator 0.21's UARCH_CYCLE_MAX. The DAVE 3.0-alpha
	// node calls the same maximum coordinate UARCH_SPAN_TO_BARCH.
	MaxUarchCycle uint64 = (1 << Log2MaxUarchCyclesPerMCycle) - 1

	// MaxMCycleDeltaPerAdvanceState is the largest endpoint delta in one
	// advance-state execution window, not its 2^48-cycle cardinality. The DAVE
	// 3.0-alpha node calls it BARCH_SPAN_TO_INPUT.
	MaxMCycleDeltaPerAdvanceState uint64 = model.MaxExecutionCycleSpan

	// MaxAdvanceStateIndexPerEpoch is the largest zero-based input-slot index.
	// The DAVE 3.0-alpha node calls it INPUT_SPAN_TO_EPOCH.
	MaxAdvanceStateIndexPerEpoch uint64 = model.MaxAdvanceStatesPerEpoch - 1

	// Log2MCycleComputationHashPeriod is the selected value passed to emulator
	// 0.21's log2_mcycle_computation_hash_period CLI parameter. DAVE expresses
	// the same period as LOG2_STRIDE in uarch cycles, hence 44-20.
	Log2MCycleComputationHashPeriod uint64 = Log2UarchCycleComputationHashPeriod - Log2MaxUarchCyclesPerMCycle // 24

	// MCycleComputationHashPeriod is the number of mcycles between state-root
	// samples. The DAVE 3.0-alpha node calls it BIG_STEPS_IN_STRIDE.
	MCycleComputationHashPeriod uint64 = 1 << Log2MCycleComputationHashPeriod // 16_777_216

	// Log2InputEntryCapacity is emulator 0.21's log2_entries_per_input when
	// log2_bundle_mcycle_count is zero. DAVE derives the same value from its
	// input span and LOG2_STRIDE.
	Log2InputEntryCapacity uint64 = model.Log2InputHashCollectionCapacity

	// InputEntryCapacity is the number of computation-hash leaves reserved
	// for one input. The DAVE 3.0-alpha node calls it STRIDE_COUNT_IN_INPUT.
	InputEntryCapacity uint64 = model.InputHashCollectionCapacity

	// Log2EpochComputationHashLeafCount is emulator 0.21's
	// log2_epoch_computation_hash_leaf_count with bundling disabled.
	Log2EpochComputationHashLeafCount uint64 = model.Log2EpochComputationHashLeafCount

	// EpochComputationHashLeafCount is the number of leaves in the root epoch
	// computation hash. The DAVE 3.0-alpha node calls it
	// STRIDE_COUNT_IN_EPOCH.
	EpochComputationHashLeafCount uint64 = 1 << Log2EpochComputationHashLeafCount

	// MaxAdvanceStatesPerEpoch is the number of input slots reserved by the
	// emulator computation-hash tree. DAVE derives it as
	// INPUT_SPAN_TO_EPOCH + 1.
	MaxAdvanceStatesPerEpoch uint64 = model.MaxAdvanceStatesPerEpoch

	// Emulator 0.21's CLI targets roughly 2^8 returned hashes per collection
	// call. Applying the same bound prevents a large execution increment from
	// producing a multi-million-entry JSON-RPC response without changing the
	// sampling sequence.
	log2ComputationHashEntriesPerChunk uint64 = 8
	mcycleComputationHashChunkSize     uint64 = MCycleComputationHashPeriod << log2ComputationHashEntriesPerChunk
)

// ValidateInputHashCollectionSpan verifies that the collected periodic hashes
// and final-state padding repetitions cover exactly one input entry capacity. It
// checks span cardinality only; callers enforce ordering, hashes, and canonical
// padding shape separately.
func ValidateInputHashCollectionSpan(hashCount, paddingRepetitions uint64) error {
	return model.ValidateInputHashCollectionSpan(hashCount, paddingRepetitions)
}

// inputHashCollectionPaddingRepetitions returns the final-state repetitions
// needed after hashCount periodic samples to fill one input entry capacity. The
// capacity check guards the subtraction below against uint64 underflow.
func inputHashCollectionPaddingRepetitions(hashCount uint64) (uint64, error) {
	if hashCount > InputEntryCapacity {
		return 0, fmt.Errorf(
			"collected state hash count %d exceeds input entry capacity %d",
			hashCount,
			InputEntryCapacity,
		)
	}
	return InputEntryCapacity - hashCount, nil
}
