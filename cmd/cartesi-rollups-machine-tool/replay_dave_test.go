// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayDaveInputs_PassesPinnedMarchID(t *testing.T) {
	tmp := t.TempDir()
	arguments := filepath.Join(tmp, "arguments")
	lua := filepath.Join(tmp, "lua")
	//nolint:gosec // This test-only executable records the real replay call arguments.
	require.NoError(t, os.WriteFile(lua, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$REPLAY_ARGUMENTS\"\n"), 0700))
	t.Setenv("REPLAY_ARGUMENTS", arguments)
	opts := replayOptions{Lua: lua, Template: "template with spaces", Store: "stored machine"}

	require.NoError(t, replayDaveInputs(t.Context(), opts, tmp, nil))

	raw, err := os.ReadFile(arguments)
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(tmp, "replay-dave.lua"),
		"--template", opts.Template,
		"--store", opts.Store,
		"--inputs-manifest", filepath.Join(tmp, "inputs.txt"),
		"--marchid", strconv.FormatUint(model.CartesiMachineMarchID, 10),
	}, strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"))
	embedded, err := os.ReadFile(filepath.Join(tmp, "replay-dave.lua"))
	require.NoError(t, err)
	require.Equal(t, replayDaveLua, string(embedded))
}

func TestCartesiSDKFallbackRoots_OnlyCurrentInstall(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"/opt/cartesi"}, cartesiSDKFallbackRoots)
}

func TestReplayDaveLua_ChecksMarchIDBeforeLoad(t *testing.T) {
	t.Parallel()

	check := strings.Index(replayDaveLua, "assert(\n    cartesi.MARCHID == tonumber(opts.marchid),")
	load := strings.Index(replayDaveLua, "cartesi.machine(opts.template)")
	require.NotEqual(t, -1, check, "the embedded script must assert numeric module identity")
	require.Greater(t, load, check, "refuse an incompatible module before loading the template")
	require.Contains(t, replayDaveLua, `tostring(cartesi.MARCHID), tostring(opts.marchid)`)
	require.Contains(t, replayDaveLua, `package.searchpath("cartesi", package.cpath)`)
}

func TestReplayDaveLua_RealModuleMarchID(t *testing.T) {
	lua, err := exec.LookPath("lua5.4")
	if err != nil {
		t.Skip("lua5.4 is unavailable")
	}
	env := append(os.Environ(), replayDaveLuaEnv(replayOptions{CartesiMachine: "cartesi-machine"})...)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, lua, "-e", `local c=require("cartesi");
io.write(tostring(c.MARCHID), "\n", tostring(package.searchpath("cartesi", package.cpath)), "\n")`)
	probe.Env = env
	output, err := probe.CombinedOutput()
	if err != nil && strings.Contains(string(output), "module 'cartesi' not found") {
		t.Skipf("cartesi Lua module is unavailable: %s", output)
	}
	require.NoError(t, err, "%s", output)
	module := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Len(t, module, 2)
	require.Equal(t, "22", module[0], "this fixture requires the pinned emulator module")
	require.NotEqual(t, "nil", module[1])

	tmp := t.TempDir()
	script := filepath.Join(tmp, "replay.lua")
	require.NoError(t, os.WriteFile(script, []byte(replayDaveLua), 0600))
	// Keep the real module and embedded admission code. Stop only at machine creation.
	shim := filepath.Join(tmp, "check.lua")
	require.NoError(t, os.WriteFile(shim, []byte(`local cartesi=require("cartesi")
cartesi.machine=function(template)
    assert(template == "unused-template")
    io.write("REPLAY_MACHINE_CREATION_REACHED\n")
    os.exit(0)
end
local script=table.remove(arg, 1)
dofile(script)
`), 0600))
	for _, marchID := range []string{"22", "21"} {
		t.Run(marchID, func(t *testing.T) {
			cmd := exec.CommandContext(ctx, lua, shim, script,
				"--template", "unused-template", "--store", "unused-store",
				"--inputs-manifest", "unused-inputs", "--marchid", marchID)
			cmd.Env = env
			output, err := cmd.CombinedOutput()
			if marchID == "22" {
				require.NoError(t, err, "%s", output)
				require.Equal(t, "REPLAY_MACHINE_CREATION_REACHED\n", string(output))
				return
			}
			require.Error(t, err, "an incompatible module must be refused before machine creation")
			require.Contains(t, string(output), "module MARCHID 22")
			require.Contains(t, string(output), "required MARCHID 21")
			require.Contains(t, string(output), module[1])
			require.NotContains(t, string(output), "REPLAY_MACHINE_CREATION_REACHED")
		})
	}
}

