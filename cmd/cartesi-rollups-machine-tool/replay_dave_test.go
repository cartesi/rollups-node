// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
