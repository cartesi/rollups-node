// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cartesimachine

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

const (
	AdvanceStateRequest RequestType = 0
	InspectStateRequest RequestType = 1
)

var (
	ErrCartesiMachine = errors.New("cartesi machine internal error")

	ErrTimedOut = errors.New("cartesi machine operation timed out")
	ErrCanceled = errors.New("cartesi machine operation canceled")

	ErrOrphanServer = errors.New("cartesi machine server was left orphan")
)

type cartesiMachine struct {
	inner  *emulator.Machine
	server *emulator.RemoteMachineManager

	address string // address of the JSON RPC remote cartesi machine server
}

// Load loads the machine stored at path into the remote server from address.
func Load(ctx context.Context,
	path string,
	address string,
	config *emulator.MachineRuntimeConfig,
) (CartesiMachine, error) {
	machine := &cartesiMachine{address: address}

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	// Creates the server machine manager (the server's manager).
	server, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		err = fmt.Errorf("could not create the remote machine manager: %w", err)
		return nil, errCartesiMachine(err)
	}
	machine.server = server

	if err := checkContext(ctx); err != nil {
		machine.server.Delete()
		return nil, err
	}

	// Loads the machine stored at path into the server.
	inner, err := server.LoadMachine(path, config)
	if err != nil {
		defer machine.server.Delete()
		err = fmt.Errorf("could not load the machine: %w", err)
		return nil, errCartesiMachine(err)
	}
	machine.inner = inner

	return machine, nil
}

// Fork forks the machine.
//
// When Fork returns with the ErrOrphanServer error, it also returns with a non-nil CartesiMachine
// that can be used to retrieve the orphan server's address.
func (machine *cartesiMachine) Fork(ctx context.Context) (CartesiMachine, error) {
	newMachine := new(cartesiMachine)

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	// Forks the server.
	address, err := machine.server.Fork()
	if err != nil {
		err = fmt.Errorf("could not fork the machine: %w", err)
		return nil, errCartesiMachine(err)
	}
	newMachine.address = address

	// Instantiates the new remote machine manager.
	server, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		err = fmt.Errorf("could not create the new remote machine manager: %w", err)
		errOrphanServer := errOrphanServerWithAddress(address)
		return newMachine, errors.Join(ErrCartesiMachine, err, errOrphanServer)
	}
	newMachine.server = server

	// Gets the inner machine reference from the server.
	inner, err := newMachine.server.GetMachine()
	if err != nil {
		err = fmt.Errorf("could not get the machine from the server: %w", err)
		return newMachine, errors.Join(ErrCartesiMachine, err, newMachine.closeServer())
	}
	newMachine.inner = inner

	return newMachine, nil
}

func (machine *cartesiMachine) Run(ctx context.Context,
	until uint64,
) (emulator.BreakReason, error) {
	if err := checkContext(ctx); err != nil {
		return -1, err
	}

	breakReason, err := machine.inner.Run(until)
	if err != nil {
		assert(breakReason == emulator.BreakReasonFailed, breakReason.String())
		err = fmt.Errorf("machine run failed: %w", err)
		return breakReason, errCartesiMachine(err)
	}
	return breakReason, nil
}

func (machine *cartesiMachine) IsAtManualYield(ctx context.Context) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	iflagsY, err := machine.inner.ReadIFlagsY()
	if err != nil {
		err = fmt.Errorf("could not read iflagsY: %w", err)
		return iflagsY, errCartesiMachine(err)
	}
	return iflagsY, nil
}

func (machine *cartesiMachine) ReadYieldReason(
	ctx context.Context,
) (emulator.HtifYieldReason, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}

	tohost, err := machine.readHtifToHostData()
	if err != nil {
		return 0, err
	}
	yieldReason := tohost >> 32 //nolint:mnd
	return emulator.HtifYieldReason(yieldReason), nil
}

