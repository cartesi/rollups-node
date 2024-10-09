// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package rollupsmachine provides the RollupsMachine type, the Input type, and the DecodeOutput
// function.
package rollupsmachine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/emulator"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

const (
	maxOutputs    = 65536 // 2^16
	addressLength = 20
	hashLength    = 32
)

// Convenient type aliases.
type (
	Cycle   = uint64
	Output  = []byte
	Report  = []byte
	Address = [addressLength]byte
	Hash    = [hashLength]byte
)

// The RollupsMachine interface covers the four core rollups-oriented functionalities of a cartesi
// machine: forking, getting the merkle tree's root hash, sending advance-state requests, and
// sending inspect-state requests.
type RollupsMachine interface {
	// Fork forks the machine.
	Fork(context.Context) (RollupsMachine, error)

	// Hash returns the machine's merkle tree root hash.
	Hash(context.Context) (Hash, error)

	// Advance sends an input to the machine.
	// It returns a boolean indicating whether or not the request was accepted.
	// It also returns the corresponding outputs, reports, and the hash of the outputs.
	// In case the request is not accepted, the function does not return outputs.
	Advance(_ context.Context, input []byte) (bool, []Output, []Report, Hash, error)

	// Inspect sends a query to the machine.
	// It returns a boolean indicating whether or not the request was accepted
	// It also returns the corresponding reports.
	Inspect(_ context.Context, query []byte) (bool, []Report, error)

	// Close closes the inner cartesi machine.
	// It returns nil if the machine has already been closed.
	Close(context.Context) error
}

// ------------------------------------------------------------------------------------------------

// rollupsMachine implements the RollupsMachine interface by wrapping a
// cartesimachine.CartesiMachine.
//
// When processing an advance-state or an inspect-state request, the machine will run in increments
// of inc cycles, for no more than max cycles.
type rollupsMachine struct {
	inner cartesimachine.CartesiMachine

	inc, max Cycle
}

// New checks if the provided cartesimachine.CartesiMachine is in a valid state to receive
// advance and inspect requests. If so, New returns a rollupsMachine that wraps the
// cartesimachine.CartesiMachine.
func New(ctx context.Context,
	inner cartesimachine.CartesiMachine,
	inc, max Cycle,
) (RollupsMachine, error) {
	machine := &rollupsMachine{inner: inner, inc: inc, max: max}

	// Ensures that the machine is at a manual yield.
	isAtManualYield, err := machine.inner.IsAtManualYield(ctx)
	if err != nil {
		return nil, err
	}
	if !isAtManualYield {
		return nil, ErrNotAtManualYield
	}

	// Ensures that the last request the machine received did not yield an exception.
	_, err = machine.lastRequestWasAccepted(ctx)
	if err != nil {
		return nil, err
	}

	return machine, nil
}

func (machine *rollupsMachine) Fork(ctx context.Context) (RollupsMachine, error) {
	inner, err := machine.inner.Fork(ctx)
	if err != nil {
		return nil, err
	}
	return &rollupsMachine{inner: inner, inc: machine.inc, max: machine.max}, nil
}

func (machine rollupsMachine) Hash(ctx context.Context) (Hash, error) {
	return machine.inner.ReadHash(ctx)
}

func (machine *rollupsMachine) Advance(
	ctx context.Context,
	input []byte,
) (bool, []Output, []Report, Hash, error) {
	requestType := cartesimachine.AdvanceStateRequest
	accepted, outputs, reports, err := machine.process(ctx, input, requestType)
	if err != nil {
		return accepted, outputs, reports, Hash{}, err
	}

	if !accepted {
		return accepted, nil, reports, Hash{}, nil
	} else {
		hashBytes, err := machine.inner.ReadMemory(ctx)
		if err != nil {
			err = fmt.Errorf("could not read the outputs' hash: %w", err)
			return accepted, outputs, reports, Hash{}, err
		}
		if length := len(hashBytes); length != hashLength {
			err = fmt.Errorf("%w (it has %d bytes)", ErrHashLength, length)
			return accepted, outputs, reports, Hash{}, err
		}
		var outputsHash Hash
		copy(outputsHash[:], hashBytes)
		return accepted, outputs, reports, outputsHash, nil
	}
}

func (machine *rollupsMachine) Inspect(ctx context.Context, query []byte) (bool, []Report, error) {
	accepted, _, reports, err := machine.process(ctx, query, cartesimachine.InspectStateRequest)
	return accepted, reports, err
}

func (machine *rollupsMachine) Close(ctx context.Context) error {
	if machine.inner == nil {
		return nil
	}
	err := machine.inner.Close(ctx)
	machine.inner = nil
	return err
}

// ------------------------------------------------------------------------------------------------

// lastRequestWasAccepted returns true if the last request was accepted and false otherwise.
// It returns the ErrException error if the last request yielded an exception.
//
// The machine MUST be at a manual yield when calling this function.
func (machine *rollupsMachine) lastRequestWasAccepted(ctx context.Context) (bool, error) {
	yieldReason, err := machine.inner.ReadYieldReason(ctx)
	if err != nil {
		return false, err
	}
	switch yieldReason { //nolint:exhaustive
	case emulator.ManualYieldReasonAccepted:
		return true, nil
	case emulator.ManualYieldReasonRejected:
		return false, nil
	case emulator.ManualYieldReasonException:
		return false, ErrException
	default:
		panic(ErrUnreachable)
	}
}

