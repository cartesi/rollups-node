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
		artifactsDir, err := integrationArtifactsDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to prepare integration artifacts dir: %v\n", err)
			os.Exit(1)
		}
		os.Setenv("CARTESI_TEST_ARTIFACTS_DIR", artifactsDir)
		os.Setenv("CARTESI_TEST_NODE_WORKDIR", artifactsDir)
		fmt.Fprintf(os.Stderr, "Integration artifacts dir: %s\n", artifactsDir)

		// `make env` exports CARTESI_SNAPSHOTS_DIR=snapshots, which used to
		// resolve under test/integration because the node inherited go test's
		// package cwd. Keep user-provided custom paths, but route the default
		// snapshot path into the integration artifacts directory.
		if snapshotsDir := os.Getenv("CARTESI_SNAPSHOTS_DIR"); snapshotsDir == "" || snapshotsDir == "snapshots" {
			os.Setenv("CARTESI_SNAPSHOTS_DIR", filepath.Join(artifactsDir, "snapshots"))
		}

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

		fmt.Fprintf(os.Stderr, "Starting node (log: %s)...\n", logPath)

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

	os.Exit(code)
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