func (machine *cartesiMachine) ReadHash(ctx context.Context) ([32]byte, error) {
	if err := checkContext(ctx); err != nil {
		return [32]byte{}, err
	}

	hash, err := machine.inner.GetRootHash()
	if err != nil {
		err := fmt.Errorf("could not get the machine's root hash: %w", err)
		return hash, errCartesiMachine(err)
	}
	return hash, nil
}

func (machine *cartesiMachine) ReadMemory(ctx context.Context) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return []byte{}, err
	}

	tohost, err := machine.readHtifToHostData()
	if err != nil {
		return nil, err
	}
	length := tohost & 0x00000000ffffffff //nolint:mnd

	if err := checkContext(ctx); err != nil {
		return []byte{}, err
	}

	read, err := machine.inner.ReadMemory(emulator.CmioTxBufferStart, length)
	if err != nil {
		err := fmt.Errorf("could not read from the memory: %w", err)
		return nil, errCartesiMachine(err)
	}

	return read, nil
}

func (machine *cartesiMachine) WriteRequest(ctx context.Context,
	data []byte,
	type_ RequestType,
) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	// Writes the request's data.
	err := machine.inner.WriteMemory(emulator.CmioRxBufferStart, data)
	if err != nil {
		err := fmt.Errorf("could not write to the memory: %w", err)
		return errCartesiMachine(err)
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	// Writes the request's type and length.
	fromhost := ((uint64(type_) << 32) | (uint64(len(data)) & 0xffffffff)) //nolint:mnd
	err = machine.inner.WriteHtifFromHostData(fromhost)
	if err != nil {
		err := fmt.Errorf("could not write HTIF fromhost data: %w", err)
		return errCartesiMachine(err)
	}

	return nil
}

func (machine *cartesiMachine) Continue(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	err := machine.inner.ResetIFlagsY()
	if err != nil {
		err = fmt.Errorf("could not reset iflagsY: %w", err)
		return errCartesiMachine(err)
	}
	return nil
}

func (machine *cartesiMachine) ReadCycle(ctx context.Context) (uint64, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}

	cycle, err := machine.inner.ReadMCycle()
	if err != nil {
		err = fmt.Errorf("could not read the machine's current cycle: %w", err)
		return cycle, errCartesiMachine(err)
	}
	return cycle, nil
}

func (machine cartesiMachine) PayloadLengthLimit() uint {
	expo := float64(emulator.CmioRxBufferLog2Size)
	var payloadLengthLimit = uint(math.Pow(2, expo)) //nolint:mnd
	return payloadLengthLimit
}

func (machine cartesiMachine) Address() string {
	return machine.address
}

// Close closes the cartesi machine. It also shuts down the remote cartesi machine server.
func (machine *cartesiMachine) Close(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	machine.inner.Delete()
	machine.inner = nil
	return machine.closeServer()
}

// ------------------------------------------------------------------------------------------------

// closeServer shuts down the server and deletes its reference.
func (machine *cartesiMachine) closeServer() error {
	err := machine.server.Shutdown()
	if err != nil {
		err = fmt.Errorf("could not shut down the server: %w", err)
		err = errors.Join(errCartesiMachine(err), errOrphanServerWithAddress(machine.address))
	}
	machine.server.Delete()
	machine.server = nil
	return err
}

func (machine *cartesiMachine) readHtifToHostData() (uint64, error) {
	tohost, err := machine.inner.ReadHtifToHostData()
	if err != nil {
		err = fmt.Errorf("could not read HTIF tohost data: %w", err)
		return tohost, errCartesiMachine(err)
	}
	return tohost, nil
}

// ------------------------------------------------------------------------------------------------

func errCartesiMachine(err error) error {
	return errors.Join(ErrCartesiMachine, err)
}

func errOrphanServerWithAddress(address string) error {
	return fmt.Errorf("%w at address %s", ErrOrphanServer, address)
}

func checkContext(ctx context.Context) error {
	err := ctx.Err()
	if err == context.DeadlineExceeded {
		return ErrTimedOut
	} else if err == context.Canceled {
		return ErrCanceled
	} else {
		return err
	}
}

func assert(condition bool, s string) {
	if !condition {
		panic("assertion error: " + s)
	}
}
