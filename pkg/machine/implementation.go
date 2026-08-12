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

type runResult struct {
	outputs             []Output
	reports             []Report
	periodicStateHashes []Hash
	paddingRepetitions  uint64
}

type processResult struct {
	runResult
	completion completionResult
}

type incrementResult struct {
	breakReason  BreakReason
	currentCycle Cycle
}

type completionResult struct {
	status CompletionStatus
	data   []byte
}

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
	result, err := m.readManualYieldResult(ctx)
	if err != nil {
		err = fmt.Errorf("could not read the outputs hash: %w", err)
		return Hash{}, err
	}

	switch result.status {
	case CompletionStatusAccepted:
		// Intentionally empty.
	case CompletionStatusRejected:
		return Hash{}, fmt.Errorf("could not read the outputs hash: %w", ErrRejected)
	case CompletionStatusException:
		return Hash{}, fmt.Errorf("could not read the outputs hash: %w", ErrException)
	case CompletionStatusHalted:
		return Hash{}, fmt.Errorf("could not read the outputs hash: %w", ErrHalted)
	case CompletionStatusUnknown:
		return Hash{}, fmt.Errorf(
			"could not read the outputs hash with completion status %d: %w",
			result.status,
			ErrMachineInternal,
		)
	}

	if length := len(result.data); length != HashSize {
		err = fmt.Errorf("invalid outputs hash: %w (it has %d bytes)", ErrHashLength, length)
		return Hash{}, err
	}

	var outputsHash Hash
	copy(outputsHash[:], result.data)
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

// Advance sends an input to the machine and processes it
func (m *machineImpl) Advance(ctx context.Context, input []byte, checkpointHash Hash, computeHashes bool) (*AdvanceResponse, error) {
	result, err := m.process(ctx, input, AdvanceStateRequest, &checkpointHash, computeHashes)
	if err != nil {
		return nil, err
	}
	if !result.completion.status.IsCompleted() {
		return nil, fmt.Errorf(
			"invalid completed advance status %d: %w",
			result.completion.status,
			ErrMachineInternal,
		)
	}

	resp := &AdvanceResponse{
		Status:          result.completion.status,
		Outputs:         result.outputs,
		Reports:         result.reports,
		Hashes:          result.periodicStateHashes,
		RemainingCycles: result.paddingRepetitions,
	}

	if resp.Status == CompletionStatusAccepted {
		if length := len(result.completion.data); length != HashSize {
			return nil, fmt.Errorf("%w (it has %d bytes)", ErrHashLength, length)
		}
		copy(resp.OutputsHash[:], result.completion.data)
	} else if resp.Status == CompletionStatusException {
		resp.ExceptionData = append([]byte{}, result.completion.data...)
	}
	return resp, nil
}

