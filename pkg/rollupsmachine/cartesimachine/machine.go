// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cartesimachine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

const (
	AdvanceStateRequest RequestType = 0
	InspectStateRequest RequestType = 1
)

var (
	ErrCartesiMachine = errors.New("cartesi machine internal error")

	ErrTainted  = errors.New("cartesi machine tainted")
	ErrCanceled = errors.New("cartesi machine operation canceled")

	ErrOrphanServer = errors.New("cartesi machine server was left orphan")
)

type Timeouts struct {
	AdvanceRun time.Duration // For running the machine for an advance-state request.
	InspectRun time.Duration // For running the machine for an inspect-state request.
	Create     time.Duration // For instantiating a new machine (be it with a Load or a Fork).
	Store      time.Duration // For storing a machine.
	Fast       time.Duration // For fast machine operations.
}

type machine struct {
	inner  *emulator.Machine
	server *emulator.RemoteMachineManager

	tainted bool

	timeouts Timeouts
	address  string // Address of the JSON RPC remote cartesi machine server.
}

// ------------------------------------------------------------------------------------------------

// Load loads the machine stored at path into the remote server from address.
func Load(ctx context.Context,
	path string,
	address string,
	timeouts Timeouts,
	config *emulator.MachineRuntimeConfig,
) (CartesiMachine, error) {
	m := &machine{address: address, timeouts: timeouts}

	// Creates the manager for the server.
	server, err := newRemoteMachineManager(ctx, timeouts, address)
	if err != nil {
		err = fmt.Errorf("failed to create the remote machine manager: %w", err)
		return nil, err
	}
	m.server = server

	// Loads the machine stored at path into the server.
	inner, err := m.load(ctx, path, config)
	if err != nil {
		if m.server != nil {
			m.server.Delete()
		}
		return nil, fmt.Errorf("failed to load the machine: %w", err)
	}
	m.inner = inner

	return m, nil
}

// ------------------------------------------------------------------------------------------------

// Fork forks the machine.
func (m *machine) Fork(ctx context.Context) (CartesiMachine, error) {
	if m.tainted {
		return nil, ErrTainted
	}

	newMachine := &machine{timeouts: m.timeouts}

	// Forks the server.
	address, err := m.fork(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fork the machine: %w", err)
	}
	newMachine.address = address

	// Instantiates the new remote machine manager.
	server, err := newRemoteMachineManager(ctx, m.timeouts, newMachine.address)
	if err != nil {
		err = fmt.Errorf("failed to create the new remote machine manager: %w", err)
		return nil, errors.Join(err, errOrphanServerWithAddress(newMachine.address))
	}
	newMachine.server = server

	// Gets the inner machine reference from the server.
	inner, err := newMachine.getMachine(ctx)
	if err != nil {
		if !errors.Is(err, ErrTainted) {
			err = errors.Join(err, newMachine.shutdown(ctx)) // NOTE
		}
		return nil, fmt.Errorf("failed to get the machine from the server: %w", err)
	}
	newMachine.inner = inner

	return newMachine, nil
}

func (m *machine) Continue(ctx context.Context) error {
	if m.tainted {
		return ErrTainted
	}

	err := m.resetIFlagsY(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset iflagsY: %w", err)
	}

	return nil
}

func (m *machine) AdvanceRun(ctx context.Context, until uint64) (emulator.BreakReason, error) {
	if m.tainted {
		return -1, ErrTainted
	}

	breakReason, err := m.run(ctx, until, m.timeouts.AdvanceRun)
	if err != nil {
		return -1, fmt.Errorf("failed to run (advance): %w", err)
	}

	return breakReason, nil
}

func (m *machine) InspectRun(ctx context.Context, until uint64) (emulator.BreakReason, error) {
	if m.tainted {
		return -1, ErrTainted
	}

	breakReason, err := m.run(ctx, until, m.timeouts.InspectRun)
	if err != nil {
		return -1, fmt.Errorf("failed to run (inspect): %w", err)
	}

	return breakReason, nil
}

// Close closes the cartesi machine. It also shuts down the remote cartesi machine server.
func (m *machine) Close(ctx context.Context) error {
	if m.tainted {
		return ErrTainted
	}

	m.inner.Delete()
	m.inner = nil

	err := m.shutdown(ctx)
	if err != nil {
		err = fmt.Errorf("could not shut down the server: %w", err)
		err = errors.Join(err, errOrphanServerWithAddress(m.address))
	}
	m.server.Delete()
	m.server = nil

	return err
}

// ------------------------------------------------------------------------------------------------

func (m *machine) IsAtManualYield(ctx context.Context) (bool, error) {
	if m.tainted {
		return false, ErrTainted
	}

	iflagsY, err := m.readIFlagsY(ctx)
	if err != nil {
		return iflagsY, fmt.Errorf("failed to read the yield type: %w", err)
	}

	return iflagsY, nil
}

func (m *machine) ReadYieldReason(ctx context.Context) (emulator.HtifYieldReason, error) {
	if m.tainted {
		return 0, ErrTainted
	}

	tohost, err := m.readHtifToHostData(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to read the yield reason from HTIF tohost: %w", err)
	}

	yieldReason := tohost >> 32 //nolint:mnd
	return emulator.HtifYieldReason(yieldReason), nil
}

func (m *machine) ReadHash(ctx context.Context) ([32]byte, error) {
	if m.tainted {
		return [32]byte{}, ErrTainted
	}

	hash, err := m.getRootHash(ctx)
	if err != nil {
		return [32]byte{}, fmt.Errorf("failed to get the machine's root hash: %w", err)
	}

	return hash, nil
}

func (m *machine) ReadCycle(ctx context.Context) (uint64, error) {
	if m.tainted {
		return 0, ErrTainted
	}

	cycle, err := m.readMCycle(ctx)
	if err != nil {
		return 0, fmt.Errorf("could not read the machine's current cycle: %w", err)
	}

	return cycle, nil
}

func (m *machine) ReadMemory(ctx context.Context) ([]byte, error) {
	if m.tainted {
		return []byte{}, ErrTainted
	}

	tohost, err := m.readHtifToHostData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the length from HTIF tohost: %w", err)
	}
	length := tohost & 0x00000000ffffffff //nolint:mnd

	data, err := m.readMemory(ctx, length)
	if err != nil {
		return nil, fmt.Errorf("failed to read the memory: %w", err)
	}

	return data, nil
}

func (m *machine) WriteRequest(ctx context.Context, data []byte, type_ RequestType) error {
	if m.tainted {
		return ErrTainted
	}

	// Writes the request's data.
	err := m.writeMemory(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to write to the memory: %w", err)
	}

	// Writes the request's type and length.
	fromhost := ((uint64(type_) << 32) | (uint64(len(data)) & 0xffffffff)) //nolint:mnd
	err = m.writeHtifFromHostData(ctx, fromhost)
	if err != nil {
		return fmt.Errorf("failed to write the length to HTIF fromhost: %w", err)
	}

	return nil
}

// ------------------------------------------------------------------------------------------------

func (m machine) PayloadLengthLimit() uint {
	expo := float64(emulator.CmioRxBufferLog2Size)
	payloadLengthLimit := uint(math.Pow(2, expo)) //nolint:mnd
	return payloadLengthLimit
}

func (m machine) Address() string {
	return m.address
}

// ------------------------------------------------------------------------------------------------

func errOrphanServerWithAddress(address string) error {
	return fmt.Errorf("%w at address %s", ErrOrphanServer, address)
}
