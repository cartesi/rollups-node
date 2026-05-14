// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

const nodeBinary = "cartesi-rollups-node"

// sharedNode is the test-managed node process, started by TestMain when no
// external node is running. All test suites share this instance. Restart
// tests stop and restart it via stopSharedNode/startSharedNode.
// When nil, the node is externally managed (e.g., Docker Compose) and
// restart tests are skipped.
var sharedNode *nodeProcess

// isNodeSelfManaged returns true if TestMain started the node process.
// When false, the node is externally managed and cannot be restarted.
func isNodeSelfManaged() bool {
	return sharedNode != nil
}

// stopSharedNode stops the test-managed node. Panics if the node is
// externally managed.
func stopSharedNode(t testing.TB) {
	if sharedNode == nil {
		t.Fatal("cannot stop node: not managed by tests (running in compose?)")
	}
	sharedNode.stop(t)
	sharedNode = nil
}

// startSharedNode starts a new test-managed node, reusing the existing log
// file. Call this after stopSharedNode to restart the node.
func startSharedNode(t testing.TB) {
	startSharedNodeWithEnv(t)
}

// startSharedNodeWithEnv is like startSharedNode but also lets the caller
// inject extra environment variables (e.g.,
// CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false to bring the node up in
// reader mode for a single test phase). Restore default mode on test
// teardown by stopping the node and calling startSharedNode again.
func startSharedNodeWithEnv(t testing.TB, extraEnv ...string) {
	if sharedNode != nil {
		t.Fatal("cannot start node: already running")
	}

	logPath := os.Getenv("CARTESI_TEST_NODE_LOG_FILE")
	var err error
	sharedNode, err = startNodeWithLog(logPath, extraEnv...)
	if err != nil {
		t.Fatalf("failed to start node: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := sharedNode.waitForHealth(ctx, t); err != nil {
		sharedNode.stop(t)
		sharedNode = nil
		t.Fatalf("node failed to become healthy: %v", err)
	}
}

// nodePortAvailable returns true if the node's telemetry port (10000) is free.
func nodePortAvailable() bool {
	conn, err := net.DialTimeout("tcp", "localhost:10000", time.Second)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

// nodeProcess represents a running node subprocess managed by the test.
type nodeProcess struct {
	cmd     *exec.Cmd
	logFile *os.File
	tail    *exec.Cmd // tail -f process for live log streaming
	tty     *os.File  // /dev/tty FD used by tail; closed in stop()
}

// startNodeWithLog starts the node binary as a subprocess, appending output
// to the given log file path. The node inherits the current environment
// (database connection, blockchain endpoint, etc.) and additionally sets
// fast polling intervals for test responsiveness. Any extraEnv entries are
// appended last, so they win against the suite defaults (useful for, e.g.,
// CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false reader-mode tests).
//
// A background `tail -f` process streams the log file to the terminal so
// the user can see node output in real time. This must be a separate process
// because `go test` captures the test process's stdout/stderr.
func startNodeWithLog(logPath string, extraEnv ...string) (*nodeProcess, error) {
	if _, err := exec.LookPath(nodeBinary); err != nil {
		return nil, fmt.Errorf("%s not found on PATH: %w", nodeBinary, err)
	}

	logFile, err := os.OpenFile( //nolint:gosec
		logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	cmd := exec.Command(nodeBinary) //nolint:gosec
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if workDir := os.Getenv("CARTESI_TEST_NODE_WORKDIR"); workDir != "" {
		if err := os.MkdirAll(workDir, 0755); err != nil { //nolint:mnd
			logFile.Close()
			return nil, fmt.Errorf("create node workdir %s: %w", workDir, err)
		}
		cmd.Dir = workDir
	}
	cmd.Env = append(os.Environ(),
		"CARTESI_ADVANCER_POLLING_INTERVAL=1",
		"CARTESI_VALIDATOR_POLLING_INTERVAL=1",
		"CARTESI_CLAIMER_POLLING_INTERVAL=1",
		"CARTESI_PRT_POLLING_INTERVAL=1",
	)
	cmd.Env = append(cmd.Env, extraEnv...)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start node: %w", err)
	}

	// Stream the log file to the terminal via a separate tail process.
	// We write to /dev/tty to bypass go test and gotestsum's output capture,
	// so the user sees node logs in real time just like the old Makefile did.
	// Falls back silently if /dev/tty is not available (e.g., CI, compose).
	tty, ttyErr := os.OpenFile("/dev/tty", os.O_WRONLY, 0) //nolint:gosec
	if ttyErr != nil {
		return &nodeProcess{cmd: cmd, logFile: logFile}, nil
	}

	tail := exec.Command("tail", "-f", logPath) //nolint:gosec
	tail.Stdout = tty
	tail.Stderr = tty
	if err := tail.Start(); err != nil {
		tty.Close()
		return &nodeProcess{cmd: cmd, logFile: logFile}, nil
	}

	return &nodeProcess{cmd: cmd, logFile: logFile, tail: tail, tty: tty}, nil
}

// waitForHealth polls the node's readyz endpoint until it responds 200 OK
// or the context is cancelled.
func (n *nodeProcess) waitForHealth(ctx context.Context, t testing.TB) error {
	client := &http.Client{Timeout: 2 * time.Second}
	return pollUntil(ctx, 2*time.Second, func() (bool, error) {
		req, err := http.NewRequestWithContext(
			ctx, "GET", "http://localhost:10000/readyz", nil)
		if err != nil {
			return false, nil
		}
		resp, err := client.Do(req)
		if err != nil {
			if t != nil {
				t.Log("    waiting for node health...")
			}
			return false, nil
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK, nil
	})
}

// stop sends an interrupt signal to the node process and waits for it to exit.
// On Unix this is SIGINT (same as Ctrl+C), triggering the node's graceful
// shutdown. Also kills the tail process if running.
func (n *nodeProcess) stop(t testing.TB) {
	if n.cmd.Process == nil {
		return
	}

	pid := n.cmd.Process.Pid
	if t != nil {
		t.Logf("    stopping node (pid=%d)...", pid)
	}

	// Stop the tail process first so it doesn't print shutdown logs.
	if n.tail != nil && n.tail.Process != nil {
		_ = n.tail.Process.Kill()
		_ = n.tail.Wait()
	}
	if n.tty != nil {
		n.tty.Close()
	}

	// Send interrupt for graceful shutdown.
	if err := n.cmd.Process.Signal(os.Interrupt); err != nil {
		if t != nil {
			t.Logf("    signal failed, killing: %v", err)
		}
		_ = n.cmd.Process.Kill()
	}

	// Wait for exit with a timeout — if the node hangs during shutdown,
	// fall back to SIGKILL so the test suite doesn't hang indefinitely.
	done := make(chan error, 1)
	go func() { done <- n.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		if t != nil {
			t.Log("    node did not exit within 30s, sending SIGKILL")
		}
		_ = n.cmd.Process.Kill()
		<-done
	}
	n.logFile.Close()
	if t != nil {
		t.Logf("    node stopped (pid=%d)", pid)
	}
}
