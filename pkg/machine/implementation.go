// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"
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

// Limits for outputs and reports per input
const maxOutputs = 65536 // 2^16
const maxReports = 65536 // 2^16

const CheckpointAddress uint64 = 0xfe0 // value from machine api: CM_AR_SHADOW_REVERT_ROOT_HASH_START
const TxBufferAddress uint64 = 0x60800000
const HashLog2Size = 5 // 32 bytes

const (
	// log2 value of the maximal number of micro instructions that emulates a big instruction
	Log2UarchSpanToBarch uint64 = 20
	// log2 value of the maximal number of big instructions that executes an input
	Log2BarchSpanToInput uint64 = 48
	// log2 value of the maximal number of inputs that allowed in an epoch
	Log2InputSpanToEpoch uint64 = 24
	// gap of each leaf in the commitment tree, should use the same value as ArbitrationConstants.sol:log2step(0)
	Log2Stride uint64 = 44
	// log2 value of the maximal number of micro instructions that executes an input
	Log2UarchSpanToInput uint64 = Log2BarchSpanToInput + Log2UarchSpanToBarch // 68

	UarchSpanToBarch uint64 = (1 << Log2UarchSpanToBarch) - 1 // 1_048_575
	BarchSpanToInput uint64 = (1 << Log2BarchSpanToInput) - 1 // 281_474_976_710_655
	InputSpanToEpoch uint64 = (1 << Log2InputSpanToEpoch) - 1 // 16_777_215

	BigStepsInStride   uint64 = 1 << (Log2Stride - Log2UarchSpanToBarch)                        // 16_777_216
	StrideCountInInput uint64 = 1 << (Log2BarchSpanToInput + Log2UarchSpanToBarch - Log2Stride) // 16_777_216

	StrideCountInEpoch uint64 = 1 << (Log2InputSpanToEpoch + Log2BarchSpanToInput + Log2UarchSpanToBarch - Log2Stride)

	Log2StridesPerInput uint64 = Log2BarchSpanToInput + Log2UarchSpanToBarch - Log2Stride

	InputsPerEpoch uint64 = 1 << Log2InputSpanToEpoch
)

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
	newServer, address, pid, err := m.backend.ForkServer(m.params.FastDeadline)
	if err != nil {
		err = fmt.Errorf("could not fork the machine: %w", err)
		return nil, errors.Join(ErrMachineInternal, err)
	}

	// Create a new machine with the forked server
	newMachine := &machineImpl{
		backend: newServer,
		address: address,
		pid:     pid,
		params:  m.params,
		logger:  m.logger,
	}

	return newMachine, nil
}

// Hash returns the machine's merkle tree root hash
func (m *machineImpl) Hash(ctx context.Context) (Hash, error) {
	if err := checkContext(ctx); err != nil {
		return Hash{}, err
	}

	hash, err := m.backend.GetRootHash(m.params.LoadDeadline)
	if err != nil {
		err := fmt.Errorf("could not get the machine's root hash: %w", err)
		return hash, errors.Join(ErrMachineInternal, err)
	}

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

func (m *machineImpl) OutputsHashProof(ctx context.Context) ([]Hash, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	siblings, err := m.backend.GetProof(TxBufferAddress, HashLog2Size, m.params.LoadDeadline)
	if err != nil {
		err := fmt.Errorf("could not get outputs hash machine proof: %w", err)
		return nil, errors.Join(ErrMachineInternal, err)
	}
	return siblings, nil
}

func (m *machineImpl) WriteCheckpointHash(ctx context.Context, hash Hash) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	err := m.backend.WriteMemory(CheckpointAddress, hash[:], m.params.FastDeadline)
	if err != nil {
		err := fmt.Errorf("could not write checkpoint hash in to machine memory: %w", err)
		return errors.Join(ErrMachineInternal, err)
	}
	return nil
}

// Advance sends an input to the machine and processes it
func (m *machineImpl) Advance(ctx context.Context, input []byte, computeHashes bool) (*AdvanceResponse, error) {
	// TODO: return the exception reason
	accepted, outputs, reports, hashes, remaining, data, err := m.process(ctx, input, AdvanceStateRequest, computeHashes)
	if err != nil {
		return &AdvanceResponse{
			Accepted:        accepted,
			Outputs:         outputs,
			Reports:         reports,
			Hashes:          hashes,
			RemainingCycles: remaining,
		}, err
	}

	resp := &AdvanceResponse{
		Accepted:        accepted,
		Outputs:         outputs,
		Reports:         reports,
		Hashes:          hashes,
		RemainingCycles: remaining,
	}

	if accepted {
		if length := len(data); length != HashSize {
			return resp, fmt.Errorf("%w (it has %d bytes)", ErrHashLength, length)
		}
		copy(resp.OutputsHash[:], data)
	}
	return resp, nil
}

