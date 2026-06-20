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
	"sync"
	"testing"
	"time"
)

// multiService describes one service subprocess and the telemetry port
// whose /readyz endpoint reports its health. Each service keeps the
// read-API ports the tests already expect (jsonrpc :10011, inspect :10012), so
// only the telemetry ports are remapped to avoid collisions on localhost.
type multiService struct {
	name          string
	telemetryAddr string
	extraEnv      []string
}

// multiNodeServices is the full deployment the multiprocess topology runs on
// the host — every service the standalone node embeds. prt is included so the
// prt shard's Dave-consensus tests run; it idles for authority/quorum apps.
var multiNodeServices = []multiService{
	{name: "cartesi-rollups-evm-reader", telemetryAddr: ":10001",
		extraEnv: []string{"CARTESI_EVM_READER_TELEMETRY_ADDRESS=:10001"}},
	{name: "cartesi-rollups-advancer", telemetryAddr: ":10002",
		extraEnv: []string{
			"CARTESI_ADVANCER_TELEMETRY_ADDRESS=:10002",
			"CARTESI_INSPECT_ADDRESS=:10012",
			"CARTESI_FEATURE_INSPECT_ENABLED=true",
		}},
	{name: "cartesi-rollups-validator", telemetryAddr: ":10003",
		extraEnv: []string{"CARTESI_VALIDATOR_TELEMETRY_ADDRESS=:10003"}},
	{name: "cartesi-rollups-claimer", telemetryAddr: ":10004",
		extraEnv: []string{"CARTESI_CLAIMER_TELEMETRY_ADDRESS=:10004"}},
	{name: "cartesi-rollups-jsonrpc-api", telemetryAddr: ":10005",
		extraEnv: []string{
			"CARTESI_JSONRPC_TELEMETRY_ADDRESS=:10005",
			"CARTESI_JSONRPC_API_ADDRESS=:10011",
		}},
	{name: "cartesi-rollups-prt", telemetryAddr: ":10006",
		extraEnv: []string{"CARTESI_PRT_TELEMETRY_ADDRESS=:10006"}},
}

var multiNodeListenAddrs = []string{
	":10000", // stale standalone node telemetry
	":10001", ":10002", ":10003", ":10004", ":10005", ":10006",
	":10011", // jsonrpc API
	":10012", // inspect API
}

type multiNodeProcess struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}

	mu      sync.Mutex
	waitErr error
}

// multiNode is a running host multiprocess deployment.
type multiNode struct {
	procs   []*multiNodeProcess
	addrs   []string
	logFile *os.File
	tail    *exec.Cmd // tail -f process streaming the log to the terminal
	tty     *os.File  // /dev/tty FD used by tail; closed in stop()
}

// startMultiNode starts each service binary as a host subprocess sharing the
// inherited environment (DB connection, blockchain endpoint, contracts,
// mnemonic, snapshot dir) plus per-service telemetry/address overrides and the
// fast polling intervals used for test responsiveness. All output is appended
// to logPath and streamed to the terminal, the same way the standalone node is.
// On any failure the already-started processes are stopped.
func startMultiNode(logPath string, extraEnv ...string) (*multiNode, error) {
	if err := preflightMultiNodePorts(); err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile( //nolint:gosec
		logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644) //nolint:mnd
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	mn := &multiNode{logFile: logFile}

	// Stream the combined service log to the terminal via a separate tail
	// process writing to /dev/tty, bypassing go test / gotestsum output
	// capture (same approach as the standalone node). Started before the
	// services so their startup output is visible. Falls back silently when
	// /dev/tty is unavailable (CI, compose).
	if tty, ttyErr := os.OpenFile("/dev/tty", os.O_WRONLY, 0); ttyErr == nil {
		tail := exec.Command("tail", "-f", logPath) //nolint:gosec
		tail.Stdout = tty
		tail.Stderr = tty
		if err := tail.Start(); err != nil {
			tty.Close()
		} else {
			mn.tail = tail
			mn.tty = tty
		}
	}

	base := append(os.Environ(),
		"CARTESI_ADVANCER_POLLING_INTERVAL=1",
		"CARTESI_VALIDATOR_POLLING_INTERVAL=1",
		"CARTESI_CLAIMER_POLLING_INTERVAL=1",
		"CARTESI_EVM_READER_POLLING_INTERVAL=1",
		"CARTESI_PRT_POLLING_INTERVAL=1",
	)

	for _, svc := range multiNodeServices {
		if _, err := exec.LookPath(svc.name); err != nil {
			mn.stop(nil)
			return nil, fmt.Errorf("%s not found on PATH: %w", svc.name, err)
		}
		cmd := exec.Command(svc.name) //nolint:gosec
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.Env = append(append(append([]string{}, base...), svc.extraEnv...), extraEnv...)
		if workDir := os.Getenv("CARTESI_TEST_NODE_WORKDIR"); workDir != "" {
			if err := os.MkdirAll(workDir, 0755); err != nil { //nolint:mnd
				mn.stop(nil)
				return nil, fmt.Errorf("create node workdir %s: %w", workDir, err)
			}
			cmd.Dir = workDir
		}
		if err := cmd.Start(); err != nil {
			mn.stop(nil)
			return nil, fmt.Errorf("start %s: %w", svc.name, err)
		}
		proc := &multiNodeProcess{name: svc.name, cmd: cmd, done: make(chan struct{})}
		go func() {
			err := cmd.Wait()
			proc.mu.Lock()
			proc.waitErr = err
			proc.mu.Unlock()
			close(proc.done)
		}()
		fmt.Fprintf(os.Stderr, "  started %s (telemetry %s, pid %d)\n",
			svc.name, svc.telemetryAddr, cmd.Process.Pid)
		mn.procs = append(mn.procs, proc)
		mn.addrs = append(mn.addrs, svc.telemetryAddr)
	}
	return mn, nil
}

