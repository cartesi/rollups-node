// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import "github.com/cartesi/rollups-node/internal/model"

const (
	// Log2MaxUarchCyclesPerMCycle is the emulator's
	// CM_ROLLUP_LOG2_MAX_UARCH_CYCLES_PER_MCYCLE; the Go name omits the C
	// ROLLUP_ prefix. The DAVE 3.0-alpha node calls the same dimension
	// LOG2_UARCH_SPAN_TO_BARCH.
	Log2MaxUarchCyclesPerMCycle uint64 = model.Log2MaxUarchCyclesPerMCycle

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
	// LOG2_STRIDE; ArbitrationConstants.log2step(0) selects it.
	Log2UarchCycleComputationHashPeriod uint64 = model.Log2RootTournamentStride

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

	// Log2MCycleComputationHashPeriod is the value passed to the emulator's
	// log2_mcycle_period CLI parameter, expressed in mcycles.
	Log2MCycleComputationHashPeriod uint64 = model.Log2MCycleComputationHashPeriod

	// MCycleComputationHashPeriod is the number of mcycles between state-root
	// samples. The DAVE 3.0-alpha node calls it BIG_STEPS_IN_STRIDE.
	MCycleComputationHashPeriod uint64 = 1 << Log2MCycleComputationHashPeriod

	// Log2StateHashesPerEntry is the sample-subtree height of each stored entry.
	Log2StateHashesPerEntry uint64 = model.Log2StateHashesPerEntry

	// Log2InputEntryCapacity is the emulator's log2_entries_per_input, counting
	// entry roots rather than individual state-hash samples.
	Log2InputEntryCapacity uint64 = model.Log2InputHashCollectionCapacity

	// InputEntryCapacity is the number of computation-hash entries reserved
	// for one input.
	InputEntryCapacity uint64 = model.InputHashCollectionCapacity

	// Log2EpochComputationHashLeafCount is the emulator's
	// log2_epoch_computation_hash_leaf_count, counting individual samples.
	Log2EpochComputationHashLeafCount uint64 = model.Log2EpochComputationHashLeafCount

	// EpochComputationHashLeafCount is the number of leaves in the root epoch
	// computation hash. The DAVE 3.0-alpha node calls it
	// STRIDE_COUNT_IN_EPOCH.
	EpochComputationHashLeafCount uint64 = 1 << Log2EpochComputationHashLeafCount

	// MaxAdvanceStatesPerEpoch is the number of input slots reserved by the
	// emulator computation-hash tree. DAVE derives it as
	// INPUT_SPAN_TO_EPOCH + 1.
	MaxAdvanceStatesPerEpoch uint64 = model.MaxAdvanceStatesPerEpoch

	// Each collection call spans at most 2^8 sampling periods. It can also
	// collect a fixed-point sample at its endpoint. This bounds per-call
	// hashing work but does not guarantee completion within its deadline.
	log2ComputationHashEntriesPerChunk uint64 = 8
	mcycleComputationHashChunkSize     uint64 = MCycleComputationHashPeriod << log2ComputationHashEntriesPerChunk
)
