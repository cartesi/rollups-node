// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/stretchr/testify/require"
)

func storeAutomaticYieldProgram(t *testing.T, cycle uint64, reason uint16, data uint32, yieldMask uint64) string {
	t.Helper()
	const ramStart = uint64(0x80000000)
	config := fmt.Sprintf(`{
        "processor":{"registers":{"pc":%d,"htif":{"iyield":%d}}},
        "ram":{"length":4096},
        "cmio":{"rx_buffer":{},"tx_buffer":{}}
    }`, ramStart, yieldMask)
	local, err := emulator.CreateMachine(config, "", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, local.Destroy()); local.Delete() })
	actualMask, err := local.ReadReg(emulator.REG_HTIF_IYIELD)
	require.NoError(t, err)
	require.Equal(t, yieldMask, actualMask)

	// Each loop iteration takes two instructions. Six other instructions precede
	// the automatic stop; an optional NOP selects either cycle parity.
	iterations := (cycle - 6) / 2
	words := []uint32{
		0x400082b7, // lui t0,0x40008: HTIF MMIO, not its shadow at 0x330
		0x00000317, // auipc t1,0: literal base is ramStart+4
	}
	appendDelay := func(count uint64) {
		upper := uint32(((count + 0x800) >> 12) & 0xfffff)
		lower := uint32(count & 0xfff)
		words = append(words,
			upper<<12|0x3b7,   // lui t2,upper
			lower<<20|0x38393, // addi t2,t2,lower
			0xfff38393,        // addi t2,t2,-1
			0xfe039ee3,        // bne t2,zero,-4
		)
	}
	appendDelay(iterations)
	if (cycle-6)%2 != 0 {
		words = append(words, 0x00000013) // addi zero,zero,0
	}
	automaticLoad := len(words)
	words = append(words, 0, 0x0072b023) // ld t2,literal(t1); sd t2,0(t0)
	// Cross at least two more periodic boundaries after the automatic yield.
	appendDelay(3 * MCycleComputationHashPeriod / 2)
	manualLoad := len(words)
	words = append(words, 0, 0x0072b023, 0x0000006f) // ld; sd; jal zero,0
	require.Less(t, 4*len(words)+7, 1<<11, "literals must fit a positive 12-bit load offset")
	literalOffset := uint32((4*len(words) + 7) & 0xff8)
	words[automaticLoad] = (literalOffset-4)<<20 | 0x33383
	words[manualLoad] = (literalOffset+8-4)<<20 | 0x33383
	program := make([]byte, literalOffset+16)
	for i, word := range words {
		binary.LittleEndian.PutUint32(program[4*i:], word)
	}
	automatic := emulator.HtifDeviceYield<<56 | uint64(emulator.YieldAutomatic)<<48 | uint64(reason)<<32 | uint64(data)
	accepted := emulator.HtifDeviceYield<<56 | uint64(emulator.YieldManual)<<48 |
		uint64(emulator.ManualYieldReasonAccepted)<<32 | HashSize
	binary.LittleEndian.PutUint64(program[literalOffset:], automatic)
	binary.LittleEndian.PutUint64(program[literalOffset+8:], accepted)
	require.NoError(t, local.WriteMemory(ramStart, program))
	require.NoError(t, local.WriteMemory(TxBufferAddress, []byte("discard")))
	require.NoError(t, local.WriteReg(emulator.REG_IFLAGS_Y, 1))
	require.NoError(t, local.WriteReg(emulator.REG_HTIF_TOHOST, accepted))
	_, err = local.GetRootHash()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "machine")
	require.NoError(t, local.Store(path))
	return path
}

// observedYieldCollector checks returned arithmetic against actual cycles,
// independently of the C API copier shared by the two collection paths.
type observedYieldCollector struct {
	Backend
	t              *testing.T
	start          uint64
	automaticCycle []uint64
}

func (b *observedYieldCollector) RunAndCollectRootHashes(
	end uint64, state *HashCollectorState, timeout time.Duration,
) (BreakReason, error) {
	reason, err := b.Backend.RunAndCollectRootHashes(end, state, timeout)
	require.NoError(b.t, err)
	cycle, err := b.ReadMCycle(timeout)
	require.NoError(b.t, err)
	if !isFixedPointBreakReason(reason) {
		delta := cycle - b.start
		require.Equal(b.t, delta%state.MCycleSamplingPeriod, state.MCyclePhase, "returned phase at cycle %d", cycle)
		var partial struct {
			LeafCount uint64 `json:"leaf_count"`
		}
		if len(state.PartialBundle) != 0 {
			require.NoError(b.t, json.Unmarshal(state.PartialBundle, &partial))
		}
		samples := uint64(len(state.Hashes))*(uint64(1)<<state.Log2BundleMCycleCount) + partial.LeafCount
		require.Equal(b.t, delta/state.MCycleSamplingPeriod, samples, "returned sample count at cycle %d", cycle)
	}
	if reason == YieldedAutomatically {
		b.automaticCycle = append(b.automaticCycle, cycle)
	}
	return reason, nil
}

