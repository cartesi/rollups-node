// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
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

func startNodeSubprocess(name string, cmd *exec.Cmd) (*nodeSubprocess, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &nodeSubprocess{name: name, cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.waitErr = err
		p.mu.Unlock()
		close(p.done)
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
