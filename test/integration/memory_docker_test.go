// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Opt in because this test needs a local Docker daemon and an existing runtime
// image. It also runs without the endtoendtests tag or blockchain services.
func TestLimitedNodeDocker(t *testing.T) {
	if os.Getenv("CARTESI_TEST_NODE_DOCKER_TEST") != "1" {
		t.Skip("set CARTESI_TEST_NODE_DOCKER_TEST=1 to validate the Docker launcher")
	}
	t.Setenv("CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES", "1073741824")
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe")
	require.NoError(t, os.WriteFile(probe, []byte(`#!/bin/sh
set -eu
cat /sys/fs/cgroup/memory.max /sys/fs/cgroup/memory.swap.max
printf '%s\n' "$NODE_PROBE_VALUE" "$PWD"
test "$$" = 1
`), 0755))
	env := append(os.Environ(), "NODE_PROBE_VALUE=line one\nline two")
	cmd, err := limitedNodeCommand(probe, io.Discard, env, dir)
	require.NoError(t, err)
	var buffer bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buffer, &buffer
	first, err := startNodeSubprocess("docker-probe", cmd)
	require.NoError(t, err)
	require.True(t, first.wait(15*time.Second))
	output := buffer.Bytes()
	require.NoError(t, first.waitErr, "%s", output)
	require.Equal(t, "1073741824\n0\nline one\nline two\n"+dir+"\n", string(output))

	// Killing docker run must not leave the node container running.
	require.NoError(t, os.WriteFile(probe, []byte("#!/bin/sh\nexec sleep 300\n"), 0755))
	cmd, err = limitedNodeCommand(probe, io.Discard, env, dir)
	require.NoError(t, err)
	name := nodeContainerName(cmd)
	proc, err := startNodeSubprocess("docker-probe", cmd)
	require.NoError(t, err)
	t.Cleanup(func() {
		if !proc.isDone() {
			_ = cmd.Process.Kill()
		}
		require.True(t, proc.wait(15*time.Second))
	})
	require.Eventually(t, func() bool {
		data, err := exec.Command(cmd.Path, "inspect", "--format={{.State.Running}}", name).Output()
		return err == nil && bytes.Equal(bytes.TrimSpace(data), []byte("true"))
	}, 15*time.Second, 100*time.Millisecond)
	require.NoError(t, cmd.Process.Kill())
	require.True(t, proc.wait(15*time.Second), "client and container must be cleaned up")
	require.Error(t, exec.Command(cmd.Path, "inspect", name).Run(), "container must be removed")
}