type unknownYieldOracleResult struct {
	cycle          uint64
	root           Hash
	hashes         []Hash
	automaticCycle []uint64
}

func runUnknownYieldOracle(t *testing.T, backend Backend, checkpoint Hash, collect bool) unknownYieldOracleResult {
	t.Helper()
	require.NoError(t, backend.SendCmioResponse(uint16(AdvanceStateRequest), []byte("request"), &checkpoint, 2*time.Second))
	state := &HashCollectorState{MCycleSamplingPeriod: MCycleComputationHashPeriod}
	const target = 6 * MCycleComputationHashPeriod
	result := unknownYieldOracleResult{}
	for result.cycle < target {
		var reason BreakReason
		var err error
		if collect {
			reason, err = backend.RunAndCollectRootHashes(target, state, 2*time.Second)
		} else {
			reason, err = backend.Run(target, 2*time.Second)
		}
		require.NoError(t, err)
		previous := result.cycle
		result.cycle, err = backend.ReadMCycle(2 * time.Second)
		require.NoError(t, err)
		// These expose iflags_H, iflags_Y and overflow. Automatic observations
		// are never received or interpreted, as in the Sling stop-point loop.
		if reason == Halted || reason == YieldedManually || reason == McycleOverflow {
			require.Equal(t, YieldedManually, reason)
			result.root, err = backend.GetRootHash(2 * time.Second)
			require.NoError(t, err)
			result.hashes = state.Hashes
			return result
		}
		if reason == YieldedAutomatically {
			result.automaticCycle = append(result.automaticCycle, result.cycle)
		}
		require.Greater(t, result.cycle, previous)
	}
	t.Fatal("oracle reached its finite target before the accepted manual yield")
	return result
}

func TestUnknownAutomaticYieldMatchesRealEmulatorOracle(t *testing.T) {
	// Each image is compared with its own direct-Run stop-point oracle. Both
	// paths share the emulator; unbundled collection also shares its C API
	// copier. Independent phase/count assertions cover that shared boundary.
	// This does not prove native CPU/hash correctness or general Sling equivalence.
	period := MCycleComputationHashPeriod
	for _, cycle := range []uint64{2*period - 1, 2 * period, 2*period + 1} {
		for _, increment := range []uint64{4 * period, period/3 + 12345} {
			t.Run(fmt.Sprintf("cycle=%d/increment=%d", cycle, increment), func(t *testing.T) {
				checkUnknownYieldAgainstOracle(t, cycle, increment, 7)
			})
		}
	}
	t.Run("oversized unknown data", func(t *testing.T) {
		checkUnknownYieldAgainstOracle(t, 2*period+1, 4*period, uint32(txBufferSize+1))
	})
}

