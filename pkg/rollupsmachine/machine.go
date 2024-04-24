// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/model"
)

type (
	Cycle  = uint64
	Output = []byte
	Report = []byte

	requestType uint8
)

const (
	DefaultInc = Cycle(10000000)
	DefaultMax = Cycle(1000000000)

	advanceStateRequest requestType = 0
	inspectStateRequest requestType = 1

	maxOutputs = 65536 // 2^16
)

// A RollupsMachine wraps an emulator.Machine and provides five basic functions:
// Fork, Destroy, Hash, Advance and Inspect.
type RollupsMachine struct {
	// For each request, the machine will run in increments of Inc cycles,
	// for no more than Max cycles.
	//
	// If these fields are left undefined,
	// the machine will use the DefaultInc and DefaultMax values.
	Inc, Max Cycle

	inner  *emulator.Machine
	remote *emulator.RemoteMachineManager
}

// Load loads the machine stored at path
// by connecting to the JSON RPC remote cartesi machine at address.
// It then checks if the machine is in a valid state to receive advance and inspect requests.
func Load(path, address string, config *emulator.MachineRuntimeConfig) (*RollupsMachine, error) {
	// Creates the machine with default values for Inc and Max.
	machine := &RollupsMachine{Inc: DefaultInc, Max: DefaultMax}

	// Creates the remote machine manager.
	remote, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		return nil, errors.Join(ErrNewRemoteMachineManager, err)
	}
	machine.remote = remote

	// Loads the machine stored at path into the server.
	// Creates the inner machine reference.
	inner, err := remote.LoadMachine(path, config)
	if err != nil {
		err = errors.Join(err, machine.remote.Shutdown())
		machine.remote.Delete()
		return nil, errors.Join(ErrRemoteLoadMachine, err)
	}
	machine.inner = inner

	// Checks if the machine is ready to receive requests.
	err = machine.isReadyForRequests()
	if err != nil {
		return nil, errors.Join(ErrNotReadyForRequests, err, machine.Destroy())
	}

	return machine, nil
}

// Fork forks an existing cartesi machine.
func (machine RollupsMachine) Fork() (*RollupsMachine, error) {
	// Creates the new machine based on the old machine.
	newMachine := &RollupsMachine{Inc: machine.Inc, Max: machine.Max}

	// TODO : ask canal da machine
	// Forks the remote server's process.
	address, err := machine.remote.Fork()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFork, err)
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
		defer machine.remote.Delete()
		if shutdownErr := newMachine.remote.Shutdown(); shutdownErr != nil {
			format := "%w: %w at address %s "
			format += "(could not shut down the remote server)"
			return newMachine, fmt.Errorf(format, err, ErrOrphanFork, address)
		} else {
			return nil, err
		}
	}

	return newMachine, nil
}

// Destroy destroys the inner machine reference,
// shuts down the remote cartesi machine server,
// and deletes the remote machine manager.
func (machine *RollupsMachine) Destroy() error {
	errMachineDestroy := machine.inner.Destroy()
	if errMachineDestroy != nil {
		errMachineDestroy = fmt.Errorf("%w: %w", ErrMachineDestroy, errMachineDestroy)
	}

	errRemoteShutdown := machine.remote.Shutdown()
	if errRemoteShutdown != nil {
		errRemoteShutdown = fmt.Errorf("%w: %w", ErrRemoteShutdown, errRemoteShutdown)
	}

	machine.remote.Delete()

	if errMachineDestroy != nil || errRemoteShutdown != nil {
		return errors.Join(errMachineDestroy, errRemoteShutdown)
	}

	return nil
}

// Hash returns the machine's merkle tree root hash.
func (machine RollupsMachine) Hash() (model.Hash, error) {
	hash, err := machine.inner.GetRootHash()
	return model.Hash(hash), err
}

// Advance sends an input to the cartesi machine
// and returns the corresponding outputs (with their hash) and reports.
func (machine *RollupsMachine) Advance(input []byte) ([]Output, []Report, model.Hash, error) {
	var hash model.Hash

	outputs, reports, err := machine.process(input, advanceStateRequest)
	if err != nil {
		return outputs, reports, hash, err
	}

	hashBytes, err := machine.readMemory()
	if err != nil {
		return outputs, reports, hash, err
	}
	if size := len(hashBytes); size != model.HashSize {
		err = fmt.Errorf("%w (it has %d bytes)", ErrHashSize, size)
		return outputs, reports, hash, err
	}
	copy(hash[:], hashBytes)

	return outputs, reports, hash, nil
}

// Inspect sends a query to the cartesi machine and returns the corresponding reports.
func (machine *RollupsMachine) Inspect(query []byte) ([]Report, error) {
	_, reports, err := machine.process(query, inspectStateRequest)
	return reports, err
}

// ------------------------------------------------------------------------------------------------
// Auxiliary
// ------------------------------------------------------------------------------------------------

