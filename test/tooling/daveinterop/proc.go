// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const (
	childStopTimeout  = 20 * time.Second
	portPollInterval  = 200 * time.Millisecond
	groupPollInterval = 100 * time.Millisecond
)

// stopSignals are the signals that end a run cleanly: its processes are
// stopped and its report is written.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

// child is a background process owned by this command. It runs in its own
// process group so that stop also reaches the processes it spawned (for
// example, the rollups node's machine servers). Nothing is ever killed by name.
type child struct {
	name    string
	logPath string
	cmd     *exec.Cmd
	logFile *os.File

	done     chan struct{}
	mu       sync.Mutex
	waitErr  error
	exitCode int
	stopOnce sync.Once
}

// startChild starts bin with args, writing stdout and stderr to logPath.
// env is the complete environment of the child.
func startChild(name, logPath, dir string, env []string, bin string, args ...string) (*child, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644) //nolint:mnd
	if err != nil {
		return nil, fmt.Errorf("opening %s log: %w", name, err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("starting %s (%s): %w", name, bin, err)
	}
	c := &child{name: name, logPath: logPath, cmd: cmd, logFile: logFile, done: make(chan struct{}), exitCode: -1}
	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		c.waitErr = err
		if cmd.ProcessState != nil {
			c.exitCode = cmd.ProcessState.ExitCode()
		}
		c.mu.Unlock()
		logFile.Close()
		close(c.done)
	}()
	logger.Debug("started", "process", name, "pid", cmd.Process.Pid, "log", logPath)
	return c, nil
}

// exited reports whether the child has exited, with its status.
func (c *child) exited() (bool, int, error) {
	select {
	case <-c.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		return true, c.exitCode, c.waitErr
	default:
		return false, 0, nil
	}
}

// errIfExited returns an error when the child exited, even with status zero:
// a long-running child that exits early has not done its work.
func (c *child) errIfExited() error {
	if ok, code, err := c.exited(); ok {
		return fmt.Errorf("%s exited early (code %d, %v); see %s", c.name, code, err, displayPath(c.logPath))
	}
	return nil
}

// stop ends the child's whole process group, even when the child itself has
// already exited: its own children (for example, the machine servers of the
// rollups node) may still run. It is safe to call more than once.
func (c *child) stop() {
	c.stopOnce.Do(func() {
		stopGroup(c.name, c.cmd.Process.Pid)
		<-c.done
		logger.Debug("stopped", "process", c.name)
	})
}

// stopGroup sends SIGTERM to a process group, waits until the group is empty,
// and sends SIGKILL to what is left after childStopTimeout.
func stopGroup(name string, pgid int) {
	if !groupAlive(pgid) {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(childStopTimeout)
	for groupAlive(pgid) {
		if time.Now().After(deadline) {
			logger.Warn("processes did not stop; killing them", "process", name)
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			return
		}
		time.Sleep(groupPollInterval)
	}
}

// groupAlive reports whether a process group still has a member that this
// user can signal.
func groupAlive(pgid int) bool {
	return syscall.Kill(-pgid, 0) == nil
}

// waitForPort waits until addr accepts TCP connections. It fails early when
// the owning child exits.
func waitForPort(ctx context.Context, addr string, owner *child, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		if owner != nil {
			if exitErr := owner.errIfExited(); exitErr != nil {
				return exitErr
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not open within %s", addr, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(portPollInterval):
		}
	}
}

// requirePortFree refuses to start a service on a port that someone else owns.
func requirePortFree(port int) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		conn.Close()
		return fmt.Errorf("port %d is already in use; a run left running? (lsof -nP -iTCP:%d -sTCP:LISTEN), "+
			"or choose other ports (--anvil-port, --node-port-base)", port, port)
	}
	return nil
}

// freePort asks the kernel for an unused local port.
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// runForeground runs bin with inherited standard streams, forwards the stop
// signals to it, and returns its exit code.
func runForeground(dir string, env []string, bin string, args ...string) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr // our stdout is reserved for the JSON result
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("starting %s: %w", bin, err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, stopSignals...)
	defer signal.Stop(signals)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case sig := <-signals:
			_ = cmd.Process.Signal(sig)
		case err := <-done:
			var exitErr *exec.ExitError
			if err != nil && !errors.As(err, &exitErr) {
				return -1, err
			}
			return cmd.ProcessState.ExitCode(), nil
		}
	}
}
