// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const machineToolBinary = "cartesi-rollups-machine-tool"

func runMachineTool(ctx context.Context, t testing.TB, args ...string) string {
	t.Helper()
	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, machineToolBinary, args...)
	cmd.Env = os.Environ()
	workDir := machineToolWorkDir(t)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		require.NoError(t, err, "%s %v failed: %s", machineToolBinary, args, stderr)
	}
	return string(out)
}

func machineToolWorkDir(t testing.TB) string {
	t.Helper()
	artifactsDir := os.Getenv("CARTESI_TEST_ARTIFACTS_DIR")
	if artifactsDir == "" {
		var err error
		artifactsDir, err = integrationArtifactsDir()
		require.NoError(t, err, "prepare integration artifacts dir")
		os.Setenv("CARTESI_TEST_ARTIFACTS_DIR", artifactsDir)
	}

	workDir := filepath.Join(artifactsDir, "machine-tool")
	require.NoError(t, os.MkdirAll(workDir, 0755), "create machine-tool artifact dir") //nolint:mnd
	return workDir
}
