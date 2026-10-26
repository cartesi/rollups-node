// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func storeCollectorLoop(t *testing.T, start, end uint64, ready bool) string {
	t.Helper()
	const ramStart = uint64(0x80000000)
	config, err := json.Marshal(map[string]any{
		"processor": map[string]any{"registers": map[string]any{"pc": ramStart}},
		"ram":       map[string]any{"length": 4096},
		"cmio":      map[string]any{"rx_buffer": map[string]any{}, "tx_buffer": map[string]any{}},
	})
	require.NoError(t, err)
	local, err := emulator.CreateMachine(string(config), "", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, local.Destroy()); local.Delete() })
	program := make([]byte, 4)
	binary.LittleEndian.PutUint32(program, 0x0000006f) // jal zero,0
	require.NoError(t, local.WriteMemory(ramStart, program))
	require.NoError(t, local.WriteReg(emulator.REG_MCYCLE, start))
	require.NoError(t, local.WriteReg(emulator.REG_IMCYCLEMAX, end))
	if ready {
		for _, register := range []struct {
			id    emulator.RegID
			value uint64
		}{
			{emulator.REG_IFLAGS_Y, 1},
			{emulator.REG_HTIF_TOHOST_DEV, emulator.HtifDeviceYield},
			{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
			{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
			{emulator.REG_HTIF_TOHOST_DATA, HashSize},
		} {
			require.NoError(t, local.WriteReg(register.id, register.value))
		}
	}
	_, err = local.GetRootHash()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "machine")
	require.NoError(t, local.Store(path))
	return path
}

func collectorBackend(t *testing.T, path string) Backend {
	t.Helper()
	backend, _, _, err := NewLibCartesiBackend("127.0.0.1:0", 2*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.ShutdownServer(2*time.Second)); backend.Delete() })
	require.NoError(t, backend.Load(path, "", 2*time.Second))
	return backend
}

func collectBackendToOverflow(t *testing.T, backend Backend, delta, period uint64, height int32) *HashCollectorState {
	t.Helper()
	state := &HashCollectorState{MCycleSamplingPeriod: period, Log2BundleMCycleCount: height}
	increment := period/3 + 12345
	for current := uint64(0); current < delta; {
		end := min(current+increment, delta)
		reason, err := backend.RunAndCollectRootHashes(end, state, 2*time.Second)
		require.NoError(t, err)
		current, err = backend.ReadMCycle(2 * time.Second)
		require.NoError(t, err)
		if reason == McycleOverflow {
			require.Equal(t, delta, current)
			require.NoError(t, validateHashCollectorProgress(state, current, true))
			return state
		}
		require.Equal(t, ReachedTargetMcycle, reason)
		if height == 0 {
			require.Equal(t, current/period, uint64(len(state.Hashes)))
			require.Empty(t, state.PartialBundle)
		} else {
			require.NoError(t, validateHashCollectorProgress(state, current, false))
			if len(state.PartialBundle) != 0 {
				original := state.PartialBundle
				var bundle map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(original, &bundle))
				var leaves uint64
				require.NoError(t, json.Unmarshal(bundle["leaf_count"], &leaves))
				bundle["leaf_count"], err = json.Marshal(leaves + 1)
				require.NoError(t, err)
				state.PartialBundle, err = json.Marshal(bundle)
				require.NoError(t, err)
				require.ErrorContains(t, validateHashCollectorProgress(state, current, false), "samples, expected")
				state.PartialBundle = original
			}
		}
	}
	t.Fatal("collector reached its endpoint without overflow")
	return nil
}

func denseCompletedEntries(periodic []Hash, final Hash, height uint64) (raw, kept []Hash) {
	width := 1 << height
	leaves := append(append([]Hash(nil), periodic...), final)
	for start := 0; start < len(leaves); start += width {
		bundle := make([]Hash, width)
		for index := range bundle {
			bundle[index] = final
			if start+index < len(leaves) {
				bundle[index] = leaves[start+index]
			}
		}
		entry := denseTestEntry(bundle)
		raw = append(raw, entry)
		if start < len(periodic) {
			kept = append(kept, entry)
		}
	}
	if height != 0 {
		bundle := make([]Hash, width)
		for index := range bundle {
			bundle[index] = final
		}
		raw = append(raw, denseTestEntry(bundle))
	}
	return raw, kept
}

func TestBundledBackendMatchesDenseUnbundledSamples(t *testing.T) {
	for _, geometry := range []struct {
		period uint64
		height uint8
	}{{18, 6}, {22, 2}} {
		period, height := uint64(1)<<geometry.period, geometry.height
		for _, delta := range []uint64{1, period, 63 * period, 64 * period, 64*period + 1, 65 * period, 128*period + 1, 130*period + 3} {
			t.Run(fmt.Sprintf("period=%d/k=%d/delta=%d", geometry.period, height, delta), func(t *testing.T) {
				path := storeCollectorLoop(t, 0, delta, false)
				bundled := collectBackendToOverflow(t, collectorBackend(t, path), delta, period, int32(height))
				plain := collectBackendToOverflow(t, collectorBackend(t, path), delta, period, 0)
				final := plain.Hashes[len(plain.Hashes)-1]
				periodic := plain.Hashes[:len(plain.Hashes)-1]
				raw, wantKept := denseCompletedEntries(periodic, final, uint64(height))
				require.Equal(t, raw, bundled.Hashes)
				if geometry.period == model.Log2MCycleComputationHashPeriod {
					kept, padding, finalEntry, err := model.CanonicalInputHashCollection(bundled.Hashes, delta)
					require.NoError(t, err)
					require.Equal(t, wantKept, append([]Hash(nil), kept...))
					require.Equal(t, InputEntryCapacity-uint64(len(wantKept)), padding)
					require.Equal(t, Hash(model.RepeatedStateHashRoot(common.Hash(final), uint64(height))), finalEntry)
					if len(periodic)%(1<<height) == 0 {
						require.Equal(t, len(wantKept)+2, len(raw))
						require.Equal(t, finalEntry, raw[len(raw)-2])
					} else {
						require.Equal(t, len(wantKept)+1, len(raw))
					}
				}
			})
		}
	}
}

func TestAdvanceProductionCollectorMatchesFinalRoot(t *testing.T) {
	period := MCycleComputationHashPeriod
	for _, samples := range []uint64{0, 1, 63, 64, 65, 130} {
		t.Run(fmt.Sprintf("samples=%d", samples), func(t *testing.T) {
			delta := samples*period + 1
			path := storeCollectorLoop(t, math.MaxUint64-delta, math.MaxUint64, true)
			config := DefaultConfig(path)
			config.ExecutionParameters.AdvanceIncCycles = period/3 + 12345
			config.ExecutionParameters.AdvanceIncDeadline = 2 * time.Second
			config.ExecutionParameters.AdvanceMaxDeadline = 20 * time.Second
			machine, err := Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), config)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, machine.Close()) })
			checkpoint, err := machine.Hash(t.Context())
			require.NoError(t, err)
			response, err := machine.Advance(t.Context(), []byte("input"), checkpoint, true)
			require.NoError(t, err)
			require.Equal(t, CompletionStatusOverflow, response.Status)
			final, err := machine.Hash(t.Context())
			require.NoError(t, err)
			require.Equal(t, Hash(model.FinalStateEntry(common.Hash(final))), response.FinalStateEntry)
			require.NoError(t, model.ValidateInputHashCollectionSpan(
				uint64(len(response.PeriodicStateHashes)), response.PaddingRepetitions))
		})
	}
}