// Inspect sends a query to the machine and returns the results.
func (m *machineImpl) Inspect(ctx context.Context, query []byte) (*InspectResponse, error) {
	// For inspect-state requests, revert_root_hash is not checked and can be NULL/empty
	result, err := m.process(ctx, query, InspectStateRequest, nil, false)
	if err != nil {
		return nil, err
	}
	if !result.completion.status.IsCompleted() {
		return nil, fmt.Errorf(
			"invalid completed inspect status %d: %w",
			result.completion.status,
			ErrMachineInternal,
		)
	}
	response := &InspectResponse{
		Status:  result.completion.status,
		Reports: result.reports,
	}
	if response.Status == CompletionStatusException {
		response.ExceptionData = append([]byte{}, result.completion.data...)
	}
	return response, nil
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

// readManualYieldResult returns the typed completion status and data reported by
// the current manual yield. The machine MUST be at a manual yield when called.
func (m *machineImpl) readManualYieldResult(ctx context.Context) (completionResult, error) {
	if err := checkContext(ctx); err != nil {
		return completionResult{}, err
	}

	_, yieldReason, data, err := m.backend.ReceiveCmioRequest(m.params.FastDeadline)
	if err != nil {
		return completionResult{}, err
	}

	switch manualYieldReason(yieldReason) {
	case ManualYieldReasonAccepted:
		return completionResult{status: CompletionStatusAccepted, data: data}, nil
	case ManualYieldReasonRejected:
		return completionResult{status: CompletionStatusRejected, data: data}, nil
	case ManualYieldReasonException:
		return completionResult{status: CompletionStatusException, data: data}, nil
	default:
		err = fmt.Errorf("invalid manual yield reason: %d: %w", yieldReason, ErrMachineInternal)
		return completionResult{}, err
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
// checkpointHash is nil for inspects.
func (m *machineImpl) process(
	ctx context.Context,
	request []byte,
	reqType requestType,
	checkpointHash *Hash,
	computeHashes bool,
) (processResult, error) {
	if err := checkContext(ctx); err != nil {
		return processResult{}, err
	}
	// Check payload length limit
	if length := uint64(len(request)); length > m.backend.CmioRxBufferSize() {
		return processResult{}, ErrPayloadLengthLimitExceeded
	}

	currentCycle, err := m.readMCycle(ctx)
	if err != nil {
		return processResult{}, err
	}
	limitCycle := currentCycle + m.params.AdvanceMaxCycles
	if reqType == InspectStateRequest {
		limitCycle = currentCycle + m.params.InspectMaxCycles
	}

	err = m.backend.SendCmioResponse(uint16(reqType), request, checkpointHash, m.params.FastDeadline)
	if err != nil {
		return processResult{}, err
	}

	execution, err := m.run(ctx, reqType, computeHashes, currentCycle, limitCycle)
	result := processResult{runResult: execution}
	switch {
	case err == nil:
		manualResult, err := m.readManualYieldResult(ctx)
		result.completion = manualResult
		return result, err
	case errors.Is(err, ErrMachineInternal):
		return result, err
	case errors.Is(err, ErrHalted):
		result.completion.status = CompletionStatusHalted
		return result, nil
	default:
		return result, err
	}
}

// run executes a request between explicit cycle bounds and returns any
// responses collected before it reaches a fixed point.
func (m *machineImpl) run(
	ctx context.Context,
	reqType requestType,
	computeHashes bool,
	currentCycle uint64,
	limitCycle uint64,
) (runResult, error) {
	startTime := time.Now()

	stepTimeout := m.params.AdvanceIncDeadline
	runTimeout := m.params.AdvanceMaxDeadline
	increment := m.params.AdvanceIncCycles
	if reqType == InspectStateRequest {
		stepTimeout = m.params.InspectIncDeadline
		runTimeout = m.params.InspectMaxDeadline
		increment = m.params.InspectIncCycles
	}

	m.logger.Debug("run",
		"startingCycle", currentCycle,
		"limitCycle", limitCycle,
		"leftover", limitCycle-currentCycle)

	result := runResult{
		outputs: make([]Output, 0, 16), //nolint:mnd
		reports: make([]Report, 0, 16), //nolint:mnd
	}

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
	finish := func(runErr error) (runResult, error) {
		if hashCollectorState != nil {
			result.periodicStateHashes = hashCollectorState.Hashes
			result.paddingRepetitions = StrideCountInInput - uint64(len(hashCollectorState.Hashes))
		}
		return result, runErr
	}

	for {
		if err := checkContext(ctx); err != nil {
			return finish(err)
		}
		if time.Since(startTime) > runTimeout {
			return finish(fmt.Errorf("run operation timed out: %w", ErrDeadlineExceeded))
		}

		interval, err := m.runIncrementInterval(
			ctx, currentCycle, limitCycle, increment, hashCollectorState, stepTimeout,
		)
		currentCycle = interval.currentCycle
		if err != nil {
			return finish(err)
		}

		switch interval.breakReason {
		case YieldedManually:
			return finish(nil)
		case Halted:
			return finish(ErrHalted)
		case McycleOverflow:
			return finish(ErrReachedLimitMcycle)
		case ReachedTargetMcycle, YieldedSoftly:
			continue
		case Failed:
			return finish(ErrMachineInternal)
		case YieldedAutomatically:
		default:
			return finish(fmt.Errorf("invalid break reason: %d: %w", interval.breakReason, ErrMachineInternal))
		}

		if err := checkContext(ctx); err != nil {
			return finish(err)
		}

		_, yieldReason, data, err := m.backend.ReceiveCmioRequest(m.params.FastDeadline)
		if err != nil {
			werr := fmt.Errorf("could not read output/report: %w", err)
			return finish(werr)
		}

		switch automaticYieldReason(yieldReason) {
		case AutomaticYieldReasonProgress:
			m.logger.Debug("ignoring yield reason progress", "value", fmt.Sprintf("%v", data))
		case AutomaticYieldReasonOutput:
			// TODO: should we remove this?
			if len(result.outputs) == maxOutputs {
				return finish(ErrOutputsLimitExceeded)
			}
			result.outputs = append(result.outputs, data)
		case AutomaticYieldReasonReport:
			if len(result.reports) == maxReports {
				return finish(ErrReportsLimitExceeded)
			}
			result.reports = append(result.reports, data)
		default:
			err := fmt.Errorf("invalid automatic yield reason: %d: %w", yieldReason, ErrMachineInternal)
			return finish(err)
		}
	}
}

// runIncrementInterval runs the machine for at most incrementLimit mcycles and
// preserves the emulator's typed break reason for the request loop.
func (m *machineImpl) runIncrementInterval(ctx context.Context,
	currentCycle Cycle,
	limitCycle Cycle,
	incrementLimit Cycle,
	hashCollectorState *HashCollectorState,
	timeout time.Duration,
) (incrementResult, error) {
	startingCycle := currentCycle

	if currentCycle >= limitCycle {
		return incrementResult{currentCycle: currentCycle}, ErrReachedLimitMcycle
	}

	increment := min(incrementLimit, limitCycle-currentCycle)

	m.logger.Debug("machine step before run", "currentCycle", currentCycle, "increment", increment)

	// Runs the machine.
	breakReason, err := m.backendRun(currentCycle+increment, hashCollectorState, timeout)
	if err != nil {
		return incrementResult{currentCycle: currentCycle}, err
	}

	// Gets the current cycle.
	currentCycle, err = m.readMCycle(ctx)
	if err != nil {
		return incrementResult{}, err
	}

	m.logger.Debug("machine step after run",
		"startingCycle", startingCycle,
		"increment", increment,
		"currentCycle", currentCycle,
		"leftover", limitCycle-currentCycle,
		"breakReason", breakReason)

	switch breakReason {
	case YieldedManually,
		YieldedAutomatically,
		YieldedSoftly,
		ReachedTargetMcycle,
		McycleOverflow,
		Halted,
		Failed:
		return incrementResult{breakReason: breakReason, currentCycle: currentCycle}, nil
	default:
		err := fmt.Errorf("invalid break reason: %d: %w", breakReason, ErrMachineInternal)
		return incrementResult{breakReason: breakReason, currentCycle: currentCycle}, err
	}
}

func (m *machineImpl) backendRun(mcycleEnd uint64, hashCollectorState *HashCollectorState, timeout time.Duration) (BreakReason, error) {
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
