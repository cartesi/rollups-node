// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/pkg/emulator"
)

type Cycle uint64

type Response []byte

const (
	DefaultIncrement = Cycle(10000000)
	DefaultMax       = Cycle(1000000000)
)

type requestType uint8

const (
	advanceStateRequest requestType = 0
	inspectStateRequest requestType = 1
)

type Machine struct {
	// For each request, the machine will run in increments of Increment cycles,
	// for no more than Max cycles.
	//
	// If these fields are left undefined,
	// the machine will use the DefaultIncrement and DefaultMax values.
	Increment, Max Cycle

	// Memory ranges.
	rxBufferStart uint64
	txBufferStart uint64

	inner  *emulator.Machine
	remote *emulator.RemoteMachineManager
}

// Loads loads a machine snapshot by connecting to the JSON RPC remote cartesi machine at address.
// It then checks if the machine is in a valid state to receive advance and inspect requests.
func Load(address, snapshot string, config *emulator.MachineRuntimeConfig) (*Machine, error) {
	// Creates the machine with default values for Increment and Max.
	machine := &Machine{Increment: DefaultIncrement, Max: DefaultMax}

	// Creates the remote machine manager.
	remote, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		return nil, err
	}
	machine.remote = remote

	// Creates the inner machine reference.
	inner, err := remote.LoadMachine(snapshot, config)
	if err != nil {
		machine.remote.Delete()
		return nil, err
	}
	machine.inner = inner

	// Checks if the machine is primed.
	err = machine.isPrimed()
	if err != nil {
		err = fmt.Errorf("machine is not primed: %w", err)
		return nil, errors.Join(err, machine.Destroy())
	}

	// Reads the required parameters from the configuration.
	err = machine.fillRxTxStart()
	if err != nil {
		err = fmt.Errorf("could not read rx/tx buffer start: %w", err)
		return nil, errors.Join(err, machine.Destroy())
	}

	return machine, nil
}

// Fork forks an existing cartesi machine.
func (machine *Machine) Fork() (*Machine, error) {
	// Creates the new machine based on the old machine.
	newMachine := &Machine{
		Increment:     machine.Increment,
		Max:           machine.Max,
		rxBufferStart: machine.rxBufferStart,
		txBufferStart: machine.txBufferStart,
	}

	// Forks the remote server's process.
	address, err := machine.remote.Fork()
	if err != nil {
		return nil, err
	}

	// Instantiates the new remote machine manager.
	newMachine.remote, err = emulator.NewRemoteMachineManager(address)
	if err != nil {
		format := "%w: %w at address %s "
		format += "(could not create the new remote machine manager)"
		return nil, fmt.Errorf(format, err, ErrOrphanFork, address)
	}

	// Gets the inner machine reference from the remote server.
	newMachine.inner, err = newMachine.remote.GetMachine()
	if err != nil {
		if shutdownErr := newMachine.remote.Shutdown(); shutdownErr != nil {
			format := "%w: %w at address %s "
			format += "(could not shut down the remote server)"
			return newMachine, fmt.Errorf(format, err, ErrOrphanFork, address)
		} else {
			machine.remote.Delete()
			return nil, err
		}
	}

	return newMachine, nil
}

// Destroy destroys the inner machine reference,
// shuts down the remote cartesi machine server,
// and deletes the remote machine manager.
func (machine *Machine) Destroy() (err error) {
	err = machine.inner.Destroy()
	if err != nil {
		return errors.Join(ErrMachineDestroy, err)
	}

	err = machine.remote.Shutdown()
	if err != nil {
		return errors.Join(ErrRemoteShutdown, err)
	}

	machine.remote.Delete()
	return
}

// Advance sends an input to the cartesi machine and returns the corresponding outputs.
func (machine *Machine) Advance(request []byte) ([]Response, error) {
	return machine.process(request, advanceStateRequest)
}

// Inspect sends a query to the cartesi machine and returns the corresponding reports.
func (machine *Machine) Inspect(request []byte) ([]Response, error) {
	return machine.process(request, inspectStateRequest)
}

// ------------------------------------------------------------------------------------------------
// Auxiliary
// ------------------------------------------------------------------------------------------------

// isPrimed returns returns nil if the machine is in a primed state.
// (A machine is considered primed if it is ready to receive an input.)
// Otherwise, it returns an error that indicates why the machine is not primed.
func (machine Machine) isPrimed() error {
	yieldedManually, err := machine.inner.ReadIFlagsY()
	if err != nil {
		return err
	}
	if !yieldedManually {
		return ErrNotAtManualYield
	}
	return machine.lastInputWasAccepted()
}

// lastInputWasAccepted returns nil if the last input sent to the machine was accepted.
// Otherwise, it returns an error that indicates why the input was not accepted.
func (machine Machine) lastInputWasAccepted() error {
	reason, err := machine.readYieldReason()
	if err != nil {
		return err
	}
	switch reason {
	case emulator.YieldReasonRxAccepted:
		return nil
	case emulator.YieldReasonRxRejected:
		return ErrLastInputWasRejected
	case emulator.YieldReasonTxException:
		return ErrLastInputYieldedAnException
	default:
		panic(unreachable)
	}
}

