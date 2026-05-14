// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cartesi/rollups-node/internal/model"
)

//go:embed replay_dave.lua
var replayDaveLua string

func replayDaveInputs(ctx context.Context, opts replayOptions, tmp string, inputs []*model.Input) error {
	manifest, err := writeReplayInputManifest(tmp, inputs)
	if err != nil {
		return err
	}
	script := filepath.Join(tmp, "replay-dave.lua")
	if err := os.WriteFile(script, []byte(replayDaveLua), 0600); err != nil {
		return fmt.Errorf("write Dave replay Lua script: %w", err)
	}

	args := []string{
		script,
		"--template", opts.Template,
		"--store", opts.Store,
		"--inputs-manifest", manifest,
	}
	return runCommandWithEnv(ctx, opts.Lua, replayDaveLuaEnv(opts), args...)
}

func writeReplayInputManifest(tmp string, inputs []*model.Input) (string, error) {
	paths, err := writeReplayInputFiles(tmp, inputs)
	if err != nil {
		return "", err
	}
	manifest := filepath.Join(tmp, "inputs.txt")
	var contents strings.Builder
	for _, path := range paths {
		contents.WriteString(path)
		contents.WriteByte('\n')
	}
	if err := os.WriteFile(manifest, []byte(contents.String()), 0600); err != nil {
		return "", fmt.Errorf("write Dave replay input manifest: %w", err)
	}
	return manifest, nil
}

func writeReplayInputFiles(tmp string, inputs []*model.Input) ([]string, error) {
	paths := make([]string, len(inputs))
	for i, input := range inputs {
		path := filepath.Join(tmp, fmt.Sprintf("input-%d.bin", i))
		if err := os.WriteFile(path, input.RawData, 0600); err != nil {
			return nil, fmt.Errorf("write replay input %d: %w", i, err)
		}
		paths[i] = path
	}
	return paths, nil
}

func replayDaveLuaEnv(opts replayOptions) []string {
	sdkRoot := opts.CartesiSDKRoot
	if sdkRoot == "" {
		sdkRoot = detectCartesiSDKRoot(opts.CartesiMachine)
	}
	if sdkRoot == "" {
		return nil
	}

	luaPath := filepath.Join(sdkRoot, "share", "lua", "5.4", "?.lua")
	luaCPath := filepath.Join(sdkRoot, "lib", "lua", "5.4", "?.so")
	env := []string{
		"LUA_PATH_5_4=" + prependLuaSearchPath(os.Getenv("LUA_PATH_5_4"), luaPath),
		"LUA_CPATH_5_4=" + prependLuaSearchPath(os.Getenv("LUA_CPATH_5_4"), luaCPath),
		"LUA_PATH=" + prependLuaSearchPath(os.Getenv("LUA_PATH"), luaPath),
		"LUA_CPATH=" + prependLuaSearchPath(os.Getenv("LUA_CPATH"), luaCPath),
	}
	if os.Getenv("CARTESI_IMAGES_PATH") == "" {
		env = append(env, "CARTESI_IMAGES_PATH="+filepath.Join(sdkRoot, "share", "cartesi-machine", "images"))
	}
	return env
}

func prependLuaSearchPath(current string, entry string) string {
	if current == "" {
		return entry + ";;"
	}
	if luaSearchPathContains(current, entry) {
		return current
	}
	return entry + ";" + current
}

func luaSearchPathContains(path string, entry string) bool {
	for _, part := range strings.Split(path, ";") {
		if part == entry {
			return true
		}
	}
	return false
}

func detectCartesiSDKRoot(cartesiMachine string) string {
	path, err := exec.LookPath(cartesiMachine)
	if err != nil {
		path = cartesiMachine
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	if root := detectCartesiSDKRootFromFile(path); root != "" {
		return root
	}
	for _, root := range []string{"/opt/cartesi-sdk21", "/opt/cartesi"} {
		if fileExists(filepath.Join(root, "share", "lua", "5.4", "cartesi.lua")) {
			return root
		}
	}
	return ""
}

var (
	cartesiMachineLuaPattern = regexp.MustCompile(`["']([^"']*/share/lua/5\.4/cartesi-machine\.lua)["']`)
	cartesiLuaPathPattern    = regexp.MustCompile(`["']?([^"'\s;]*/share/lua/5\.4)/\?\.lua`)
)

func detectCartesiSDKRootFromFile(path string) string {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return ""
	}
	text := string(raw)
	if match := cartesiMachineLuaPattern.FindStringSubmatch(text); len(match) == 2 {
		return strings.TrimSuffix(match[1], "/share/lua/5.4/cartesi-machine.lua")
	}
	if match := cartesiLuaPathPattern.FindStringSubmatch(text); len(match) == 2 {
		return strings.TrimSuffix(match[1], "/share/lua/5.4")
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
