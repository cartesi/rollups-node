// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
)

// RequestType represents the type of request to send to the machine
type requestType uint8

const (
	AdvanceStateRequest requestType = 0x0
	InspectStateRequest requestType = 0x1
)

type yieldType uint8

const (
	AutomaticYield yieldType = 0x0
	ManualYield    yieldType = 0x1
)

type automaticYieldReason uint16

const (
	AutomaticYieldReasonProgress automaticYieldReason = 0x1
	AutomaticYieldReasonOutput   automaticYieldReason = 0x2
	AutomaticYieldReasonReport   automaticYieldReason = 0x4
)

type manualYieldReason uint16

const (
	ManualYieldReasonAccepted  manualYieldReason = 0x1
	ManualYieldReasonRejected  manualYieldReason = 0x2
	ManualYieldReasonException manualYieldReason = 0x4
)

// Constants
const maxOutputs = 65536 // 2^16

// machineImpl implements the Machine interface by wrapping an emulator.RemoteMachine
type machineImpl struct {
	backend Backend

	address string // address of the JSON RPC remote cartesi machine server
	pid     uint32 // process ID of the machine server
	params  model.ExecutionParameters
	logger  *slog.Logger
}

// Fork creates a new machine instance by forking the current one
func (m *machineImpl) Fork(ctx context.Context) (Machine, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	// Forks the server.
	newServer, address, _, err := m.backend.ForkServer(m.params.FastDeadline)
	if err != nil {
		err = fmt.Errorf("could not fork the machine: %w", err)
		return nil, errors.Join(ErrMachineInternal, err)
	}

	// Create a new machine with the forked server
	newMachine := &machineImpl{
		backend: newServer,
		address: address,
		params:  m.params,
		logger:  m.logger,
	}

	return newMachine, nil
}

// Hash returns the machine's merkle tree root hash
func (m *machineImpl) Hash(ctx context.Context) (Hash, error) {
	hash := Hash{}
	if err := checkContext(ctx); err != nil {
		return hash, err
	}

	hashSlice, err := m.backend.GetRootHash(m.params.LoadDeadline)
	if err != nil {
		err := fmt.Errorf("could not get the machine's root hash: %w", err)
		return hash, errors.Join(ErrMachineInternal, err)
	}

	if len(hashSlice) != HashSize {
		err := fmt.Errorf("invalid machine root hash length: expected 32 bytes, got %d bytes", len(hashSlice))
		return hash, errors.Join(ErrMachineInternal, err)
	}

	copy(hash[:], hashSlice)
	return hash, nil
}

// OutputsHash returns the outputs hash stored in the cmio tx buffer
func (m *machineImpl) OutputsHash(ctx context.Context) (Hash, error) {
	accepted, data, err := m.wasLastRequestAccepted(ctx)
	if err != nil {
		err = fmt.Errorf("could not read the outputs hash: %w", err)
		return Hash{}, err
	}

	if !accepted {
		err = fmt.Errorf("could not read the outputs hash: machine manual yield reason is not accepted")
		return Hash{}, err
	}

	if length := len(data); length != HashSize {
		err = fmt.Errorf("invalid outputs hash: %w (it has %d bytes)", ErrHashLength, length)
		return Hash{}, err
	}

	var outputsHash Hash
	copy(outputsHash[:], data)
	return outputsHash, nil
}

// Advance sends an input to the machine and processes it
func (m *machineImpl) Advance(ctx context.Context, input []byte) (bool, []Output, []Report, Hash, error) {
	// TODO: return the exception reason
	accepted, outputs, reports, data, err := m.process(ctx, input, AdvanceStateRequest)
	if err != nil {
		return accepted, outputs, reports, Hash{}, err
	}

	if length := len(data); length != HashSize {
		err = fmt.Errorf("%w (it has %d bytes)", ErrHashLength, length)
		return accepted, outputs, reports, Hash{}, err
	}

	var outputsHash Hash
	copy(outputsHash[:], data)
	return accepted, outputs, reports, outputsHash, nil
}

// Inspect sends a query to the machine and returns the results
func (m *machineImpl) Inspect(ctx context.Context, query []byte) (bool, []Report, error) {
	// TODO: return the exception reason
	accepted, _, reports, _, err := m.process(ctx, query, InspectStateRequest)
	return accepted, reports, err
}