func checkUnknownYieldAgainstOracle(t *testing.T, cycle, increment uint64, data uint32) {
	t.Helper()
	path := storeAutomaticYieldProgram(t, cycle, 0x7f, data, 3)
	config := DefaultConfig(path)
	config.ExecutionParameters.AdvanceIncCycles = increment
	runtime, err := Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), config)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	checkpoint, err := runtime.Hash(t.Context())
	require.NoError(t, err)
	plain := runUnknownYieldOracle(t, collectorBackend(t, path), checkpoint, false)
	unbundledBackend := collectorBackend(t, path)
	unbundledStart, err := unbundledBackend.ReadMCycle(2 * time.Second)
	require.NoError(t, err)
	unbundled := &observedYieldCollector{Backend: unbundledBackend, t: t, start: unbundledStart}
	collected := runUnknownYieldOracle(t, unbundled, checkpoint, true)
	require.Equal(t, plain.cycle, collected.cycle)
	require.Equal(t, plain.root, collected.root)
	require.Equal(t, []uint64{cycle}, plain.automaticCycle)
	require.Equal(t, plain.automaticCycle, collected.automaticCycle)
	require.GreaterOrEqual(t, plain.cycle, cycle+2*MCycleComputationHashPeriod)
	impl := runtime.(*machineImpl)
	start, err := impl.backend.ReadMCycle(2 * time.Second)
	require.NoError(t, err)
	observed := &observedYieldCollector{Backend: impl.backend, t: t, start: start}
	impl.backend = observed
	response, err := runtime.Advance(t.Context(), []byte("request"), checkpoint, true)
	require.NoError(t, err)
	require.Equal(t, CompletionStatusAccepted, response.Status)
	require.Equal(t, []IgnoredAutomaticYield{{Reason: 0x7f, Count: 1}}, response.IgnoredAutomaticYields)
	require.Empty(t, response.Outputs)
	require.Empty(t, response.Reports)
	require.Equal(t, []uint64{cycle}, observed.automaticCycle)
	final, err := runtime.Hash(t.Context())
	require.NoError(t, err)
	require.Equal(t, plain.root, final)
	actualCycle, err := impl.backend.ReadMCycle(2 * time.Second)
	require.NoError(t, err)
	require.Equal(t, plain.cycle, actualCycle)
	hashes := collected.hashes
	require.Equal(t, (plain.cycle-1)/MCycleComputationHashPeriod+1, uint64(len(hashes)))
	require.Equal(t, plain.root, hashes[len(hashes)-1])
	_, kept := denseCompletedEntries(hashes[:len(hashes)-1], plain.root, Log2StateHashesPerEntry)
	require.NotEmpty(t, kept)
	require.Equal(t, kept, response.PeriodicStateHashes)
	require.Equal(t, InputEntryCapacity-uint64(len(kept)), response.PaddingRepetitions)
	finalLeaves := make([]Hash, 1<<Log2StateHashesPerEntry)
	for i := range finalLeaves {
		finalLeaves[i] = plain.root
	}
	require.Equal(t, denseTestEntry(finalLeaves), response.FinalStateEntry)
}

func TestAutomaticYieldRequestMatchesRealEmulator(t *testing.T) {
	for _, test := range []struct {
		name   string
		reason emulator.CmioYieldReason
		data   uint32
		want   []byte
	}{
		{name: "progress", reason: emulator.AutomaticYieldReasonProgress, data: 0x01020304, want: []byte{4, 3, 2, 1}},
		{name: "output", reason: emulator.AutomaticYieldReasonOutput, data: 7, want: []byte("discard")},
		{name: "report", reason: emulator.AutomaticYieldReasonReport, data: 7, want: []byte("discard")},
	} {
		t.Run(test.name, func(t *testing.T) {
			const cycle = 2*MCycleComputationHashPeriod + 1
			path := storeAutomaticYieldProgram(t, cycle, uint16(test.reason), test.data, 3)
			backend := collectorBackend(t, path)
			checkpoint, err := backend.GetRootHash(2 * time.Second)
			require.NoError(t, err)
			require.NoError(t, backend.SendCmioResponse(uint16(AdvanceStateRequest), []byte("request"), &checkpoint, 2*time.Second))
			stop, err := backend.Run(cycle+1, 2*time.Second)
			require.NoError(t, err)
			require.Equal(t, YieldedAutomatically, stop)

			machine := &machineImpl{backend: backend, params: DefaultConfig(path).ExecutionParameters}
			reason, data, err := machine.readAutomaticYieldRequest()
			require.NoError(t, err)
			require.Equal(t, uint16(test.reason), reason)
			require.Equal(t, test.want, data)

			command, nativeReason, nativeData, err := backend.ReceiveCmioRequest(2 * time.Second)
			require.NoError(t, err)
			require.EqualValues(t, emulator.YieldAutomatic, command)
			require.Equal(t, uint16(test.reason), nativeReason)
			require.Equal(t, test.want, nativeData)
		})
	}
}

func TestUnknownAutomaticYieldProgramCanDisableYields(t *testing.T) {
	path := storeAutomaticYieldProgram(t, 2*MCycleComputationHashPeriod+1, 0x7f, 7, 0)
	backend := collectorBackend(t, path)
	checkpoint, err := backend.GetRootHash(2 * time.Second)
	require.NoError(t, err)
	require.NoError(t, backend.SendCmioResponse(uint16(AdvanceStateRequest), []byte("request"), &checkpoint, 2*time.Second))
	// A misplaced iyield configuration key would leave the default mask3 and
	// stop at the automatic yield instead of reaching this target.
	const target = 6 * MCycleComputationHashPeriod
	reason, err := backend.Run(target, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, ReachedTargetMcycle, reason)
	cycle, err := backend.ReadMCycle(2 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint64(target), cycle)
}
