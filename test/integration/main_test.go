// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain manages the node and enforces sequential test execution.
//
// Unless a node is already running on port 10000, TestMain starts the
// test-managed node — the all-in-one process (standalone) or the service
// subprocesses (multiprocess, NODE_TOPOLOGY=multiprocess) — waits for health,
// and stops it after all tests complete. This keeps the node lifecycle
// transparent to the suites.
//
// Restart/snapshot tests call stopSharedNode/startSharedNode to exercise the
// node's synchronization path; this works under either topology. They are
// skipped only when an external node is already running (not test-managed).
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "skipping integration tests in short mode")
		os.Exit(0)
	}

	// -list only builds and lists tests; skip node management entirely.
	if l := flag.Lookup("test.list"); l != nil && l.Value.String() != "" {
		os.Exit(m.Run())
	}

	// Enforce sequential execution — tests share blockchain state.
	p := flag.Lookup("test.parallel")
	if p != nil && p.Value.String() != "1" {
		fmt.Fprintln(os.Stderr,
			"WARNING: integration tests must not run in parallel "+
				"(-test.parallel should be 1). Forcing -test.parallel=1.")
		if err := p.Value.Set("1"); err != nil {
			fmt.Fprintf(os.Stderr,
				"failed to set -test.parallel=1: %v\n", err)
			os.Exit(1)
		}
	}

	// The node is started here by TestMain (the Compose integration service runs
	// this same test binary) unless a developer already has one running on
	// :10000, in which case we attach and the restart tests are skipped. The
	// multiprocess topology starts the services as subprocesses — host or inside
	// the test container — so the lifecycle tests can restart them too.
	nodeTopology = envOrDefault("NODE_TOPOLOGY", "standalone")
	healthTimeout := 2 * time.Minute

	mustPrepareRuntime := func() string {
		logPath, err := prepareNodeRuntime()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to prepare node runtime: %v\n", err)
			os.Exit(1)
		}
		return logPath
	}

	bringUp := func(h nodeHandle, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start node: %v\n", err)
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
		if err := h.waitForHealth(ctx, nil); err != nil {
			cancel()
			h.stop(nil)
			fmt.Fprintf(os.Stderr, "node failed to become healthy: %v\n", err)
			os.Exit(1)
		}
		cancel()
		sharedNode = h
		fmt.Fprintln(os.Stderr, "Node is healthy. Running integration tests...")
	}

	switch nodeTopology {
	case "multiprocess":
		logPath := mustPrepareRuntime()
		fmt.Fprintf(os.Stderr, "Starting multiprocess node (log: %s)...\n", logPath)
		bringUp(startMultiNode(logPath))
	case "standalone":
		if nodePortAvailable() {
			logPath := mustPrepareRuntime()
			fmt.Fprintf(os.Stderr, "Starting node (log: %s)...\n", logPath)
			bringUp(startNodeWithLog(logPath))
		} else {
			fmt.Fprintln(os.Stderr,
				"Node already running on :10000 (external). Restart tests will be skipped.")
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown NODE_TOPOLOGY %q (expected standalone or multiprocess)\n",
			nodeTopology)
		os.Exit(1)
	}

	code := m.Run()

	if sharedNode != nil {
		if checker, ok := sharedNode.(nodeExitChecker); ok {
			if err := checker.exitedProcessError(); err != nil {
				fmt.Fprintf(os.Stderr, "node subprocess exited unexpectedly: %v\n", err)
				if code == 0 {
					code = 1
				}
			}
		}
		fmt.Fprintln(os.Stderr, "Stopping node...")
		sharedNode.stop(nil)
	}

	os.Exit(code)
}

// prepareNodeRuntime sets up the artifacts dir, snapshot dir, and node log
// file shared by the standalone and host-multiprocess paths, returning the
// log path.
func prepareNodeRuntime() (string, error) {
	artifactsDir, err := integrationArtifactsDir()
	if err != nil {
		return "", fmt.Errorf("prepare integration artifacts dir: %w", err)
	}
	os.Setenv("CARTESI_TEST_ARTIFACTS_DIR", artifactsDir)
	os.Setenv("CARTESI_TEST_NODE_WORKDIR", artifactsDir)
	fmt.Fprintf(os.Stderr, "Integration artifacts dir: %s\n", artifactsDir)

	// `make env` exports CARTESI_SNAPSHOTS_DIR=snapshots, which used to resolve
	// under test/integration because the node inherited go test's package cwd.
	// Keep user-provided custom paths, but route the default into the artifacts dir.
	if snapshotsDir := os.Getenv("CARTESI_SNAPSHOTS_DIR"); snapshotsDir == "" || snapshotsDir == "snapshots" {
		os.Setenv("CARTESI_SNAPSHOTS_DIR", filepath.Join(artifactsDir, "snapshots"))
	}

	logPath := os.Getenv("CARTESI_TEST_NODE_LOG_FILE")
	if logPath == "" {
		f, err := os.CreateTemp("", "rollups-node-integration-*.log")
		if err != nil {
			return "", fmt.Errorf("create node log file: %w", err)
		}
		logPath = f.Name()
		f.Close()
		os.Setenv("CARTESI_TEST_NODE_LOG_FILE", logPath)
	}
	return logPath, nil
}

func integrationArtifactsDir() (string, error) {
	if dir := os.Getenv("CARTESI_TEST_ARTIFACTS_DIR"); dir != "" {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("resolve CARTESI_TEST_ARTIFACTS_DIR: %w", err)
		}
		if err := os.MkdirAll(absDir, 0755); err != nil { //nolint:mnd
			return "", fmt.Errorf("create CARTESI_TEST_ARTIFACTS_DIR: %w", err)
		}
		return absDir, nil
	}

	root, err := repositoryRoot()
	if err != nil {
		return "", err
	}
	baseDir := filepath.Join(root, "applications", "integration-artifacts")
	if err := os.MkdirAll(baseDir, 0755); err != nil { //nolint:mnd
		return "", fmt.Errorf("create default integration artifacts base dir: %w", err)
	}
	dir, err := os.MkdirTemp(baseDir, "run-*")
	if err != nil {
		return "", fmt.Errorf("create default integration artifacts dir: %w", err)
	}
	return dir, nil
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat go.mod: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found from %s", dir)
		}
		dir = parent
	}
}
