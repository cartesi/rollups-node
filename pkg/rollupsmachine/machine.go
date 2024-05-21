// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/emulator"
)

// Convenient type aliases.
type (
	Cycle  = uint64
	Output = []byte
	Report = []byte
)

type requestType uint8

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

	address string

	inner  *emulator.Machine
	remote *emulator.RemoteMachineManager
}

// Load loads the machine stored at path into the remote server from address.
// It then checks if the machine is in a valid state to receive advance and inspect requests.
func Load(path, address string, config *emulator.MachineRuntimeConfig) (*RollupsMachine, error) {
	// Creates the machine with default values for Inc and Max.
	machine := &RollupsMachine{Inc: DefaultInc, Max: DefaultMax, address: address}

	// Creates the remote machine manager.
	remote, err := emulator.NewRemoteMachineManager(address)
	if err != nil {
		err = fmt.Errorf("could not create the remote machine manager: %w", err)
		return nil, errCartesiMachine(err)
	}
	machine.remote = remote

	// Loads the machine stored at path into the server.
	// Creates the inner machine reference.
	inner, err := remote.LoadMachine(path, config)
	if err != nil {
		defer machine.remote.Delete()
		err = fmt.Errorf("could not load the machine: %w", err)
		return nil, errCartesiMachine(err)
	}
	machine.inner = inner

	// Ensures that the machine is at a manual yield.
	isAtManualYield, err := machine.inner.ReadIFlagsY()
	if err != nil {
		defer machine.remote.Delete()
		err = fmt.Errorf("could not read iflagsY: %w", err)
		return nil, errors.Join(errCartesiMachine(err), machine.closeInner())
	}
	if !isAtManualYield {
		defer machine.remote.Delete()
		return nil, errors.Join(ErrNotAtManualYield, machine.closeInner())
	}

	// Ensures that the last request the machine received did not yield and exception.
	_, err = machine.lastRequestWasAccepted()
	if err != nil {
		defer machine.remote.Delete()
		return nil, errors.Join(err, machine.closeInner())
	}

	return machine, nil
}

// Fork forks an existing cartesi machine.
func (machine *RollupsMachine) Fork() (_ *RollupsMachine, address string, _ error) {
	// Creates the new machine based on the old machine.
	newMachine := &RollupsMachine{Inc: machine.Inc, Max: machine.Max}

	// Forks the remote server's process.
	address, err := machine.remote.Fork()
	if err != nil {
		err = fmt.Errorf("could not fork the machine: %w", err)
		return nil, address, errCartesiMachine(err)
	}

	// Instantiates the new remote machine manager.
	newMachine.remote, err = emulator.NewRemoteMachineManager(address)
	if err != nil {
		err = fmt.Errorf("could not create the new remote machine manager: %w", err)
		errOrphanServer := errOrphanServerWithAddress(address)
		return nil, address, errors.Join(errCartesiMachine(err), errOrphanServer)
	}

	// Gets the inner machine reference from the remote server.
	newMachine.inner, err = newMachine.remote.GetMachine()
	if err != nil {
		err = fmt.Errorf("could not get the machine from the server: %w", err)
		return nil, address, errors.Join(errCartesiMachine(err), newMachine.closeServer())
	}

	return newMachine, address, nil
}

// Hash returns the machine's merkle tree root hash.
func (machine RollupsMachine) Hash() (model.Hash, error) {
	hash, err := machine.inner.GetRootHash()
	if err != nil {
		err := fmt.Errorf("could not get the machine's root hash: %w", err)
		return model.Hash(hash), errCartesiMachine(err)
	}
	return model.Hash(hash), nil
}

// Advance sends an input to the cartesi machine.
// It returns a boolean indicating whether or not the request was accepted.
// It also returns the corresponding outputs, reports, and the hash of the outputs.
//
// If the request was not accepted, the function does not return outputs.
func (machine *RollupsMachine) Advance(input []byte) (bool, []Output, []Report, model.Hash, error) {
	var outputsHash model.Hash

	accepted, outputs, reports, err := machine.process(input, advanceStateRequest)
	if err != nil {
		return accepted, outputs, reports, outputsHash, err
	}

	if !accepted {
		return accepted, nil, reports, model.Hash{}, nil
	} else {
		hashBytes, err := machine.readMemory()
		if err != nil {
			err := fmt.Errorf("could not read the outputs' hash from the memory: %w", err)
			return accepted, outputs, reports, outputsHash, errCartesiMachine(err)
		}
		if length := len(hashBytes); length != model.HashLength {
			err := fmt.Errorf("%w (it has %d bytes)", ErrHashLength, length)
			return accepted, outputs, reports, outputsHash, err
		}
		copy(outputsHash[:], hashBytes)

		return accepted, outputs, reports, outputsHash, nil
	}
}