// Inspect sends a query to the machine and returns the results
func (m *machineImpl) Inspect(ctx context.Context, query []byte) (bool, []Report, error) {
	// TODO: return the exception reason
	accepted, _, reports, _, _, _, err := m.process(ctx, query, InspectStateRequest, false)
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
		// ShutdownServer can fail because SIGINT was delivered to the
		// entire process group: the child is already shutting down (closing
		// sockets) so our RPC gets "connection reset" or "end of stream".
		// Wait briefly for the child to finish exiting before reporting it
		// as orphaned.
		if m.pid != 0 && waitForExit(m.pid, 500*time.Millisecond) { //nolint: mnd
			m.backend.Delete()
			m.backend = nil
			return nil
		}
		err = fmt.Errorf("could not shut down the server: %w", err)
		err = errors.Join(errors.Join(ErrMachineInternal, err),
			fmt.Errorf("%w at address %s", ErrOrphanServer, m.address))
	}
	m.backend.Delete()
	m.backend = nil
	return err
}

// waitForExit polls until the process with the given PID has exited or the
// timeout elapses. Returns true if the process exited within the timeout.
func waitForExit(pid uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		proc, err := os.FindProcess(int(pid))
		if err != nil {
			return true
		}
		// try send no-op signal 0 to check process is still receiving signals.
		if proc.Signal(syscall.Signal(0)) != nil {
			return true
		}
		time.Sleep(50 * time.Millisecond) //nolint: mnd
	}
	return false
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
		err = fmt.Errorf("invalid manual yield reason: %d: %w", yieldReason, ErrMachineInternal)
		return false, nil, err
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
	computeHashes bool,
) (bool, []Output, []Report, []Hash, uint64, []byte, error) {
	if err := checkContext(ctx); err != nil {
		return false, nil, nil, nil, 0, nil, err
	}
	// Check payload length limit
	if length := uint64(len(request)); length > m.backend.CmioRxBufferSize() {
		return false, nil, nil, nil, 0, nil, ErrPayloadLengthLimitExceeded
	}

	err := m.backend.SendCmioResponse(uint16(reqType), request, m.params.FastDeadline)
	if err != nil {
		return false, nil, nil, nil, 0, nil, err
	}

	outputs, reports, hashes, remaining, err := m.run(ctx, reqType, computeHashes)
	if err != nil {
		return false, outputs, reports, nil, 0, nil, err
	}

	accepted, data, err := m.wasLastRequestAccepted(ctx)

	return accepted, outputs, reports, hashes, remaining, data, err
}

