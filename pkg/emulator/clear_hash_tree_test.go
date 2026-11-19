// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clearing discards the cached hash tree; the next hash request must rebuild
// it lazily from machine state and reproduce the same root.
func TestClearHashTree_RebuildsLazyWithoutChangingState(t *testing.T) {
	machine := createLoopMachine(t)

	breakReason, err := machine.Run(65)
	require.NoError(t, err)
	require.Equal(t, BreakReasonReachedTargetMcycle, breakReason)

	before, err := machine.GetRootHash()
	require.NoError(t, err)
	mcycleBefore, err := machine.ReadReg(REG_MCYCLE)
	require.NoError(t, err)

	// Clearing is idempotent and must not fail on a machine with a populated cache.
	require.NoError(t, machine.ClearHashTree())
	require.NoError(t, machine.ClearHashTree())

	after, err := machine.GetRootHash()
	require.NoError(t, err)
	require.Equal(t, before, after)

	// Machine state is untouched by the clear.
	mcycleAfter, err := machine.ReadReg(REG_MCYCLE)
	require.NoError(t, err)
	require.Equal(t, mcycleBefore, mcycleAfter)

	// Node hashes and proofs still work after the clear (lazy rebuild).
	node, err := machine.GetNodeHash(UarchStateStartAddress, UarchStateLog2Size)
	require.NoError(t, err)
	require.NotEqual(t, Hash{}, node)
}

// The runtime configuration field must be accepted by the emulator and reported
// in the applied configuration.
func TestClearHashTreeRuntimeConfig(t *testing.T) {
	cfg := NewMachineRuntimeConfig()
	cfg.ClearHashTree = true
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	var marshaled map[string]any
	require.NoError(t, json.Unmarshal(raw, &marshaled))
	require.Equal(t, true, marshaled["clear_hash_tree"])

	machine := createLoopMachineWithRuntime(t, string(raw))

	applied, err := machine.GetRuntimeConfig()
	require.NoError(t, err)
	var appliedCfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(applied), &appliedCfg))
	require.Equal(t, true, appliedCfg["clear_hash_tree"])
}

// The JSON-RPC path (protocol 0.9.0) must serve machine.clear_hash_tree for a
// loaded machine on a spawned server.
func TestClearHashTree_Remote(t *testing.T) {
	const timeout = 5 * time.Second

	local := createLoopMachine(t)
	breakReason, err := local.Run(65)
	require.NoError(t, err)
	require.Equal(t, BreakReasonReachedTargetMcycle, breakReason)
	localRoot, err := local.GetRootHash()
	require.NoError(t, err)

	// The emulator's store creates the directory itself, so it must not pre-exist.
	dir := filepath.Join(t.TempDir(), "stored")
	require.NoError(t, local.Store(dir))

	remote, _, _, err := SpawnServer("127.0.0.1:0", timeout)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, remote.ShutdownServer())
		remote.Delete()
	})
	require.NoError(t, remote.SetTimeout(timeout.Milliseconds()))
	require.NoError(t, remote.Load(dir, ""))
	require.NoError(t, remote.ClearHashTree())

	remoteRoot, err := remote.GetRootHash()
	require.NoError(t, err)
	require.Equal(t, localRoot, remoteRoot)
}