// Inspect sends a query to the cartesi machine.
// It returns a boolean indicating whether or not the request was accepted
// It also returns the corresponding reports.
func (machine *RollupsMachine) Inspect(query []byte) (bool, []Report, error) {
	accepted, _, reports, err := machine.process(query, inspectStateRequest)
	return accepted, reports, err
}

// Close destroys the inner cartesi machine, deletes its reference,
// shutsdown the server, and deletes the server's reference.
func (machine *RollupsMachine) Close() error {
	return errors.Join(machine.closeInner(), machine.closeServer())
}

// ------------------------------------------------------------------------------------------------

// closeInner destroys the machine and deletes its reference.
func (machine *RollupsMachine) closeInner() error {
	defer machine.inner.Delete()
	err := machine.inner.Destroy()
	if err != nil {
		err = fmt.Errorf("could not destroy the machine: %w", err)
		err = errCartesiMachine(err)
	}
	return err
}

// closeServer shutsdown the server and deletes its reference.
func (machine *RollupsMachine) closeServer() error {
	defer machine.remote.Delete()
	err := machine.remote.Shutdown()
	if err != nil {
		err = fmt.Errorf("could not shutdown the server: %w", err)
		err = errCartesiMachine(err)
		err = errors.Join(err, errOrphanServerWithAddress(machine.address))
	}
	return err
}

// lastRequestWasAccepted returns true if the last request was accepted and false otherwise.
//
// The machine MUST be at a manual yield when calling this function.
func (machine *RollupsMachine) lastRequestWasAccepted() (bool, error) {
	yieldReason, err := machine.readYieldReason()
	if err != nil {
		err := fmt.Errorf("could not read the yield reason: %w", err)
		return false, errCartesiMachine(err)
	}
	switch yieldReason { //nolint:exhaustive
	case emulator.ManualYieldReasonAccepted:
		return true, nil
	case emulator.ManualYieldReasonRejected:
		return false, nil
	case emulator.ManualYieldReasonException:
		return false, ErrException
	default:
		panic(unreachable)
	}
}

// process processes a request, be it an avance-state or an inspect-state request.
// It returns the accepted state and any collected responses.
//
// It expects the machine to be ready to receive requests before execution,
// and leaves the machine in a state ready to receive requests after an execution with no errors.
func (machine *RollupsMachine) process(
	request []byte,
	requestType requestType,
) (accepted bool, _ []Output, _ []Report, _ error) {
	// Writes the request's data.
	err := machine.inner.WriteMemory(emulator.CmioRxBufferStart, request)
	if err != nil {
		err := fmt.Errorf("could not write the request's data to the memory: %w", err)
		return false, nil, nil, errCartesiMachine(err)
	}

	// Writes the request's type and length.
	fromhost := ((uint64(requestType) << 32) | (uint64(len(request)) & 0xffffffff)) //nolint:mnd
	err = machine.inner.WriteHtifFromHostData(fromhost)
	if err != nil {
		err := fmt.Errorf("could not write HTIF fromhost data: %w", err)
		return false, nil, nil, errCartesiMachine(err)
	}

	// Green-lights the machine to keep running.
	err = machine.inner.ResetIFlagsY()
	if err != nil {
		err := fmt.Errorf("could not reset iflagsY: %w", err)
		return false, nil, nil, errCartesiMachine(err)
	}

	outputs, reports, err := machine.runAndCollect()
	if err != nil {
		return false, outputs, reports, err
	}

	accepted, err = machine.lastRequestWasAccepted()

	return accepted, outputs, reports, err
}