// Store saves the machine state to the specified path
func (m *machineImpl) Store(ctx context.Context, path string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	err := m.backend.Store(path, m.params.StoreDeadline)
	if err != nil {
		err = fmt.Errorf("could not store the machine state: %w", err)
		return errors.Join(ErrMachineInternal, err)
	}

	return nil
}

// Close shuts down the machine and its server
func (m *machineImpl) Close() error {
	if m.backend == nil {
		return nil
	}

	err := m.backend.ShutdownServer(m.params.FastDeadline)
	if err != nil {
		err = fmt.Errorf("could not shut down the server: %w", err)
		err = errors.Join(errors.Join(ErrMachineInternal, err),
			fmt.Errorf("%w at address %s", ErrOrphanServer, m.address))
	}
	m.backend.Delete()
	m.backend = nil
	return err
}

// Address returns the address of the machine server
func (m *machineImpl) Address() string {
	return m.address
}

// Helper methods

// isAtManualYield checks if the machine is at a manual yield point
func (m *machineImpl) isAtManualYield(ctx context.Context) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	isAtManualYield, err := m.backend.IsAtManualYield(m.params.FastDeadline)
	if err != nil {
		err = fmt.Errorf("could not read iflagsY: %w", err)
		return false, errors.Join(ErrMachineInternal, err)
	}
	return isAtManualYield, nil
}

// wasLastRequestAccepted returns true if the last request was accepted and false otherwise.
// It returns the ErrException error if the last request yielded an exception.
//
// The machine MUST be at a manual yield when calling this function.
func (m *machineImpl) wasLastRequestAccepted(ctx context.Context) (bool, []byte, error) {
	if err := checkContext(ctx); err != nil {
		return false, nil, err
	}

	_, yieldReason, data, err := m.backend.ReceiveCmioRequest(m.params.FastDeadline)
	if err != nil {
		return false, nil, err
	}

	switch manualYieldReason(yieldReason) {
	case ManualYieldReasonAccepted:
		return true, data, nil
	case ManualYieldReasonRejected:
		return false, data, nil
	case ManualYieldReasonException:
		return false, data, ErrException
	default:
		panic("unreachable code: invalid manual yield reason")
	}
}

// readCycle reads the current cycle from the machine
func (m *machineImpl) readMCycle(ctx context.Context) (uint64, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}

	mcycle, err := m.backend.ReadMCycle(m.params.FastDeadline)
	if err != nil {
		err = fmt.Errorf("could not read the machine's current cycle: %w", err)
		return mcycle, errors.Join(ErrMachineInternal, err)
	}
	return mcycle, nil
}

// process processes a request, be it an advance-state or an inspect-state request.
// It returns the accepted state and any collected responses.
//
// It expects the machine to be ready to receive requests before execution,
// and leaves the machine in a state ready to receive requests after an execution with no errors.
func (m *machineImpl) process(
	ctx context.Context,
	request []byte,
	reqType requestType,
) (bool, []Output, []Report, []byte, error) {
	if err := checkContext(ctx); err != nil {
		return false, nil, nil, nil, err
	}
	// Check payload length limit
	if length := uint64(len(request)); length > m.backend.CmioRxBufferSize() {
		return false, nil, nil, nil, ErrPayloadLengthLimitExceeded
	}

	err := m.backend.SendCmioResponse(uint16(reqType), request, m.params.FastDeadline)
	if err != nil {
		return false, nil, nil, nil, err
	}

	outputs, reports, err := m.run(ctx, reqType)
	if err != nil {
		return false, outputs, reports, nil, err
	}

	accepted, data, err := m.wasLastRequestAccepted(ctx)

	return accepted, outputs, reports, data, err
}

