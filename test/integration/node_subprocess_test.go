// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// nodeSubprocess waits exactly once and makes process exit visible to readiness
// and shutdown checks in both test topologies.
type nodeSubprocess struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}

	mu      sync.Mutex
	waitErr error
}

var managedNodeProcesses = struct {
	sync.Mutex
	processes map[*nodeSubprocess]struct{}
	stopping  bool
}{processes: map[*nodeSubprocess]struct{}{}}

func startNodeSubprocess(name string, cmd *exec.Cmd) (*nodeSubprocess, error) {
	managedNodeProcesses.Lock()
	if managedNodeProcesses.stopping {
		managedNodeProcesses.Unlock()
		return nil, fmt.Errorf("cannot start %s: integration deadline cleanup has begun", name)
	}
	if err := cmd.Start(); err != nil {
		managedNodeProcesses.Unlock()
		return nil, err
	}
	p := &nodeSubprocess{name: name, cmd: cmd, done: make(chan struct{})}
	managedNodeProcesses.processes[p] = struct{}{}
	managedNodeProcesses.Unlock()
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.waitErr = err
		p.mu.Unlock()
		close(p.done)
		managedNodeProcesses.Lock()
		delete(managedNodeProcesses.processes, p)
		managedNodeProcesses.Unlock()
	}()
	return p, nil
}

func (p *nodeSubprocess) exitedProcessError() error {
	select {
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.waitErr != nil {
			return fmt.Errorf("%s exited unexpectedly: %w", p.name, p.waitErr)
		}
		return fmt.Errorf("%s exited unexpectedly", p.name)
	default:
		return nil
	}
}

func (p *nodeSubprocess) isDone() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *nodeSubprocess) wait(timeout time.Duration) bool {
	select {
	case <-p.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestNodeSubprocess(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run(fmt.Sprintf("exit_%d", exitCode), func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "sh", "-c", "read value; exit \"$value\"")
			stdin, err := cmd.StdinPipe()
			require.NoError(t, err)
			p, err := startNodeSubprocess("test-node", cmd)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = stdin.Close()
				if !p.isDone() {
					_ = cmd.Process.Kill()
				}
				require.True(t, p.wait(time.Second), "child must be reaped")
			})
			require.NoError(t, p.exitedProcessError())
			require.False(t, p.wait(time.Millisecond))
			_, err = io.WriteString(stdin, fmt.Sprintf("%d\n", exitCode))
			require.NoError(t, err)
			require.NoError(t, stdin.Close())
			require.True(t, p.wait(time.Second))
			require.True(t, p.isDone())
			err = p.exitedProcessError()
			require.ErrorContains(t, err, "test-node exited unexpectedly")
			if exitCode != 0 {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				require.Equal(t, exitCode, exitErr.ExitCode())
			}
			require.True(t, p.wait(time.Second), "repeated checks must not consume exit status")
		})
	}
}

// Stop before testing's timeout panic, which cannot run TestMain cleanup. The
// registry includes both topologies and log tails. It avoids reading sharedNode
// from the timer goroutine while a test is restarting that node.
func installManagedNodeDeadline(timeout time.Duration) func() {
	if timeout <= 0 {
		return func() {}
	}
	grace := min(15*time.Second, timeout/4)
	cutoff := timeout - 3*grace
	timer := time.AfterFunc(cutoff, func() {
		fmt.Fprintln(os.Stderr, "integration deadline reached; stopping managed child processes before test timeout")
		if profile := pprof.Lookup("goroutine"); profile != nil {
			if err := profile.WriteTo(os.Stderr, 2); err != nil {
				fmt.Fprintf(os.Stderr, "write timeout goroutines: %v\n", err)
			}
		}
		if err := stopManagedNodeProcesses(grace); err != nil {
			fmt.Fprintf(os.Stderr, "stop integration child processes: %v\n", err)
		}
		os.Exit(1)
	})
	return func() { timer.Stop() }
}

func stopManagedNodeProcesses(grace time.Duration) error {
	managedNodeProcesses.Lock()
	managedNodeProcesses.stopping = true
	processes := make([]*nodeSubprocess, 0, len(managedNodeProcesses.processes))
	for process := range managedNodeProcesses.processes {
		processes = append(processes, process)
	}
	managedNodeProcesses.Unlock()
	var errs []error
	for _, process := range processes {
		if !process.isDone() {
			if err := process.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
				errs = append(errs, fmt.Errorf("interrupt %s: %w", process.name, err))
			}
		}
	}
	deadline := time.Now().Add(grace)
	for _, process := range processes {
		if !process.wait(max(0, time.Until(deadline))) {
			if err := process.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				errs = append(errs, fmt.Errorf("kill %s: %w", process.name, err))
			}
		}
	}
	reapDeadline := time.Now().Add(grace)
	for _, process := range processes {
		if !process.wait(max(0, time.Until(reapDeadline))) {
			errs = append(errs, fmt.Errorf("%s did not exit after kill", process.name))
		}
	}
	return errors.Join(errs...)
}

const (
	managedDeadlineChildEnv    = "CARTESI_TEST_MANAGED_DEADLINE_PID_FILE"
	managedDeadlineTestPattern = "^TestManagedNodeDeadlineStopsChildrenBeforeProcessExit$"
)

func TestManagedNodeDeadlineStopsChildrenBeforeProcessExit(t *testing.T) {
	if path := os.Getenv(managedDeadlineChildEnv); path != "" {
		//nolint:gosec // G702: Fixed script; the parent passes its t.TempDir PID path as a quoted positional argument.
		cmd := exec.Command("sh", "-c", `trap 'exit 0' INT TERM; echo $$ >"$1"; while read -r value; do :; done`, "child", path)
		stdin, err := cmd.StdinPipe()
		require.NoError(t, err)
		defer func() { require.NoError(t, stdin.Close()) }()
		_, err = startNodeSubprocess("deadline-child", cmd)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			//nolint:gosec // G703: This child reads only the fresh PID file created under the parent test's t.TempDir.
			_, err := os.Stat(path)
			return err == nil
		}, 5*time.Second, 10*time.Millisecond)
		installManagedNodeDeadline(800 * time.Millisecond)
		select {} // TestMain's normal cleanup cannot help this child process.
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run="+managedDeadlineTestPattern, "-test.v")
	cmd.Env = append(os.Environ(), managedDeadlineChildEnv+"="+path)
	output, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "%s", output)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "%s", output)
	require.Equal(t, 1, exitErr.ExitCode())
	require.Contains(t, string(output), "integration deadline reached; stopping managed child processes")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH, "the managed child must not survive the test runner")
}
