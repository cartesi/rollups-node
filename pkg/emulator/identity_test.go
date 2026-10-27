// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package emulator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMarchID(t *testing.T) {
	marchID, err := DefaultMarchID()
	require.NoError(t, err)
	require.Equal(t, MarchID, marchID)
}

func TestUarchIdentityHash(t *testing.T) {
	pristine, err := GetUarchPristineHash()
	require.NoError(t, err)
	machine := createLoopMachine(t)
	localPristine, err := machine.GetUarchPristineHash()
	require.NoError(t, err)
	require.Equal(t, pristine, localPristine)
	node, err := machine.GetNodeHash(UarchStateStartAddress, UarchStateLog2Size)
	require.NoError(t, err)
	require.Equal(t, pristine, node)

	require.NoError(t, machine.WriteReg(REG_UARCH_X1, 1))
	node, err = machine.GetNodeHash(UarchStateStartAddress, UarchStateLog2Size)
	require.NoError(t, err)
	require.NotEqual(t, pristine, node)
	localPristine, err = machine.GetUarchPristineHash()
	require.NoError(t, err)
	require.Equal(t, pristine, localPristine)
}

func TestUarchIdentityHashFromServer(t *testing.T) {
	const timeout = 5 * time.Second
	remote, _, _, err := SpawnServer("127.0.0.1:0", timeout)
	require.NoError(t, err)
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			assert.NoError(t, remote.ShutdownServer())
		}
		remote.Delete()
	})
	require.NoError(t, remote.SetTimeout(timeout.Milliseconds()))
	expected, err := GetUarchPristineHash()
	require.NoError(t, err)
	actual, err := remote.GetUarchPristineHash()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	require.NoError(t, remote.ShutdownServer())
	stopped = true
	actual, err = remote.GetUarchPristineHash()
	require.Error(t, err)
	require.Zero(t, actual)
}