func TestDetectCartesiSDKRootFromFile_WrapperWithCartesiMachineLua_ReturnsRoot(t *testing.T) {
	t.Parallel()

	sdkRoot := filepath.Join(t.TempDir(), "cartesi-sdk")
	luaDir := filepath.Join(sdkRoot, "share", "lua", "5.4")
	wrapper := filepath.Join(t.TempDir(), "cartesi-machine")
	contents := `#!/bin/sh
export LUA_PATH_5_4="` + luaDir + `/?.lua;${LUA_PATH_5_4:-;}"
lua5.4 "` + filepath.Join(luaDir, "cartesi-machine.lua") + `" "$@"
`
	require.NoError(t, os.WriteFile(wrapper, []byte(contents), 0600))

	assert.Equal(t, sdkRoot, detectCartesiSDKRootFromFile(wrapper))
}

func TestPrependLuaSearchPath_EmptyPath_KeepsLuaDefaultFallback(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/opt/cartesi-sdk21/share/lua/5.4/?.lua;;",
		prependLuaSearchPath("", "/opt/cartesi-sdk21/share/lua/5.4/?.lua"))
}

func TestPrependLuaSearchPath_ExistingPath_PrependsWithoutDroppingExisting(t *testing.T) {
	t.Parallel()

	got := prependLuaSearchPath("/custom/?.lua", "/opt/cartesi-sdk21/share/lua/5.4/?.lua")

	assert.Equal(t, "/opt/cartesi-sdk21/share/lua/5.4/?.lua;/custom/?.lua", got)
}

func TestPrependLuaSearchPath_AlreadyPresent_DoesNotDuplicate(t *testing.T) {
	t.Parallel()

	path := "/opt/cartesi-sdk21/share/lua/5.4/?.lua;/custom/?.lua"

	assert.Equal(t, path, prependLuaSearchPath(path, "/opt/cartesi-sdk21/share/lua/5.4/?.lua"))
}

func TestReplayDaveLuaEnv_AutoDetectsSDKRootFromCartesiMachineWrapper(t *testing.T) {
	sdkRoot := filepath.Join(t.TempDir(), "cartesi-sdk")
	luaDir := filepath.Join(sdkRoot, "share", "lua", "5.4")
	wrapper := filepath.Join(t.TempDir(), "cartesi-machine")
	contents := `#!/bin/sh
export LUA_PATH_5_4="` + luaDir + `/?.lua;${LUA_PATH_5_4:-;}"
export LUA_CPATH_5_4="` + filepath.Join(sdkRoot, "lib", "lua", "5.4") + `/?.so;${LUA_CPATH_5_4:-;}"
lua5.4 "` + filepath.Join(luaDir, "cartesi-machine.lua") + `" "$@"
`
	require.NoError(t, os.WriteFile(wrapper, []byte(contents), 0600))
	t.Setenv("LUA_PATH_5_4", "/custom/?.lua")
	t.Setenv("LUA_CPATH_5_4", "")
	t.Setenv("LUA_PATH", "")
	t.Setenv("LUA_CPATH", "")
	t.Setenv("CARTESI_IMAGES_PATH", "")

	env := replayDaveLuaEnv(replayOptions{CartesiMachine: wrapper})
	envByKey := envMap(env)

	assert.Equal(t, filepath.Join(luaDir, "?.lua")+";/custom/?.lua", envByKey["LUA_PATH_5_4"])
	assert.Equal(t, filepath.Join(sdkRoot, "lib", "lua", "5.4", "?.so")+";;", envByKey["LUA_CPATH_5_4"])
	assert.Equal(t, filepath.Join(luaDir, "?.lua")+";;", envByKey["LUA_PATH"])
	assert.Equal(t, filepath.Join(sdkRoot, "share", "cartesi-machine", "images"), envByKey["CARTESI_IMAGES_PATH"])
}

func TestWriteReplayInputManifest_WritesPayloadsAndManifest(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	inputs := []*model.Input{
		{RawData: []byte{0x01, 0x02}},
		{RawData: []byte("cartesi")},
	}

	manifest, err := writeReplayInputManifest(tmp, inputs)
	require.NoError(t, err)

	raw, err := os.ReadFile(manifest)
	require.NoError(t, err)
	paths := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, paths, len(inputs))
	for i, path := range paths {
		payload, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, inputs[i].RawData, payload)
	}
}

func envMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, item := range env {
		key, value, _ := strings.Cut(item, "=")
		result[key] = value
	}
	return result
}
