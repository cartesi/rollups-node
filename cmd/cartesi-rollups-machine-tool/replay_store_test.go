// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const replayStoredRootOffset = int64(0x60)

// The guest accepts one input and changes registers and RX memory. The reference
// uses a separate real machine, so reading its root cannot refresh the replay.
const replayStoreFixtureLua = `local cartesi = require("cartesi")
local function hex(data)
    return (data:gsub(".", function(c) return string.format("%02x", string.byte(c)) end))
end
local mode, directory = arg[1], arg[2]
if mode == "create" then
    local machine <close> = cartesi.machine({
        processor = {registers = {pc = 0x80000000}},
        ram = {length = 4096},
        cmio = {rx_buffer = {}, tx_buffer = {}},
    })
    machine:write_memory(0x80000000, string.pack("<I4I4I4I4I4I4I4I4",
        0x000a3023, 0x000a3423, 0x000a3823, 0x000a3c23,
        0x400082b7, 0x0002b423, 0x0132b023, 0xfe5ff06f))
    local request = (cartesi.HTIF_DEV_YIELD << 56) | (cartesi.HTIF_YIELD_CMD_MANUAL << 48)
        | (cartesi.HTIF_YIELD_MANUAL_REASON_RX_ACCEPTED << 32) | 32
    machine:write_reg("x19", request)
    machine:write_reg("x20", 0x60800000)
    machine:write_reg("iflags_Y", 1)
    machine:write_reg("htif_tohost_dev", cartesi.HTIF_DEV_YIELD)
    machine:write_reg("htif_tohost_cmd", cartesi.HTIF_YIELD_CMD_MANUAL)
    machine:write_reg("htif_tohost_reason", cartesi.HTIF_YIELD_MANUAL_REASON_RX_ACCEPTED)
    machine:write_reg("htif_tohost_data", 32)
    local root = machine:get_root_hash()
    machine:store(directory)
    io.write(hex(root), "\n")
elseif mode == "reference" then
    local machine <close> = cartesi.machine(directory)
    local input <close> = assert(io.open(arg[3], "rb"))
    local data = assert(input:read("*a"))
    local checkpoint = machine:get_root_hash()
    machine:send_cmio_response(cartesi.HTIF_YIELD_REASON_ADVANCE_STATE, data, checkpoint)
    assert(machine:run(math.maxinteger) == cartesi.BREAK_REASON_YIELDED_MANUALLY)
    local command, reason, response = machine:receive_cmio_request()
    assert(command == cartesi.HTIF_YIELD_CMD_MANUAL)
    assert(reason == cartesi.HTIF_YIELD_MANUAL_REASON_RX_ACCEPTED)
    assert(#response == 32)
    io.write(hex(machine:get_root_hash()), "\n")
elseif mode == "reload" then
    local machine <close> = cartesi.machine(directory)
    io.write(hex(machine:get_root_hash()), "\n")
else
    error("unknown fixture mode")
end
`

func TestReplayDaveLua_StoreKeepsFinalRoot(t *testing.T) {
	lua, err := exec.LookPath("lua5.4")
	if err != nil {
		t.Skip("lua5.4 is unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	opts := replayOptions{Lua: lua, CartesiMachine: "cartesi-machine"}
	env := append(os.Environ(), replayDaveLuaEnv(opts)...)
	probe := exec.CommandContext(ctx, lua, "-e", `local c=require("cartesi"); print(c.MARCHID)`)
	probe.Env = env
	output, err := probe.CombinedOutput()
	if err != nil && strings.Contains(string(output), "module 'cartesi' not found") {
		t.Skipf("cartesi Lua module is unavailable: %s", output)
	}
	require.NoError(t, err, "%s", output)
	require.Equal(t, "22", strings.TrimSpace(string(output)))

	tmp := t.TempDir()
	fixture := filepath.Join(tmp, "fixture.lua")
	require.NoError(t, os.WriteFile(fixture, []byte(replayStoreFixtureLua), 0600))
	opts.Template, opts.Store = filepath.Join(tmp, "template"), filepath.Join(tmp, "replayed")
	templateRoot := runReplayStoreFixture(ctx, t, env, fixture, "create", opts.Template)
	require.Equal(t, templateRoot, readReplayStoredRoot(t, opts.Template))
	templateFiles := replayArchiveHashes(t, opts.Template)
	input := []byte("one accepted input")
	inputPath := filepath.Join(tmp, "input.bin")
	require.NoError(t, os.WriteFile(inputPath, input, 0600))
	finalRoot := runReplayStoreFixture(ctx, t, env, fixture, "reference", opts.Template, inputPath)
	require.NotEqual(t, templateRoot, finalRoot, "the fixture must change the machine state")

	require.NoError(t, replayDaveInputs(ctx, opts, tmp, []*model.Input{{RawData: input}}))
	// Inspect persisted metadata before any reload can recompute dirty nodes.
	storedRoot := readReplayStoredRoot(t, opts.Store)
	assert.Equal(t, finalRoot, storedRoot, "stored SHT header root must match the final replay root before reload")
	// Keep this diagnostic after a failed header assertion: reload can conceal it.
	reloadedRoot := runReplayStoreFixture(ctx, t, env, fixture, "reload", opts.Store)
	assert.Equal(t, finalRoot, reloadedRoot)
	unchanged := assert.Equal(t, templateFiles, replayArchiveHashes(t, opts.Template), "source template must remain intact")
	t.Logf("template_root=%x reference_final_root=%x raw_stored_root=%x reloaded_root=%x template_unchanged=%t",
		templateRoot, finalRoot, storedRoot, reloadedRoot, unchanged)
}

func runReplayStoreFixture(ctx context.Context, t *testing.T, env []string, fixture string, args ...string) [32]byte {
	t.Helper()
	cmd := exec.CommandContext(ctx, "lua5.4", append([]string{fixture}, args...)...) //nolint:gosec // Only private test fixtures.
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	decoded, err := hex.DecodeString(strings.TrimSpace(string(output)))
	require.NoError(t, err)
	var root [32]byte
	require.Len(t, decoded, len(root))
	copy(root[:], decoded)
	return root
}

func readReplayStoredRoot(t *testing.T, directory string) [32]byte {
	t.Helper()
	file, err := os.Open(filepath.Join(directory, "hash_tree.sht"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, file.Close()) }()
	var root [32]byte
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
		if err != nil {
			return err
		}
		hashes[path] = sha256.Sum256(raw)
		return nil
	})
	require.NoError(t, err)
	return hashes
}
