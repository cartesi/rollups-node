// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const replayStoredRootOffset = int64(0x5c)

// createReplayStoreTemplate builds a machine template that accepts one input and
// changes registers and RX memory, mirroring the legacy Lua fixture. It is
// stored at a manual accepted yield so machine.Advance can process an input.
func createReplayStoreTemplate(t *testing.T) string {
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

	yield := emulator.HtifDeviceYield<<emulator.HtifDeviceShift |
		uint64(emulator.YieldManual)<<emulator.HtifCommandShift |
		uint64(emulator.ManualYieldReasonAccepted)<<emulator.HtifReasonShift | machine.HashSize
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
	// Refresh the root so the stored template header is current.
	_, err = local.GetRootHash()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "template")
	require.NoError(t, local.Store(path))
	return path
}

// TestReplayStoreKeepsFinalRoot is the native-runtime port of the legacy Lua
// regression. It verifies that Machine.Store persists the final execution root in
// the SHT header: the raw stored root (read before any reload can recompute
// dirty nodes) must match the final root, survive a reload, and leave the source
// template unchanged.
func TestReplayStoreKeepsFinalRoot(t *testing.T) {
	ctx := t.Context()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	templateDir := createReplayStoreTemplate(t)
	templateFiles := replayArchiveHashes(t, templateDir)

	m, err := machine.Load(ctx, logger, machine.DefaultConfig(templateDir))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })

	checkpoint, err := m.Hash(ctx)
	require.NoError(t, err)
	require.Equal(t, readReplayStoredRoot(t, templateDir), checkpoint,
		"template SHT header root must match the loaded template root")

	input := []byte("one accepted input")
	_, err = m.Advance(ctx, input, checkpoint, false)
	require.NoError(t, err)

	// Refresh the root before storing, matching runReplay, so the persisted SHT
	// header matches the final execution root; a reload would otherwise be
	// required to recompute it.
	finalRoot, err := m.Hash(ctx)
	require.NoError(t, err)
	require.NotEqual(t, checkpoint, finalRoot, "the fixture must change the machine state")

	storeDir := filepath.Join(t.TempDir(), "replayed")
	require.NoError(t, m.Store(ctx, storeDir))

	// Inspect persisted metadata before any reload can recompute dirty nodes.
	storedRoot := readReplayStoredRoot(t, storeDir)
	assert.Equal(t, finalRoot, storedRoot, "stored SHT header root must match the final replay root before reload")

	// Keep this diagnostic after a failed header assertion: reload can conceal it.
	reloaded, err := machine.Load(ctx, logger, machine.DefaultConfig(storeDir))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reloaded.Close()) })
	reloadedRoot, err := reloaded.Hash(ctx)
	require.NoError(t, err)
	assert.Equal(t, finalRoot, reloadedRoot, "stored machine root must survive a reload")

	assert.Equal(t, templateFiles, replayArchiveHashes(t, templateDir), "source template must remain intact")
	t.Logf("template_root=%x final_root=%x raw_stored_root=%x reloaded_root=%x",
		checkpoint, finalRoot, storedRoot, reloadedRoot)
}

func readReplayStoredRoot(t *testing.T, directory string) machine.Hash {
	t.Helper()
	file, err := os.Open(filepath.Join(directory, "hash_tree.sht"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, file.Close()) }()
	var root machine.Hash
	n, err := file.ReadAt(root[:], replayStoredRootOffset)
	require.NoError(t, err)
	require.Equal(t, len(root), n)
	return root
}

func replayArchiveHashes(t *testing.T, directory string) map[string][32]byte {
	t.Helper()
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	defer func() { assert.NoError(t, root.Close()) }()
	hashes := make(map[string][32]byte)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := root.ReadFile(path)
		require.NoError(t, err)
		hashes[path] = sha256.Sum256(raw)
		return nil
	})
	require.NoError(t, err)
	return hashes
}
