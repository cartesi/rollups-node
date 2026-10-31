// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"encoding/binary"
	"encoding/json"
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

const endpointProgramCycles = uint64(3)

func loadEndpointGuest(t *testing.T, start uint64, reason emulator.CmioYieldReason, halted bool) Machine {
	t.Helper()
	const (
		ramStart          = uint64(0x80000000)
		ramLength         = uint64(4096)
		instructionBytes  = 4
		loadHTIF          = uint32(0x400082b7) // lui t0,0x40008
		clearFromhost     = uint32(0x0002b423) // sd zero,8(t0)
		yieldPrepared     = uint32(0x0132b023) // sd x19,0(t0)
		operationDeadline = 2 * time.Second
	)
	config, err := json.Marshal(map[string]any{
		"processor": map[string]any{"registers": map[string]any{"pc": ramStart}},
		"ram":       map[string]any{"length": ramLength},
		"cmio":      map[string]any{"rx_buffer": map[string]any{}, "tx_buffer": map[string]any{}},
	})
	require.NoError(t, err)
	local, err := emulator.CreateMachine(string(config), "", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, local.Destroy())
		local.Delete()
	})
	instructions := []uint32{loadHTIF, clearFromhost, yieldPrepared}
	program := make([]byte, instructionBytes*len(instructions))
	for index, instruction := range instructions {
		binary.LittleEndian.PutUint32(program[instructionBytes*index:], instruction)
	}
	require.NoError(t, local.WriteMemory(ramStart, program))
	length := uint64(0)
	if reason == emulator.ManualYieldReasonAccepted {
		length = HashSize
	}
	preparedYield := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldManual)<<emulator.HtifCommandShift |
		uint64(reason)<<emulator.HtifReasonShift | length
	for _, register := range []struct {
		id    emulator.RegID
		value uint64
	}{
		{emulator.REG_MCYCLE, start},
		{emulator.REG_X19, preparedYield},
		{emulator.REG_IFLAGS_Y, 1},
		{emulator.REG_HTIF_TOHOST_DEV, emulator.HtifDeviceYield},
		{emulator.REG_HTIF_TOHOST_CMD, uint64(emulator.YieldManual)},
		{emulator.REG_HTIF_TOHOST_REASON, uint64(emulator.ManualYieldReasonAccepted)},
		{emulator.REG_HTIF_TOHOST_DATA, HashSize},
	} {
		require.NoError(t, local.WriteReg(register.id, register.value))
	}
	if halted {
		require.NoError(t, local.WriteReg(emulator.REG_IFLAGS_H, 1))
	}
	_, err = local.GetRootHash()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "machine")
	require.NoError(t, local.Store(path))
	machineConfig := DefaultConfig(path)
	machineConfig.ExecutionParameters.InspectIncDeadline = operationDeadline
	machineConfig.ExecutionParameters.InspectMaxDeadline = operationDeadline
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	machine, err := Load(t.Context(), logger, machineConfig)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, machine.Close()) })
	return machine
}

func TestSaturatedEndpointPreservesEmulatorCompletion(t *testing.T) {
	for _, test := range []struct {
		name    string
		start   uint64
		reason  emulator.CmioYieldReason
		halted  bool
		collect bool
		want    CompletionStatus
	}{
		{
			"accepted on last cycle", math.MaxUint64 - endpointProgramCycles,
			emulator.ManualYieldReasonAccepted, false, false, CompletionStatusAccepted,
		},
		{
			"rejected on last cycle", math.MaxUint64 - endpointProgramCycles,
			emulator.ManualYieldReasonRejected, false, false, CompletionStatusRejected,
		},
		{
			"rejected with collection", math.MaxUint64 - endpointProgramCycles,
			emulator.ManualYieldReasonRejected, false, true, CompletionStatusRejected,
		},
		{
			"overflow before yield", math.MaxUint64 - endpointProgramCycles + 1,
			emulator.ManualYieldReasonAccepted, false, false, CompletionStatusOverflow,
		},
		{"halt at maximum", math.MaxUint64, emulator.ManualYieldReasonAccepted, true, false, CompletionStatusHalted},
	} {
		t.Run(test.name, func(t *testing.T) {
			machine := loadEndpointGuest(t, test.start, test.reason, test.halted)
			before, err := machine.Hash(t.Context())
			require.NoError(t, err)
			response, err := machine.Advance(t.Context(), []byte("input"), before, test.collect)
			require.NoError(t, err)
			require.Equal(t, test.want, response.Status)
			if test.collect && test.reason == emulator.ManualYieldReasonRejected {
				require.Equal(t, Hash(model.FinalStateEntry(common.Hash(before))), response.FinalStateEntry)
			}
		})
	}
}