// run runs the machine until it manually yields.
// It returns any collected responses.
func (m *machineImpl) run(ctx context.Context, reqType requestType) ([]Output, []Report, error) {
	startTime := time.Now()

	currentCycle, err := m.readMCycle(ctx)
	if err != nil {
		return nil, nil, err
	}

	limitCycle := currentCycle + m.params.AdvanceMaxCycles
	m.logger.Debug("run",
		"startingCycle", currentCycle,
		"limitCycle", limitCycle,
		"leftover", limitCycle-currentCycle)

	outputs := []Output{}
	reports := []Report{}

	stepTimeout := m.params.AdvanceIncDeadline
	runTimeout := m.params.AdvanceMaxDeadline
	if reqType == InspectStateRequest {
		stepTimeout = m.params.InspectIncDeadline
		runTimeout = m.params.InspectMaxDeadline
	}

	for {
		var yt *yieldType
		var err error

		// Steps the machine as many times as needed until it manually/automatically yields.
		for yt == nil {
			if time.Since(startTime) > runTimeout {
				return outputs, reports, fmt.Errorf("run operation timed out: %w", ErrDeadlineExceeded)
			}
			yt, currentCycle, err = m.step(ctx, currentCycle, limitCycle, stepTimeout)
			if err != nil {
				return outputs, reports, err
			}
		}

		// Returns with the responses when the machine manually yields.
		if *yt == ManualYield {
			return outputs, reports, nil
		}

		// Asserts the machine yielded automatically.
		if *yt != AutomaticYield {
			panic("unreachable code: invalid yield type")
		}
		yt = nil

		_, yieldReason, data, err := m.backend.ReceiveCmioRequest(m.params.FastDeadline)
		if err != nil {
			return outputs, reports, fmt.Errorf("could not read output/report: %w", err)
		}

		switch automaticYieldReason(yieldReason) {
		case AutomaticYieldReasonProgress:
			m.logger.Debug("ignoring yield reason progress", "value", fmt.Sprintf("%v", data))
		case AutomaticYieldReasonOutput:
			// TODO: should we remove this?
			if len(outputs) == maxOutputs {
				return outputs, reports, ErrOutputsLimitExceeded
			}
			outputs = append(outputs, data)
		case AutomaticYieldReasonReport:
			reports = append(reports, data)
		default:
			panic("unreachable code: invalid automatic yield reason")
		}
	}
}

// step runs the machine for at most machine.inc cycles (or the amount of cycles left to reach
// limitCycle, whichever is the lowest).
// It returns the yield type and the machine cycle after the step.
// If the machine did not manually/automatically yield, the yield type will be nil (meaning step
// must be called again to complete the computation).
func (m *machineImpl) step(ctx context.Context,
	currentCycle Cycle,
	limitCycle Cycle,
	timeout time.Duration,
) (*yieldType, Cycle, error) {
	startingCycle := currentCycle

	// Returns with an error if the next run would exceed limitCycle.
	if currentCycle >= limitCycle && m.params.AdvanceIncCycles != 0 {
		return nil, 0, ErrReachedTargetMcycle
	}

	// Calculates the increment.
	increment := min(m.params.AdvanceIncCycles, limitCycle-currentCycle)

	m.logger.Debug("machine step before run", "currentCycle", currentCycle, "increment", increment)

	// Runs the machine.
	breakReason, err := m.backend.Run(currentCycle+increment, timeout)
	if err != nil {
		return nil, 0, err
	}

	// Gets the current cycle.
	currentCycle, err = m.readMCycle(ctx)
	if err != nil {
		return nil, 0, err
	}

	m.logger.Debug("machine step after run",
		"startingCycle", startingCycle,
		"increment", increment,
		"currentCycle", currentCycle,
		"leftover", limitCycle-currentCycle,
		"breakReason", breakReason)

	switch breakReason {
	case YieldedManually:
		yt := ManualYield
		return &yt, currentCycle, nil // returns with the yield type
	case YieldedAutomatically:
		yt := AutomaticYield
		return &yt, currentCycle, nil // returns with the yield type
	case YieldedSoftly:
		return nil, currentCycle, nil // returns with no yield type
	case ReachedTargetMcycle:
		return nil, currentCycle, ErrReachedTargetMcycle
	case Halted:
		return nil, currentCycle, ErrHalted
	case Failed:
		fallthrough // covered by backend.Run() err
	default:
		panic("unreachable code: invalid break reason")
	}
}

// Helper functions

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrDeadlineExceeded
	} else if errors.Is(err, context.Canceled) {
		return ErrCanceled
	} else {
		return err
	}
}
