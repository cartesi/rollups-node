// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func managerNativeYieldGuest(t *testing.T, reason emulator.CmioYieldReason) machine.Machine {
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
	instructions := []uint32{0x400082b7, 0x0002b423, 0x0132b023}
	program := make([]byte, 4*len(instructions))
	for index, instruction := range instructions {
		binary.LittleEndian.PutUint32(program[4*index:], instruction)
	}
	require.NoError(t, local.WriteMemory(ramStart, program))
	length := uint64(0)
	if reason == emulator.ManualYieldReasonAccepted {
		length = machine.HashSize
	}
	yield := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldManual)<<emulator.HtifCommandShift | uint64(reason)<<emulator.HtifReasonShift | length
	for _, register := range []struct {
		id    emulator.RegID
		value uint64
	}{
		{emulator.REG_X19, yield},
		{emulator.REG_IFLAGS_Y, 1},
		{emulator.REG_HTIF_TOHOST_DEV, emulator.HtifDeviceYield},
		{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
		{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
		{emulator.REG_HTIF_TOHOST_DATA, machine.HashSize},
	} {
		require.NoError(t, local.WriteReg(register.id, register.value))
	}
	_, err = local.GetRootHash()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "machine")
	require.NoError(t, local.Store(path))
	machineConfig := machine.DefaultConfig(path)
	machineConfig.ExecutionParameters.AdvanceIncDeadline = 2 * time.Second
	machineConfig.ExecutionParameters.AdvanceMaxDeadline = 2 * time.Second
	runtime, err := machine.Load(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)), machineConfig)
	require.NoError(t, err)
	return runtime
}

func TestManagerChecksNativeAcceptedAndRejectedFinalEntries(t *testing.T) {
	for _, reason := range []emulator.CmioYieldReason{emulator.ManualYieldReasonAccepted, emulator.ManualYieldReasonRejected} {
		t.Run(fmt.Sprintf("reason=%d", reason), func(t *testing.T) {
			runtime := managerNativeYieldGuest(t, reason)
			before, err := runtime.Hash(t.Context())
			require.NoError(t, err)
			app := &model.Application{ExecutionParameters: model.ExecutionParameters{
				AdvanceMaxDeadline: 2 * time.Second, MaxConcurrentInspects: 1,
			}}
			instance, err := NewMachineInstanceWithFactory(t.Context(), app, 0,
				slog.New(slog.NewTextHandler(io.Discard, nil)), &MockMachineRuntimeFactory{RuntimeToReturn: runtime})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, instance.Close()) })
			result, err := instance.Advance(t.Context(), []byte("input"), 0, 0, true)
			require.NoError(t, err)
			require.Equal(t, uint64(1), instance.ProcessedInputs())
			require.NoError(t, model.ValidateInputHashCollectionSpan(uint64(len(result.PeriodicStateHashes)), result.PaddingRepetitions))
			if reason == emulator.ManualYieldReasonRejected {
				require.Equal(t, model.InputCompletionStatus_Rejected, result.Status)
				require.Equal(t, common.Hash(before), result.MachineHash)
			} else {
				require.Equal(t, model.InputCompletionStatus_Accepted, result.Status)
				require.NotEqual(t, common.Hash(before), result.MachineHash)
			}
		})
	}
}
