// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateBinding_SharedWorktreePermissions(t *testing.T) {
	t.Chdir(t.TempDir())
	// Keep this test serial: both Chdir and Umask change process-wide state.
	// The usual umask removes group write during creation. Generation must
	// restore it so another user in the same group can regenerate the package.
	const sourceCreationMask = 0022
	previousMask := syscall.Umask(sourceCreationMask)
	t.Cleanup(func() { syscall.Umask(previousMask) })
	const artifact = `{
		"abi": [{
			"type": "function",
			"name": "value",
			"inputs": [],
			"outputs": [{"name": "", "type": "uint256"}],
			"stateMutability": "view"
		}]
	}`

	for _, name := range []string{"first generation", "regeneration"} {
		t.Run(name, func(t *testing.T) {
			generateBinding(contractBinding{typeName: "TestContract"}, []byte(artifact))

			directory, err := os.Stat("testcontract")
			require.NoError(t, err)
			require.True(t, directory.IsDir())
			const sharedDirectoryBits os.FileMode = 0775
			require.Equal(t, sharedDirectoryBits, directory.Mode().Perm()&sharedDirectoryBits,
				"group members must be able to regenerate the publicly readable package")

			binding, err := os.Stat(filepath.Join("testcontract", "testcontract.go"))
			require.NoError(t, err)
			require.True(t, binding.Mode().IsRegular())
			const sharedSourceBits os.FileMode = 0664
			require.Equal(t, sharedSourceBits, binding.Mode().Perm()&sharedSourceBits,
				"group members must be able to edit the publicly readable Go source")
			const executeBits os.FileMode = 0111
			require.Zero(t, binding.Mode().Perm()&executeBits, "generated Go source must not be executable")
		})
	}
}