// runAndCollect runs the machine until it manually yields.
// It returns any collected responses.
func (machine *RollupsMachine) runAndCollect() ([]Output, []Report, error) {
	startingCycle, err := machine.readMachineCycle()
	if err != nil {
		err := fmt.Errorf("could not read the machine's cycle: %w", err)
		return nil, nil, errCartesiMachine(err)
	}
	maxCycle := startingCycle + machine.Max
	slog.Debug("runAndCollect",
		"startingCycle", startingCycle,
		"maxCycle", maxCycle,
		"leftover", maxCycle-startingCycle)

	outputs := []Output{}
	reports := []Report{}
	for {
		var (
			breakReason emulator.BreakReason
			err         error
		)
		breakReason, startingCycle, err = machine.run(startingCycle, maxCycle)
		if err != nil {
			return outputs, reports, err
		}

		switch breakReason { //nolint:exhaustive
		case emulator.BreakReasonYieldedManually:
			return outputs, reports, nil // returns with the responses
		case emulator.BreakReasonYieldedAutomatically:
			break // breaks from the switch to read the outputs/reports
		default:
			panic(unreachable)
		}

		yieldReason, err := machine.readYieldReason()
		if err != nil {
			err := fmt.Errorf("could not read the yield reason: %w", err)
			return outputs, reports, errCartesiMachine(err)
		}

		switch yieldReason { //nolint:exhaustive
		case emulator.AutomaticYieldReasonProgress:
			return outputs, reports, ErrProgress
		case emulator.AutomaticYieldReasonOutput:
			output, err := machine.readMemory()
			if err != nil {
				err := fmt.Errorf("could not read the output from the memory: %w", err)
				return outputs, reports, errCartesiMachine(err)
			}
			if len(outputs) == maxOutputs {
				return outputs, reports, ErrOutputsLimitExceeded
			}
			outputs = append(outputs, output)
		case emulator.AutomaticYieldReasonReport:
			report, err := machine.readMemory()
			if err != nil {
				err := fmt.Errorf("could not read the report from the memory: %w", err)
				return outputs, reports, errCartesiMachine(err)
			}
			reports = append(reports, report)
		default:
			panic(unreachable)
		}
	}
}

// run runs the machine until it yields.
//
// If there are no errors, it returns one of two possible break reasons:
//   - emulator.BreakReasonYieldedManually
//   - emulator.BreakReasonYieldedAutomatically
//
// Otherwise, it returns one of four possible errors:
//   - ErrCartesiMachine
//   - ErrHalted
//   - ErrSoftYield
//   - ErrCycleLimitExceeded
func (machine *RollupsMachine) run(
	startingCycle Cycle,
	maxCycle Cycle,
) (emulator.BreakReason, Cycle, error) {
	currentCycle := startingCycle
	slog.Debug("run", "startingCycle", startingCycle, "leftover", maxCycle-startingCycle)

	for {
		// Calculates the increment.
		increment := min(machine.Inc, maxCycle-currentCycle)

		// Returns with an error if the next run would exceed Max cycles.
		if currentCycle+increment >= maxCycle {
			return emulator.BreakReasonReachedTargetMcycle, currentCycle, ErrCycleLimitExceeded
		}

		// Runs the machine.
		breakReason, err := machine.inner.Run(currentCycle + increment)
		if err != nil {
			assert(breakReason == emulator.BreakReasonFailed, breakReason.String())
			err := fmt.Errorf("machine run failed: %w", err)
			return breakReason, currentCycle, errCartesiMachine(err)
		}

		// Gets the current cycle.
		currentCycle, err = machine.readMachineCycle()
		if err != nil {
			err := fmt.Errorf("could not read the machine's cycle: %w", err)
			return emulator.BreakReasonFailed, currentCycle, errCartesiMachine(err)
		}
		slog.Debug("run", "currentCycle", currentCycle, "leftover", maxCycle-currentCycle)

		switch breakReason {
		case emulator.BreakReasonFailed:
			panic(unreachable) // covered above
		case emulator.BreakReasonHalted:
			return emulator.BreakReasonHalted, currentCycle, ErrHalted
		case emulator.BreakReasonYieldedManually, emulator.BreakReasonYieldedAutomatically:
			return breakReason, currentCycle, nil // returns with the break reason
		case emulator.BreakReasonYieldedSoftly:
			return emulator.BreakReasonYieldedSoftly, currentCycle, ErrSoftYield
		case emulator.BreakReasonReachedTargetMcycle:
			continue // keeps on running
		default:
			panic(unreachable)
		}
	}
}

// ------------------------------------------------------------------------------------------------

// readMemory reads the machine's memory to retrieve the data from emitted outputs/reports.
func (machine *RollupsMachine) readMemory() ([]byte, error) {
	tohost, err := machine.inner.ReadHtifToHostData()
	if err != nil {
		return nil, err
	}
	length := tohost & 0x00000000ffffffff //nolint:mnd
	return machine.inner.ReadMemory(emulator.CmioTxBufferStart, length)
}

func (machine *RollupsMachine) readYieldReason() (emulator.HtifYieldReason, error) {
	value, err := machine.inner.ReadHtifToHostData()
	return emulator.HtifYieldReason(value >> 32), err //nolint:mnd
}

func (machine *RollupsMachine) readMachineCycle() (Cycle, error) {
	cycle, err := machine.inner.ReadMCycle()
	return Cycle(cycle), err
}

// ------------------------------------------------------------------------------------------------

func assert(condition bool, s string) {
	if !condition {
		panic("assertion error: " + s)
	}
}
