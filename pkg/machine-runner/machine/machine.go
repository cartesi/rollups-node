// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/machine-runner/machine/binding"
)

type Response []byte

const (
	DefaultIncrement = binding.Cycle(10000000)
	DefaultMax       = binding.Cycle(1000000000)
)

type Machine struct {
	// For each request, the machine will run in increments of Increment cycles,
	// for no more than Max cycles.
	//
	// If these fields are left undefined,
	// the machine will use the DefaultIncrement and DefaultMax values.
	Increment, Max binding.Cycle

	// Remote connection manager.
	remote *binding.Remote

	// Machine binding.
	binding *binding.Binding
}

// Loads loads a machine snapshot by connecting to the JSON RPC remote cartesi machine at address.
// It then checks if the machine is in a valid state to receive advance and inspect requests.
func Load(address, snapshot string, config binding.RuntimeConfig) (*Machine, error) {
	// Creates the machine with default values for Increment and Max.
	machine := &Machine{Increment: DefaultIncrement, Max: DefaultMax}

	// Creates the remote connection manager.
	remote, err := binding.NewRemote(address)
	if err != nil {
		return nil, err
	}
	machine.remote = remote

	// Creates the binding.
	binding, err := binding.Load(snapshot, machine.remote, config)
	if err != nil {
		machine.remote.Delete()
		return nil, err
	}
	machine.binding = binding

	// Checks if the machine is primed.
	err = machine.isPrimed()
	if err != nil {
		err = fmt.Errorf("machine is not primed: %w", err)
		machine.remote.Delete()
		destroyErr := machine.binding.Destroy()
		if destroyErr != nil {
			return machine, errors.Join(err, destroyErr)
		} else {
			return nil, err
		}
	}

	return machine, nil
}

// Fork forks an existing cartesi machine.
func (machine *Machine) Fork() (*Machine, error) {
	// Creates the new machine with the old Increment and Max values.
	newMachine := &Machine{Increment: machine.Increment, Max: machine.Max}

	// Forks the server's process.
	address, err := machine.remote.Fork()
	if err != nil {
		return nil, err
	}

	// Creates the new remote connection manager.
	remote, err := binding.NewRemote(address)
	if err != nil {
		format := "remote cartesi machine at %s was left orphan"
		format += "(could not create the remote connection manager)"
		slog.Error(fmt.Sprintf(format, address))
		return nil, err
	}
	newMachine.remote = remote

	// Creates the new binding.
	binding, err := binding.From(remote)
	if err != nil {
		shutdownErr := remote.Shutdown()
		if shutdownErr != nil {
			format := "remote cartesi machine at %s was left orphan"
			format += "(could not shut down the server using the remote connection manager)"
			slog.Error(fmt.Sprintf(format, address))
			return newMachine, errors.Join(err, shutdownErr)
		} else {
			remote.Delete()
			return nil, err
		}
	}
	newMachine.binding = binding

	return newMachine, nil
}

// Destroy shuts down the remote cartesi machine,
// deletes the remote connection manager,
// and destroys the machine binding.
func (machine *Machine) Destroy() error {
	if machine.remote != nil {
		if err := machine.remote.Shutdown(); err != nil {
			return errors.Join(ErrRemoteShutdown, err)
		}
		machine.remote.Delete()
	}
	if machine.binding != nil {
		if err := machine.binding.Destroy(); err != nil {
			return errors.Join(ErrBindingDestroy, err)
		}
	}
	return nil
}

// Advance sends an input to the cartesi machine and returns the corresponding outputs.
func (machine *Machine) Advance(request []byte) ([]Response, error) {
	return machine.process(request, binding.AdvanceStateRequest)
}

// Inspect sends a query to the cartesi machine and returns the corresponding reports.
func (machine *Machine) Inspect(request []byte) ([]Response, error) {
	return machine.process(request, binding.InspectStateRequest)
}

// ------------------------------------------------------------------------------------------------
// Auxiliary
// ------------------------------------------------------------------------------------------------