// isReadyForRequests returns nil if the machine is ready to receive a request,
// otherwise, it returns an error that indicates why that is not the case.
//
// A machine is ready to receive requests if
// (1) it is at a manual yield and
// (2) the last input it received was accepted.
func (machine RollupsMachine) isReadyForRequests() error {
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
//
// The machine must be at a manual yield when calling this function.
func (machine RollupsMachine) lastInputWasAccepted() error {
	reason, err := machine.readYieldReason()
	if err != nil {
		return err
	}
	switch reason {
	case emulator.ManualYieldReasonAccepted:
		return nil
	case emulator.ManualYieldReasonRejected:
		return ErrLastInputWasRejected
	case emulator.ManualYieldReasonException:
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
func (machine *RollupsMachine) process(request []byte, t requestType) ([]Output, []Report, error) {
	// Writes the request's data.
	err := machine.inner.WriteMemory(emulator.CmioRxBufferStart, request)
	if err != nil {
		return nil, nil, err
	}

	// Writes the request's type and length.
	fromhost := ((uint64(t) << 32) | (uint64(len(request)) & 0xffffffff))
	err = machine.inner.WriteHtifFromHostData(fromhost)
	if err != nil {
		return nil, nil, err
	}

	// Green-lights the machine to keep running.
	err = machine.inner.ResetIFlagsY()
	if err != nil {
		return nil, nil, err
	}

	outputs, reports, err := machine.runAndCollect()
	if err != nil {
		return outputs, reports, err
	}

	return outputs, reports, machine.lastInputWasAccepted()
}

// runAndCollect runs the machine until it yields manually.
// It returns any collected responses.
// (The slices with the responses will never be nil, even in case of errors.)
func (machine *RollupsMachine) runAndCollect() ([]Output, []Report, error) {
	outputs := []Output{}
	reports := []Report{}

	startingCycle, err := machine.readMachineCycle()
	if err != nil {
		return outputs, reports, err
	}

	for {
		switch reason, err := machine.runUntilYield(startingCycle); {
		case err != nil:
			return outputs, reports, err // returns with an error
		case reason == emulator.BreakReasonYieldedManually:
			return outputs, reports, nil // returns with the responses
		case reason == emulator.BreakReasonYieldedAutomatically:
			break // breaks from the switch to read the output/report
		default:
			panic(unreachable)
		}

		switch reason, err := machine.readYieldReason(); {
		case err != nil:
			return outputs, reports, err
		case reason == emulator.AutomaticYieldReasonProgress:
			return outputs, reports, ErrYieldedWithProgress
		case reason == emulator.AutomaticYieldReasonOutput:
			output, err := machine.readMemory()
			if err != nil {
				return outputs, reports, err
			} else {
				outputs = append(outputs, output)
				if len(outputs) > maxOutputs {
					return outputs, reports, ErrMaxOutputs
				}
			}
		case reason == emulator.AutomaticYieldReasonReport:
			report, err := machine.readMemory()
			if err != nil {
				return outputs, reports, err
			} else {
				reports = append(reports, report)
			}
		default:
			panic(unreachable)
		}
	}
}

// runUntilYield runs the machine until it yields.
// It returns the yield type or an error if the machine reaches the internal cycle limit.
func (machine *RollupsMachine) runUntilYield(startingCycle Cycle) (emulator.BreakReason, error) {
	currentCycle, err := machine.readMachineCycle()
	if err != nil {
		return emulator.BreakReasonFailed, err
	}

	for currentCycle-startingCycle < machine.Max {
		reason, err := machine.inner.Run(uint64(currentCycle + machine.Inc))
		if err != nil {
			return emulator.BreakReasonFailed, err
		}
		currentCycle, err = machine.readMachineCycle()
		if err != nil {
			return emulator.BreakReasonFailed, err
		}

		// TODO : diego server manager parametros de configuração
		switch reason {
		case emulator.BreakReasonReachedTargetMcycle:
			continue // continues to run unless the limit cycle has been reached
		case emulator.BreakReasonYieldedManually, emulator.BreakReasonYieldedAutomatically:
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

	return emulator.BreakReasonFailed, ErrMaxCycles
}

// readMemory reads the machine's memory to retrieve the data from emmited outputs/reports.
func (machine RollupsMachine) readMemory() ([]byte, error) {
	tohost, err := machine.inner.ReadHtifToHostData()
	if err != nil {
		return nil, err
	}
	length := tohost & 0x00000000ffffffff
	return machine.inner.ReadMemory(emulator.CmioTxBufferStart, length)
}

// writeRequestTypeAndLength writes to the HTIF fromhost register the request's type and length.
func (machine *RollupsMachine) writeRequestTypeAndLength(t requestType, length uint32) error {
	fromhost := ((uint64(t) << 32) | (uint64(length) & 0xffffffff))
	return machine.inner.WriteHtifFromHostData(fromhost)
}

func (machine RollupsMachine) readYieldReason() (emulator.HtifYieldReason, error) {
	value, err := machine.inner.ReadHtifToHostData()
	return emulator.HtifYieldReason(value >> 32), err
}

func (machine RollupsMachine) readMachineCycle() (Cycle, error) {
	cycle, err := machine.inner.ReadMCycle()
	return Cycle(cycle), err
}
