// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestMain manages the node process and enforces sequential test execution.
//
// If no node is already running on port 10000 (e.g., in Docker Compose),
// TestMain starts the node binary as a subprocess, waits for health, and
// stops it after all tests complete. This makes the node lifecycle
// transparent to individual test suites — they don't need to know whether
// the node was started by the test or by an external process.
//
// Restart/snapshot tests call stopSharedNode/startSharedNode to exercise
// the node's synchronization path. When the node is externally managed
// (Compose), those tests are skipped.
func TestMain(m *testing.M) {
	var createdSnapshotsDir string
	flag.Parse()
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "skipping integration tests in short mode")
		os.Exit(0)
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

	// Start the node if none is running (local execution).
	// In Docker Compose, the node is a separate container and is already
	// running — we detect this by checking if port 10000 is in use.
	if nodePortAvailable() {
		logPath := os.Getenv("CARTESI_TEST_NODE_LOG_FILE")
		if logPath == "" {
			f, err := os.CreateTemp("", "rollups-node-integration-*.log")
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"failed to create node log file: %v\n", err)
				os.Exit(1)
			}
			logPath = f.Name()
			f.Close()
			os.Setenv("CARTESI_TEST_NODE_LOG_FILE", logPath)
		}

		// Use a temporary directory for snapshots so they don't pollute the
		// repo and are cleaned up even if the test fails.
		if os.Getenv("CARTESI_SNAPSHOTS_DIR") == "" {
			snapshotsDir, err := os.MkdirTemp("", "rollups-node-snapshots-*")
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"failed to create snapshots dir: %v\n", err)
				os.Exit(1)
			}
			os.Setenv("CARTESI_SNAPSHOTS_DIR", snapshotsDir)
			createdSnapshotsDir = snapshotsDir
			fmt.Fprintf(os.Stderr, "Snapshots dir: %s\n", snapshotsDir)
		}

		fmt.Fprintf(os.Stderr, "Starting node (log: %s)...\n", logPath)

		var err error
		sharedNode, err = startNodeWithLog(logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start node: %v\n", err)
			os.Exit(1)
		}

		ctx, cancel := context.WithTimeout(
			context.Background(), 2*time.Minute)
		if err := sharedNode.waitForHealth(ctx, nil); err != nil {
			cancel()
			sharedNode.stop(nil)
			fmt.Fprintf(os.Stderr,
				"node failed to become healthy: %v\n", err)
			os.Exit(1)
		}
		cancel()
		fmt.Fprintln(os.Stderr, "Node is healthy. Running integration tests...")
	} else {
		fmt.Fprintln(os.Stderr,
			"Node already running on port 10000 (external). "+
				"Restart tests will be skipped.")
	}

	code := m.Run()

	if sharedNode != nil {
		fmt.Fprintln(os.Stderr, "Stopping node...")
		sharedNode.stop(nil)
	}

	// Clean up snapshots directory only if we created the temp dir ourselves.
	if createdSnapshotsDir != "" {
		fmt.Fprintf(os.Stderr, "Cleaning up snapshots dir: %s\n", createdSnapshotsDir)
		os.RemoveAll(createdSnapshotsDir)
	}

	os.Exit(code)
}