func preflightMultiNodePorts() error {
	for _, addr := range multiNodeListenAddrs {
		conn, err := net.DialTimeout("tcp", "localhost"+addr, time.Second)
		if err == nil {
			conn.Close()
			return fmt.Errorf("cannot start multiprocess node: localhost%s is already in use", addr)
		}
	}
	return nil
}

// waitForHealth polls every service's /readyz until all respond 200 OK or the
// context is cancelled.
func (mn *multiNode) waitForHealth(ctx context.Context, _ testing.TB) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for i, addr := range mn.addrs {
		svc := multiNodeServices[i].name
		url := "http://localhost" + addr + "/readyz"
		err := pollUntil(ctx, 2*time.Second, func() (bool, error) {
			if err := mn.exitedProcessError(); err != nil {
				return false, err
			}
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return false, nil
			}
			resp, err := client.Do(req) //nolint:gosec // url is a fixed localhost telemetry port
			if err != nil {
				return false, nil
			}
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusOK, nil
		})
		if err != nil {
			return fmt.Errorf("%s did not become healthy at %s: %w", svc, url, err)
		}
		fmt.Fprintf(os.Stderr, "  %s healthy\n", svc)
	}
	return nil
}

func (mn *multiNode) exitedProcessError() error {
	for _, proc := range mn.procs {
		if exited, err := proc.exitStatus(); exited {
			if err != nil {
				return fmt.Errorf("%s exited unexpectedly: %w", proc.name, err)
			}
			return fmt.Errorf("%s exited unexpectedly", proc.name)
		}
	}
	return nil
}

func (p *multiNodeProcess) exitStatus() (bool, error) {
	select {
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return true, p.waitErr
	default:
		return false, nil
	}
}

// stop interrupts every service subprocess (in reverse start order) and waits
// for it to exit, then stops the log tail and closes the log file.
func (mn *multiNode) stop(t testing.TB) {
	// Stop the tail first so it does not print shutdown noise.
	if mn.tail != nil && mn.tail.Process != nil {
		_ = mn.tail.Process.Kill()
		_ = mn.tail.Wait()
	}
	if mn.tty != nil {
		mn.tty.Close()
	}
	for i := len(mn.procs) - 1; i >= 0; i-- {
		proc := mn.procs[i]
		if proc.cmd.Process == nil || proc.isDone() {
			continue
		}
		if err := proc.cmd.Process.Signal(os.Interrupt); err != nil {
			if t != nil {
				t.Logf("    signal %s failed, killing: %v", proc.name, err)
			}
			_ = proc.cmd.Process.Kill()
		}
	}
	for _, proc := range mn.procs {
		if proc.wait(30 * time.Second) { //nolint:mnd
			continue
		}
		if t != nil {
			t.Logf("    %s did not exit within 30s, sending SIGKILL", proc.name)
		}
		if proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
		<-proc.done
	}
	if mn.logFile != nil {
		mn.logFile.Close()
	}
}

func (p *multiNodeProcess) isDone() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *multiNodeProcess) wait(timeout time.Duration) bool {
	select {
	case <-p.done:
		return true
	case <-time.After(timeout):
		return false
	}
}