// process processes a request, be it an advance-state or an inspect-state request.
// It returns the accepted state and any collected responses.
//
// It expects the machine to be ready to receive requests before execution,
// and leaves the machine in a state ready to receive requests after an execution with no errors.
func (machine *rollupsMachine) process(
	ctx context.Context,
	request []byte,
	requestType cartesimachine.RequestType,
) (accepted bool, _ []Output, _ []Report, _ error) {
	if length := uint(len(request)); length > machine.inner.PayloadLengthLimit() {
		return false, nil, nil, ErrPayloadLengthLimitExceeded
	}

	// Writes the request.
	err := machine.inner.WriteRequest(ctx, request, requestType)
	if err != nil {
		return false, nil, nil, err
	}

	// Green-lights the machine to keep running.
	err = machine.inner.Continue(ctx)
	if err != nil {
		return false, nil, nil, err
	}

	outputs, reports, err := machine.run(ctx)
	if err != nil {
		return false, outputs, reports, err
	}

	accepted, err = machine.lastRequestWasAccepted(ctx)

	return accepted, outputs, reports, err
}

type yieldType uint8

const (
	manualYield yieldType = iota
	automaticYield
)

// run runs the machine until it manually yields.
// It returns any collected responses.
func (machine *rollupsMachine) run(ctx context.Context) ([]Output, []Report, error) {
	currentCycle, err := machine.inner.ReadCycle(ctx)
	if err != nil {
		return nil, nil, err
	}

	limitCycle := currentCycle + machine.max
	slog.Debug("run",
		"startingCycle", currentCycle,
		"limitCycle", limitCycle,
		"leftover", limitCycle-currentCycle)

	outputs := []Output{}
	reports := []Report{}

	for {
		var yt *yieldType
		var err error

		// Steps the machine as many times as needed until it manually/automatically yields.
		for yt == nil {
			yt, currentCycle, err = machine.step(ctx, currentCycle, limitCycle)
			if err != nil {
				return outputs, reports, err
			}
		}

		// Returns with the responses when the machine manually yields.
		if *yt == manualYield {
			return outputs, reports, nil
		}

		// Asserts the machine yielded automatically.
		if *yt != automaticYield {
			panic(ErrUnreachable)
		}

		yieldReason, err := machine.inner.ReadYieldReason(ctx)
		if err != nil {
			return outputs, reports, err
		}

		switch yieldReason { //nolint:exhaustive
		case emulator.AutomaticYieldReasonProgress:
			return outputs, reports, ErrProgress
		case emulator.AutomaticYieldReasonOutput:
			output, err := machine.inner.ReadMemory(ctx)
			if err != nil {
				return outputs, reports, fmt.Errorf("could not read the output: %w", err)
			}
			if len(outputs) == maxOutputs {
				return outputs, reports, ErrOutputsLimitExceeded
			}
			outputs = append(outputs, output)
		case emulator.AutomaticYieldReasonReport:
			report, err := machine.inner.ReadMemory(ctx)
			if err != nil {
				return outputs, reports, fmt.Errorf("could not read the report: %w", err)
			}
			reports = append(reports, report)
		default:
			panic(ErrUnreachable)
		}
	}
}

// step runs the machine for at most machine.inc cycles (or the amount of cycles left to reach
// limitCycle, whichever is the lowest).
// It returns the yield type and the machine cycle after the step.
// If the machine did not manually/automatically yield, the yield type will be nil (meaning step
// must be called again to complete the computation).
func (machine *rollupsMachine) step(ctx context.Context,
	currentCycle Cycle,
	limitCycle Cycle,
) (*yieldType, Cycle, error) {
	startingCycle := currentCycle

	// Returns with an error if the next run would exceed limitCycle.
	if currentCycle >= limitCycle && machine.inc != 0 {
		return nil, 0, ErrCycleLimitExceeded
	}

	// Calculates the increment.
	increment := min(machine.inc, limitCycle-currentCycle)

	// Runs the machine.
	breakReason, err := machine.inner.Run(ctx, currentCycle+increment)
	if err != nil {
		return nil, 0, err
	}

	// Gets the current cycle.
	currentCycle, err = machine.inner.ReadCycle(ctx)
	if err != nil {
		return nil, 0, err
	}

	slog.Debug("step",
		"startingCycle", startingCycle,
		"increment", increment,
		"currentCycle", currentCycle,
		"leftover", limitCycle-currentCycle,
		"breakReason", breakReason)

	switch breakReason {
	case emulator.BreakReasonYieldedManually:
		yt := manualYield
		return &yt, currentCycle, nil // returns with the yield type
	case emulator.BreakReasonYieldedAutomatically:
		yt := automaticYield
		return &yt, currentCycle, nil // returns with the yield type
	case emulator.BreakReasonReachedTargetMcycle:
		return nil, currentCycle, nil // returns with no yield type
	case emulator.BreakReasonHalted:
		return nil, 0, ErrHalted
	case emulator.BreakReasonYieldedSoftly:
		return nil, 0, ErrSoftYield
	case emulator.BreakReasonFailed:
		fallthrough // covered by inner.Run()
	default:
		panic(ErrUnreachable)
	}
}