// isPrimed returns returns nil if the machine is in a primed state.
// (A machine is considered primed if it is ready to receive an input.)
// Otherwise, it returns an error that indicates why the machine is not primed.
func (machine Machine) isPrimed() error {
	yieldedManually, err := machine.binding.ReadIflagsY()
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
	reason, err := machine.binding.ReadYieldReason()
	if err != nil {
		return err
	}
	switch reason {
	case binding.YieldReasonRxAccepted:
		return nil
	case binding.YieldReasonRxRejected:
		return ErrLastInputWasRejected
	case binding.YieldReasonTxException:
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
func (machine *Machine) process(data []byte, t binding.RequestType) ([]Response, error) {
	// Writes the request's data.
	err := machine.binding.WriteMemory(machine.binding.RxBufferStart, data)
	if err != nil {
		return nil, err
	}

	// Writes the request's type and length.
	fromhost := ((uint64(t) << 32) | (uint64(len(data)) & 0xffffffff))
	err = machine.binding.WriteHtifFromHostData(fromhost)
	if err != nil {
		return nil, err
	}

	// Green-lights the machine to keep running.
	err = machine.binding.ResetIflagsY()
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
	startingCycle, err := machine.binding.ReadMachineCycle()
	if err != nil {
		return responses, err
	}

	for {
		switch reason, err := machine.runUntilYield(startingCycle); {
		case err != nil:
			return responses, err // returns with an error
		case reason == binding.BreakReasonYieldedManually:
			return responses, nil // returns with the responses
		case reason == binding.BreakReasonYieldedAutomatically:
			break // breaks from the switch to read the output/report
		default:
			panic(unreachable)
		}

		switch reason, err := machine.binding.ReadYieldReason(); {
		case err != nil:
			return responses, err
		case reason == binding.YieldReasonProgress:
			panic("TODO: What are we gonna do with progress? Reset startingCycle?")
		case reason == binding.YieldReasonTxOutput:
			output, err := machine.readMemory()
			if err != nil {
				return responses, err
			}
			// TODO: proofs
			responses = append(responses, output)
		case reason == binding.YieldReasonTxReport:
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
func (machine *Machine) runUntilYield(startingCycle binding.Cycle) (binding.BreakReason, error) {
	currentCycle, err := machine.binding.ReadMachineCycle()
	if err != nil {
		return binding.InvalidBreakReason, err
	}

	for currentCycle-startingCycle < machine.Max {
		reason, err := machine.binding.Run(currentCycle + machine.Increment)
		if err != nil {
			return binding.InvalidBreakReason, err
		}
		currentCycle, err = machine.binding.ReadMachineCycle()
		if err != nil {
			return binding.InvalidBreakReason, err
		}

		switch reason {
		case binding.BreakReasonReachedTargetCycle:
			continue // continues to run unless the limit cycle has been reached
		case binding.BreakReasonYieldedManually,
			binding.BreakReasonYieldedAutomatically:
			return reason, nil // returns with the yield reason
		case binding.BreakReasonYieldedSoftly:
			return reason, ErrYieldedSoftly
		case binding.BreakReasonFailed:
			return reason, ErrFailed
		case binding.BreakReasonHalted:
			return reason, ErrHalted
		default:
			panic(unreachable)
		}
	}

	return binding.InvalidBreakReason, ErrReachedLimitCycles
}

// readMemory reads the machine's memory to retrieve the data from emmited outputs/reports.
func (machine Machine) readMemory() ([]byte, error) {
	tohost, err := machine.binding.ReadHtifToHostData()
	if err != nil {
		return nil, err
	}
	length := tohost & 0x00000000ffffffff
	return machine.binding.ReadMemory(machine.binding.TxBufferStart, length)
}

// writeRequestTypeAndLength writes to the HTIF fromhost register the request's type and length.
func (machine *Machine) writeRequestTypeAndLength(t binding.RequestType, length uint32) error {
	fromhost := ((uint64(t) << 32) | (uint64(length) & 0xffffffff))
	return machine.binding.WriteHtifFromHostData(fromhost)
}
