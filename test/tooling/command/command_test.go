// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package command

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	grandchildProcess  = "COMMAND_TEST_GRANDCHILD_PROCESS"
	grandchildCommand  = "hold-output-pipes"
	grandchildLifetime = 5 * time.Second
)

var inheritedCredentialKeys = []string{
	"CARTESI_AUTH_PRIVATE_KEY",
	"AWS_SECRET_ACCESS_KEY",
	"PGPASSWORD",
	"UNRELATED_PROVIDER_TOKEN",
}

func TestSecretCommandProcess(t *testing.T) {
	root := &cobra.Command{Use: "cartesi-command-helper"}
	Child(t, func() {
		if len(os.Args) > 1 && os.Args[1] == grandchildCommand {
			executable, err := os.Executable()
			require.NoError(t, err)
			grandchild := exec.Command(executable, "-test.run=^TestOutputPipeGrandchildProcess$")
			grandchild.Env = append(os.Environ(), grandchildProcess+"=1")
			grandchild.Stdout, grandchild.Stderr = os.Stdout, os.Stderr
			require.NoError(t, grandchild.Start())
			fmt.Printf("grandchild pid: %d\n", grandchild.Process.Pid)
			require.NoError(t, grandchild.Process.Release())
			return
		}
		for _, key := range inheritedCredentialKeys {
			_, present := os.LookupEnv(key)
			fmt.Printf("credential inherited %s: %t\n", key, present)
		}
		fmt.Println("explicit input:", os.Getenv("COMMAND_TEST_INPUT"))
	}, root)
}

func TestOutputPipeGrandchildProcess(t *testing.T) {
	if os.Getenv(grandchildProcess) != "1" {
		t.Skip("only runs in an output-pipe grandchild")
	}
	// The grandchild inherits the command's output descriptors after its parent
	// exits. The harness must return before these descriptors close naturally.
	time.Sleep(grandchildLifetime)
}

func TestRunBoundsOutputWaitAfterChildExit(t *testing.T) {
	stdout, stderr, err := Run(t, nil, grandchildCommand)
	pidLine, _, _ := strings.Cut(stdout, "\n")
	pid, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(pidLine, "grandchild pid: ")))
	require.NoError(t, parseErr, stdout+stderr)
	require.Positive(t, pid)
	grandchild, findErr := os.FindProcess(pid)
	require.NoError(t, findErr)
	t.Cleanup(func() {
		_ = grandchild.Kill()
		_ = grandchild.Release()
	})
	require.ErrorIs(t, err, exec.ErrWaitDelay, "output wait must stop before the grandchild exits")
}

func TestRunIsolatesEnvironmentAndCleansMarkdown(t *testing.T) {
	for _, key := range inheritedCredentialKeys {
		t.Setenv(key, "SYNTHETIC_INHERITED_CREDENTIAL")
	}
	stdout, stderr, err := Run(t, []string{"COMMAND_TEST_INPUT=present"})
	require.NoError(t, err, stderr)
	for _, key := range inheritedCredentialKeys {
		require.Contains(t, stdout, "credential inherited "+key+": false")
	}
	require.Contains(t, stdout, "explicit input: present")

	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Run("markdown", func(t *testing.T) {
		stdout, stderr, err := Run(t, nil, markdownCommand)
		require.NoError(t, err, stderr)
		require.Contains(t, stdout, "cartesi-command-helper")
	})
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "child-owned temporary directories survived parent cleanup")
}
