// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cartesimachine

import (
	"context"
	"errors"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

func newRemoteMachineManager(
	ctx context.Context,
	timeouts Timeouts,
	address string,
) (*emulator.RemoteMachineManager, error) {
	timeout := timeouts.Fast
	f := func() (*emulator.RemoteMachineManager, error) {
		return emulator.NewRemoteMachineManager(address)
	}
	server, err := timedCall(ctx, timeout, f)
	if err != nil && err != ErrTainted {
		err = errCartesiMachine(err)
	}
	return server, err
}

func (m *machine) load(ctx context.Context,
	path string,
	runtime *emulator.MachineRuntimeConfig,
) (*emulator.Machine, error) {
	timeout := m.timeouts.Create
	f := func() (*emulator.Machine, error) { return m.server.LoadMachine(path, runtime) }
	address, err := timedCall(ctx, timeout, f)
	if err != nil && err != ErrTainted {
		err = errCartesiMachine(err)
	}
	return address, err
}

func (m *machine) fork(ctx context.Context) (address string, _ error) {
	timeout := m.timeouts.Create
	f := m.server.Fork
	address, err := timedCall(ctx, timeout, f)
	if err != nil && err != ErrTainted {
		err = errCartesiMachine(err)
	}
	return address, err
}

// ------------------------------------------------------------------------------------------------

func (m *machine) shutdown(ctx context.Context) error {
	timeout := m.timeouts.Fast
	f := func() (bool, error) { return true, m.server.Shutdown() }
	_, err := apiCall(m, ctx, timeout, f)
	return err
}

func (m *machine) getMachine(ctx context.Context) (*emulator.Machine, error) {
	timeout := m.timeouts.Fast
	f := m.server.GetMachine
	return apiCall(m, ctx, timeout, f)
}

func (m *machine) readIFlagsY(ctx context.Context) (bool, error) {
	timeout := m.timeouts.Fast
	f := m.inner.ReadIFlagsY
	return apiCall(m, ctx, timeout, f)
}

func (m *machine) readHtifToHostData(ctx context.Context) (uint64, error) {
	timeout := m.timeouts.Fast
	f := m.inner.ReadHtifToHostData
	return apiCall(m, ctx, timeout, f)
}

func (m *machine) writeHtifFromHostData(ctx context.Context, fromhost uint64) error {
	timeout := m.timeouts.Fast
	f := func() (bool, error) { return true, m.inner.WriteHtifFromHostData(fromhost) }
	_, err := apiCall(m, ctx, timeout, f)
	return err
}

func (m *machine) getRootHash(ctx context.Context) (emulator.MerkleTreeHash, error) {
	timeout := m.timeouts.Fast
	f := m.inner.GetRootHash
	return apiCall(m, ctx, timeout, f)
}

func (m *machine) readMemory(ctx context.Context, length uint64) ([]byte, error) {
	timeout := m.timeouts.Fast
	f := func() ([]byte, error) { return m.inner.ReadMemory(emulator.CmioTxBufferStart, length) }
	return apiCall(m, ctx, timeout, f)
}

func (m *machine) writeMemory(ctx context.Context, data []byte) error {
	timeout := m.timeouts.Fast
	f := func() (bool, error) { return true, m.inner.WriteMemory(emulator.CmioRxBufferStart, data) }
	_, err := apiCall(m, ctx, timeout, f)
	return err
}

func (m *machine) resetIFlagsY(ctx context.Context) error {
	timeout := m.timeouts.Fast
	f := func() (bool, error) { return true, m.inner.ResetIFlagsY() }
	_, err := apiCall(m, ctx, timeout, f)
	return err
}

func (m *machine) run(ctx context.Context, until uint64, timeout time.Duration) (emulator.BreakReason, error) {
	f := func() (emulator.BreakReason, error) { return m.inner.Run(until) }
	breakReason, err := apiCall(m, ctx, timeout, f)
	assert(err == nil || breakReason == emulator.BreakReasonFailed, breakReason.String())
	return breakReason, err
}

func (m *machine) readMCycle(ctx context.Context) (uint64, error) {
	timeout := m.timeouts.Fast
	f := m.inner.ReadMCycle
	return apiCall(m, ctx, timeout, f)
}

// ------------------------------------------------------------------------------------------------

// apiCall wraps a timedCall to close the machine in case of an ErrTainted.
func apiCall[V any](
	m *machine,
	ctx context.Context,
	timeout time.Duration,
	f func() (V, error),
) (V, error) {
	v, err := timedCall(ctx, timeout, f)
	if err != nil {
		var v V
		if err == ErrTainted {
			return v, errors.Join(err, m.closeTainted())
		} else {
			return v, errCartesiMachine(err)
		}
	}

	return v, nil
}

// timedCall calls the given function in a separate thread and waits for its completion.
// It returns early if the context gets canceled or if the timeout elapses.
// (If returns an ErrTainted if the timeout elapses.)
func timedCall[V any](ctx context.Context, timeout time.Duration, f func() (V, error)) (V, error) {
	type result struct {
		v   V
		err error
	}

	ch := make(chan result, 1)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		v, err := f()
		ch <- result{v, err}
	}()

	select {
	case <-ctx.Done():
		var v V
		return v, ctx.Err()
	case <-timeoutCtx.Done():
		var v V
		return v, ErrTainted
	case res := <-ch:
		return res.v, res.err
	}
}

// ------------------------------------------------------------------------------------------------

func (m *machine) closeTainted() error {
	if m.inner != nil {
		m.inner.Delete()
		m.inner = nil
	}
	if m.server != nil {
		m.server.Delete()
		m.server = nil
	}
	m.tainted = true
	// NOTE: we have to kill the machine's process now, but we need its PID and PPID for that.
	// errClose = fmt.Errorf("failed to close the tainted machine: %w", errClose)
	return nil
}

func errCartesiMachine(err error) error {
	return errors.Join(ErrCartesiMachine, err)
}

func assert(condition bool, s string) {
	if !condition {
		panic("assertion error: " + s)
	}
}