// run runs the machine until it manually yields.
// It returns any collected responses.
func (m *machineImpl) run(ctx context.Context, reqType requestType, computeHashes bool) ([]Output, []Report, []Hash, uint64, error) {
	startTime := time.Now()

	currentCycle, err := m.readMCycle(ctx)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	limitCycle := currentCycle + m.params.AdvanceMaxCycles
	stepTimeout := m.params.AdvanceIncDeadline
	runTimeout := m.params.AdvanceMaxDeadline
	if reqType == InspectStateRequest {
		limitCycle = currentCycle + m.params.InspectMaxCycles
		stepTimeout = m.params.InspectIncDeadline
		runTimeout = m.params.InspectMaxDeadline
	}

	m.logger.Debug("run",
		"startingCycle", currentCycle,
		"limitCycle", limitCycle,
		"leftover", limitCycle-currentCycle)

	outputs := make([]Output, 0, 16) //nolint:mnd
	reports := make([]Report, 0, 16) //nolint:mnd

	var hashCollectorState *HashCollectorState
	if computeHashes {
		hashCollectorState = &HashCollectorState{
			Period:     BigStepsInStride,
			Phase:      0,
			MaxHashes:  0,
			BundleLog2: 0,
			Hashes:     []Hash{},
		}
	}
	hashes := func() []Hash {
		if computeHashes {
			return hashCollectorState.Hashes
		}
		return []Hash{}
	}
	remainingMetaCycles := func() uint64 {
		if computeHashes {
			return StrideCountInInput - uint64(len(hashCollectorState.Hashes))
		}
		return 0
	}

	for {
		var yt *yieldType
		var err error

		// Steps the machine as many times as needed until it manually/automatically yields.
		for yt == nil {
			if err := checkContext(ctx); err != nil {
				return outputs, reports, hashes(), remainingMetaCycles(), err
			}
			if time.Since(startTime) > runTimeout {
				werr := fmt.Errorf("run operation timed out: %w", ErrDeadlineExceeded)
				return outputs, reports, hashes(), remainingMetaCycles(), werr
			}
			yt, currentCycle, err = m.runIncrementInterval(ctx, currentCycle, limitCycle, hashCollectorState, stepTimeout)
			if err != nil && err != ErrReachedTargetMcycle {
				return outputs, reports, hashes(), remainingMetaCycles(), err
			}
		}

		// Returns with the responses when the machine manually yields.
		if *yt == ManualYield {
			return outputs, reports, hashes(), remainingMetaCycles(), nil
		}

		// Asserts the machine yielded automatically.
		if *yt != AutomaticYield {
			err := fmt.Errorf("invalid yield type: %d: %w", *yt, ErrMachineInternal)
			return outputs, reports, hashes(), remainingMetaCycles(), err
		}
		yt = nil

		if err := checkContext(ctx); err != nil {
			return outputs, reports, hashes(), remainingMetaCycles(), err
		}

		_, yieldReason, data, err := m.backend.ReceiveCmioRequest(m.params.FastDeadline)
		if err != nil {
			werr := fmt.Errorf("could not read output/report: %w", err)
			return outputs, reports, hashes(), remainingMetaCycles(), werr
		}

		switch automaticYieldReason(yieldReason) {
		case AutomaticYieldReasonProgress:
			m.logger.Debug("ignoring yield reason progress", "value", fmt.Sprintf("%v", data))
		case AutomaticYieldReasonOutput:
			// TODO: should we remove this?
			if len(outputs) == maxOutputs {
				return outputs, reports, hashes(), remainingMetaCycles(), ErrOutputsLimitExceeded
			}
			outputs = append(outputs, data)
		case AutomaticYieldReasonReport:
			if len(reports) == maxReports {
				return outputs, reports, hashes(), remainingMetaCycles(), ErrReportsLimitExceeded
			}
			reports = append(reports, data)
		default:
			err := fmt.Errorf("invalid automatic yield reason: %d: %w", yieldReason, ErrMachineInternal)
			return outputs, reports, hashes(), remainingMetaCycles(), err
		}
	}
}

// runIncrementInterval runs the machine for at most machine.inc cycles (or the amount of cycles left to reach
// limitCycle, whichever is the lowest).
// It returns the yield type and the machine cycle after the increment interval.
// If the machine did not manually/automatically yield, the yield type will be nil (meaning runIncrementInterval
// must be called again to complete the computation).
func (m *machineImpl) runIncrementInterval(ctx context.Context,
	currentCycle Cycle,
	limitCycle Cycle,
	hashCollectorState *HashCollectorState,
	timeout time.Duration,
) (*yieldType, Cycle, error) {
	startingCycle := currentCycle

	// Returns with an error if the next run would exceed limitCycle.
	if currentCycle >= limitCycle && m.params.AdvanceIncCycles != 0 {
		return nil, 0, ErrReachedLimitMcycle
	}

	// Calculates the increment.
	increment := min(m.params.AdvanceIncCycles, limitCycle-currentCycle)

	m.logger.Debug("machine step before run", "currentCycle", currentCycle, "increment", increment)

	// Runs the machine.
	breakReason, err := m.backend_run(currentCycle+increment, hashCollectorState, timeout)
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
		return nil, currentCycle, ErrMachineInternal
	default:
		err := fmt.Errorf("invalid break reason: %d: %w", breakReason, ErrMachineInternal)
		return nil, currentCycle, err
	}
}

func (m *machineImpl) backend_run(mcycleEnd uint64, hashCollectorState *HashCollectorState, timeout time.Duration) (BreakReason, error) {
	if hashCollectorState != nil {
		m.logger.Debug("Running with root hash collection")
		return m.backend.RunAndCollectRootHashes(mcycleEnd, hashCollectorState, timeout)
	}
	return m.backend.Run(mcycleEnd, timeout)
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