// process processes a request,
// be it an avance-state or an inspect-state request,
// and returns any collected responses.
//
// It expects the machine to be primed before execution.
// It also leaves the machine in a primed state after an execution with no errors.
func (machine *Machine) process(data []byte, t requestType) ([]Response, error) {
	// Writes the request's data.
	err := machine.inner.WriteMemory(machine.rxBufferStart, data)
	if err != nil {
		return nil, err
	}

	// Writes the request's type and length.
	fromhost := ((uint64(t) << 32) | (uint64(len(data)) & 0xffffffff))
	err = machine.inner.WriteHtifFromHostData(fromhost)
	if err != nil {
		return nil, err
	}

	// Green-lights the machine to keep running.
	err = machine.inner.ResetIFlagsY()
	if err != nil {
		return nil, err
	}

	responses, err := machine.runAndCollect()
	if err != nil {
		return responses, err
	}

	return responses, machine.lastInputWasAccepted()
}

// runAndCollect runs the machine until it yields manually.
// It returns any collected responses.
func (machine *Machine) runAndCollect() (responses []Response, _ error) {
	startingCycle, err := machine.readMachineCycle()
	if err != nil {
		return responses, err
	}

	for {
		switch reason, err := machine.runUntilYield(startingCycle); {
		case err != nil:
			return responses, err // returns with an error
		case reason == emulator.YieldManual:
			return responses, nil // returns with the responses
		case reason == emulator.YieldAutomatic:
			break // breaks from the switch to read the output/report
		default:
			panic(unreachable)
		}

		switch reason, err := machine.readYieldReason(); {
		case err != nil:
			return responses, err
		case reason == emulator.YieldReasonProgress:
			panic("TODO: What are we gonna do with progress? Reset startingCycle?")
		case reason == emulator.YieldReasonTxOutput:
			output, err := machine.readMemory()
			if err != nil {
				return responses, err
			}
			// TODO: proofs
			responses = append(responses, output)
		case reason == emulator.YieldReasonTxReport:
			report, err := machine.readMemory()
			if err != nil {
				return responses, err
			}
			responses = append(responses, report)
		default:
			panic(unreachable)
		}
	}
}

// runUntilYield runs the machine until it yields.
// It returns the yield type or an error if the machine reaches the internal cycle limit.
func (machine *Machine) runUntilYield(startingCycle Cycle) (emulator.BreakReason, error) {
	currentCycle, err := machine.readMachineCycle()
	if err != nil {
		return emulator.BreakReasonFailed, err
	}

	for currentCycle-startingCycle < machine.Max {
		reason, err := machine.inner.Run(uint64(currentCycle + machine.Increment))
		if err != nil {
			return emulator.BreakReasonFailed, err
		}
		currentCycle, err = machine.readMachineCycle()
		if err != nil {
			return emulator.BreakReasonFailed, err
		}

		switch reason {
		case emulator.BreakReasonReachedTargetMcycle:
			continue // continues to run unless the limit cycle has been reached
		case emulator.BreakReasonYieldedManually,
			emulator.BreakReasonYieldedAutomatically:
			return reason, nil // returns with the yield reason
		case emulator.BreakReasonYieldedSoftly:
			return reason, ErrYieldedSoftly
		case emulator.BreakReasonFailed:
			return reason, ErrFailed
		case emulator.BreakReasonHalted:
			return reason, ErrHalted
		default:
			panic(unreachable)
		}
	}

	return emulator.BreakReasonFailed, ErrReachedLimitCycles
}

// readMemory reads the machine's memory to retrieve the data from emmited outputs/reports.
func (machine Machine) readMemory() ([]byte, error) {
	tohost, err := machine.inner.ReadHtifToHostData()
	if err != nil {
		return nil, err
	}
	length := tohost & 0x00000000ffffffff
	return machine.inner.ReadMemory(machine.txBufferStart, length)
}

// writeRequestTypeAndLength writes to the HTIF fromhost register the request's type and length.
func (machine *Machine) writeRequestTypeAndLength(t requestType, length uint32) error {
	fromhost := ((uint64(t) << 32) | (uint64(length) & 0xffffffff))
	return machine.inner.WriteHtifFromHostData(fromhost)
}

func (machine Machine) readYieldReason() (emulator.HtifYieldReason, error) {
	value, err := machine.inner.ReadHtifToHostData()
	return emulator.HtifYieldReason(value >> 32), err
}

func (machine Machine) readMachineCycle() (Cycle, error) {
	cycle, err := machine.inner.ReadMCycle()
	return Cycle(cycle), err
}

func (machine *Machine) fillRxTxStart() error {
	config, err := machine.inner.GetInitialConfig()
	if err != nil {
		return err
	}
	machine.rxBufferStart = config.Cmio.RxBuffer.Start
	machine.txBufferStart = config.Cmio.TxBuffer.Start
	return nil
}
