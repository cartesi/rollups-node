// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/crypto"
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

// executionBounds is the resolved cycle window for one request. A configured
// span of zero is converted to the fixed machine window exactly once here;
// downstream execution and diagnostics consume the same resolved values.
type executionBounds struct {
	start      Cycle
	limit      Cycle
	span       Cycle
	configured bool
}

func (r requestType) String() string {
	switch r {
	case AdvanceStateRequest:
		return "advance"
	case InspectStateRequest:
		return "inspect"
	default:
		return fmt.Sprintf("request_type_%d", r)
	}
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

// maxOutputs and maxReports are local operational resource ceilings. They
// bound host memory consumption; exceeding them is an incomplete execution
// failure, never a protocol-level input completion status.
const maxOutputs = 65536 // 2^16
const maxReports = 65536 // 2^16

const (
	// These addresses and hash-tree sizes are defined by the Cartesi Machine
	// emulator.
	iflagsYAddress          uint64 = 0x308
	htifTohostAddress       uint64 = 0x330
	TxBufferAddress         uint64 = 0x60800000
	HashLog2Size                   = 5 // 32-byte data block
	machineMemoryLog2Size   int32  = 64
	memoryProofSiblingCount        = int(machineMemoryLog2Size) - HashLog2Size
)

const (
	// These values encode the HTIF fields proved by the accepted-state check.
	htifDeviceYield         uint64 = 2
	htifCommandManual       uint64 = 1
	htifReasonInputAccepted uint64 = 1
	htifDeviceShift                = 56
	htifCommandShift               = 48
	htifReasonShift                = 32
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

func (m *machineImpl) StateProof(ctx context.Context) (*StateProof, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	machineHash, err := m.backend.GetRootHash(m.params.LoadDeadline)
	if err != nil {
		return nil, errors.Join(
			ErrMachineInternal,
			fmt.Errorf("could not get the machine root for its validity proof: %w", err),
		)
	}

	iflagsYProof, err := m.readLeafProof(ctx, machineHash, iflagsYAddress)
	if err != nil {
		return nil, fmt.Errorf("could not prove iflags_Y: %w", err)
	}
	htifTohostProof, err := m.readLeafProof(ctx, machineHash, htifTohostAddress)
	if err != nil {
		return nil, fmt.Errorf("could not prove HTIF tohost: %w", err)
	}
	txBufferProof, err := m.readLeafProof(ctx, machineHash, TxBufferAddress)
	if err != nil {
		return nil, fmt.Errorf("could not prove the CMIO TX buffer: %w", err)
	}

	return &StateProof{
		MachineHash:     machineHash,
		IflagsYProof:    iflagsYProof,
		HtifTohostProof: htifTohostProof,
		TxBufferProof:   txBufferProof,
	}, nil
}

// ValidateAcceptedState checks the state semantics required when an epoch
// is published through the released v3 contracts. StateProof itself remains
// generic so the exact post-run proof can also be persisted for terminal
// outcomes.
func ValidateAcceptedState(proof *StateProof) error {
	if proof == nil {
		return fmt.Errorf("state proof is nil: %w", ErrInvalidMachineProof)
	}
	if readMachineWord(proof.IflagsYProof.DataBlock, iflagsYAddress) == 0 {
		return fmt.Errorf("iflags_Y is zero: %w", ErrInvalidMachineProof)
	}
	tohost := readMachineWord(proof.HtifTohostProof.DataBlock, htifTohostAddress)
	if !isAcceptedManualYield(tohost) {
		return fmt.Errorf("HTIF tohost does not signal an accepted manual yield: %w", ErrInvalidMachineProof)
	}
	return nil
}

func (m *machineImpl) readLeafProof(ctx context.Context, machineHash Hash, wordAddress uint64) (LeafProof, error) {
	if err := checkContext(ctx); err != nil {
		return LeafProof{}, err
	}
	dataBlockAddress := wordAddress &^ ((uint64(1) << HashLog2Size) - 1)
	proof, err := m.backend.GetProof(
		dataBlockAddress,
		int32(HashLog2Size),
		machineMemoryLog2Size,
		m.params.LoadDeadline,
	)
	if err != nil {
		return LeafProof{}, errors.Join(ErrMachineInternal, fmt.Errorf("could not get memory proof: %w", err))
	}
	data, err := m.backend.ReadMemory(
		dataBlockAddress,
		uint64(1)<<HashLog2Size,
		m.params.LoadDeadline,
	)
	if err != nil {
		return LeafProof{}, errors.Join(ErrMachineInternal, fmt.Errorf("could not read proof data block: %w", err))
	}
	leafProof, err := verifyMemoryProof(machineHash, dataBlockAddress, proof, data)
	if err != nil {
		return LeafProof{}, errors.Join(ErrMachineInternal, err)
	}
	return leafProof, nil
}

func verifyMemoryProof(machineHash Hash, dataBlockAddress uint64, proof MemoryProof, data []byte) (LeafProof, error) {
	if proof.Log2RootSize != machineMemoryLog2Size {
		return LeafProof{}, fmt.Errorf(
			"root log2 size is %d, expected %d: %w",
			proof.Log2RootSize, machineMemoryLog2Size, ErrInvalidMachineProof,
		)
	}
	if proof.Log2TargetSize != int32(HashLog2Size) {
		return LeafProof{}, fmt.Errorf(
			"target log2 size is %d, expected %d: %w",
			proof.Log2TargetSize, HashLog2Size, ErrInvalidMachineProof,
		)
	}
	if proof.TargetAddress != dataBlockAddress {
		return LeafProof{}, fmt.Errorf(
			"target address is %#x, expected %#x: %w",
			proof.TargetAddress, dataBlockAddress, ErrInvalidMachineProof,
		)
	}
	if proof.RootHash != machineHash {
		return LeafProof{}, fmt.Errorf("proof root does not match the current machine root: %w", ErrInvalidMachineProof)
	}
	if len(proof.Siblings) != memoryProofSiblingCount {
		return LeafProof{}, fmt.Errorf(
			"proof has %d siblings, expected %d: %w",
			len(proof.Siblings), memoryProofSiblingCount, ErrInvalidMachineProof,
		)
	}
	if len(data) != 1<<HashLog2Size {
		return LeafProof{}, fmt.Errorf("proof data block has %d bytes, expected %d: %w", len(data), 1<<HashLog2Size, ErrInvalidMachineProof)
	}

	var dataBlock Hash
	copy(dataBlock[:], data)
	targetHash := Hash(crypto.Keccak256Hash(data))
	if proof.TargetHash != targetHash {
		return LeafProof{}, fmt.Errorf("proof target hash does not match its data block: %w", ErrInvalidMachineProof)
	}

	root := targetHash
	index := dataBlockAddress >> HashLog2Size
	for _, sibling := range proof.Siblings {
		if index&1 == 0 {
			root = Hash(crypto.Keccak256Hash(root[:], sibling[:]))
		} else {
			root = Hash(crypto.Keccak256Hash(sibling[:], root[:]))
		}
		index >>= 1
	}
	if root != machineHash {
		return LeafProof{}, fmt.Errorf("proof siblings do not reconstruct the current machine root: %w", ErrInvalidMachineProof)
	}

	return LeafProof{
		DataBlock: dataBlock,
		Siblings:  append([]Hash(nil), proof.Siblings...),
	}, nil
}

func readMachineWord(dataBlock Hash, address uint64) uint64 {
	offset := int(address & ((uint64(1) << HashLog2Size) - 1))
	return binary.LittleEndian.Uint64(dataBlock[offset : offset+8])
}

func isAcceptedManualYield(tohost uint64) bool {
	return tohost>>htifDeviceShift == htifDeviceYield &&
		(tohost>>htifCommandShift)&0xff == htifCommandManual &&
		(tohost>>htifReasonShift)&0xffff == htifReasonInputAccepted
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
		Status:              result.completion.status,
		Outputs:             result.outputs,
		Reports:             result.reports,
		PeriodicStateHashes: result.periodicStateHashes,
		PaddingRepetitions:  result.paddingRepetitions,
	}

	if resp.Status == CompletionStatusAccepted {
		if length := len(result.completion.data); length != HashSize {
			return nil, fmt.Errorf("%w (it has %d bytes)", ErrHashLength, length)
		}
	}
	if resp.Status == CompletionStatusException {
		resp.ExceptionData = append([]byte{}, result.completion.data...)
	}

	return resp, nil
}

// Inspect sends a query to the machine and returns the results.
func (m *machineImpl) Inspect(ctx context.Context, query []byte) (*InspectResponse, error) {
	// For inspect-state requests, revert_root_hash is not checked and can be NULL/empty
	result, err := m.process(ctx, query, InspectStateRequest, nil, false)
	response := &InspectResponse{Reports: result.reports}
	if err != nil {
		return response, err
	}

	response.Status = result.completion.status
	if !response.Status.IsCompleted() {
		return response, fmt.Errorf(
			"invalid completed inspect status %d: %w",
			result.completion.status,
			ErrMachineInternal,
		)
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
		return completionResult{status: CompletionStatusUnexpectedYield}, nil
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
	// An L1 advance arrives as the full EvmAdvance encoding, which the canonical
	// InputBox bounds to CanonicalMachine.INPUT_MAX_SIZE = 2^16 bytes (it reverts
	// InputTooLarge above that), while the standard CMIO RX buffer holds 2^21
	// bytes. Advance overflow is therefore defensive on the supported path;
	// inspect and custom input providers still need the runtime check.
	if length, capacity := uint64(len(request)), m.backend.CmioRxBufferSize(); length > capacity {
		return processResult{}, fmt.Errorf(
			"%s request payload length %d exceeds CMIO receive buffer capacity %d: %w",
			reqType, length, capacity, ErrPayloadLengthLimitExceeded,
		)
	}

	// Validate the request-specific increment before consulting or changing the
	// machine. A zero increment would otherwise make the run loop spin forever.
	if err := m.validateExecutionCycleIncrement(reqType); err != nil {
		return processResult{}, err
	}
	// Validate the execution target before sending the request. Sending a CMIO
	// response mutates the candidate, so invalid configuration must fail before
	// that point. A valid target that exceeds uint64 saturates at MaxUint64,
	// matching the emulator's imcyclemax calculation.
	bounds, err := m.executionCycleBounds(ctx, reqType)
	if err != nil {
		return processResult{}, err
	}

	err = m.backend.SendCmioResponse(uint16(reqType), request, checkpointHash, m.params.FastDeadline)
	if err != nil {
		return processResult{}, err
	}

	execution, err := m.run(ctx, reqType, computeHashes, bounds)
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
	case errors.Is(err, ErrMcycleOverflow):
		result.completion.status = CompletionStatusOverflow
		return result, nil
	default:
		return result, err
	}
}

// run executes a request whose increment and cycle bounds were validated before
// SendCmioResponse mutated the machine. It returns collected responses and the
// canonical compressed input hash collection.
func (m *machineImpl) run(
	ctx context.Context,
	reqType requestType,
	computeHashes bool,
	bounds executionBounds,
) (runResult, error) {
	startTime := time.Now()
	currentCycle := bounds.start

	stepTimeout := m.params.AdvanceIncDeadline
	runTimeout := m.params.AdvanceMaxDeadline
	increment := m.executionCycleIncrement(reqType)
	if reqType == InspectStateRequest {
		stepTimeout = m.params.InspectIncDeadline
		runTimeout = m.params.InspectMaxDeadline
	}
	if computeHashes {
		increment = min(increment, mcycleComputationHashChunkSize)
	}

	m.logger.Debug("run",
		"startingCycle", currentCycle,
		"limitCycle", bounds.limit,
		"leftover", bounds.limit-currentCycle)

	result := runResult{
		outputs: make([]Output, 0, 16),
		reports: make([]Report, 0, 16),
	}

	var hashCollectorState *HashCollectorState
	if computeHashes {
		hashCollectorState = &HashCollectorState{
			MCycleSamplingPeriod:  MCycleComputationHashPeriod,
			MCyclePhase:           0,
			Log2BundleMCycleCount: 0,
			Hashes:                []Hash{},
		}
	}
	finish := func(runErr error, terminalHashAppended bool) (runResult, error) {
		return finalizeRunResult(result, hashCollectorState, terminalHashAppended, runErr)
	}
	// Incomplete executions may expose partial outputs and reports, but their PRT
	// collections are discarded rather than normalized into persistable evidence.
	fail := func(runErr error) (runResult, error) {
		return result, runErr
	}

	for {
		if err := checkContext(ctx); err != nil {
			return fail(err)
		}
		if time.Since(startTime) > runTimeout {
			return fail(fmt.Errorf("run operation timed out: %w", ErrDeadlineExceeded))
		}

		hashCountBeforeRun := 0
		if hashCollectorState != nil {
			hashCountBeforeRun = len(hashCollectorState.Hashes)
		}
		incrementResult, err := m.runIncrementInterval(
			ctx,
			currentCycle,
			bounds.limit,
			increment,
			hashCollectorState,
			stepTimeout,
		)
		currentCycle = incrementResult.currentCycle
		if err != nil {
			if errors.Is(err, ErrReachedLimitMcycle) {
				return fail(executionLimitError(reqType, bounds, currentCycle, err))
			}
			return fail(err)
		}

		terminalHashAppended := computeHashes && isFixedPointBreakReason(incrementResult.breakReason)
		if terminalHashAppended && len(hashCollectorState.Hashes) <= hashCountBeforeRun {
			return fail(fmt.Errorf(
				"machine stopped at fixed point %d without appending its state hash: %w",
				incrementResult.breakReason,
				ErrMachineInternal,
			))
		}

		switch incrementResult.breakReason {
		case YieldedManually:
			return finish(nil, terminalHashAppended)
		case Halted:
			return finish(ErrHalted, terminalHashAppended)
		case McycleOverflow:
			return finish(ErrMcycleOverflow, terminalHashAppended)
		case ReachedTargetMcycle, YieldedSoftly:
			continue
		case Failed:
			return fail(ErrMachineInternal)
		case YieldedAutomatically:
			// Service the CMIO request below, then continue execution.
		default:
			return fail(fmt.Errorf(
				"invalid break reason: %d: %w",
				incrementResult.breakReason,
				ErrMachineInternal,
			))
		}

		if err := checkContext(ctx); err != nil {
			return fail(err)
		}

		_, yieldReason, data, err := m.backend.ReceiveCmioRequest(m.params.FastDeadline)
		if err != nil {
			werr := fmt.Errorf("could not read output/report: %w", err)
			return fail(werr)
		}

		switch automaticYieldReason(yieldReason) {
		case AutomaticYieldReasonProgress:
			m.logger.Debug("ignoring yield reason progress", "value", fmt.Sprintf("%v", data))
		case AutomaticYieldReasonOutput:
			if len(result.outputs) == maxOutputs {
				return fail(executionResponseLimitError(
					reqType, "output", len(result.outputs)+1, maxOutputs, ErrOutputsLimitExceeded,
				))
			}
			result.outputs = append(result.outputs, data)
		case AutomaticYieldReasonReport:
			if len(result.reports) == maxReports {
				return fail(executionResponseLimitError(
					reqType, "report", len(result.reports)+1, maxReports, ErrReportsLimitExceeded,
				))
			}
			result.reports = append(result.reports, data)
		default:
			err := fmt.Errorf("invalid automatic yield reason: %d: %w", yieldReason, ErrMachineInternal)
			return fail(err)
		}
	}
}

// finalizeRunResult converts the emulator collector's fixed-point-inclusive
// result into the node's compressed representation. Keeping this at the return
// boundary leaves the execution loop concerned only with execution policy.
func finalizeRunResult(
	result runResult,
	collector *HashCollectorState,
	terminalHashAppended bool,
	runErr error,
) (runResult, error) {
	if collector == nil {
		return result, runErr
	}

	collected := collector.Hashes
	canonicalCount, paddingRepetitions, spanErr := canonicalInputHashCollectionShape(
		uint64(len(collected)),
		terminalHashAppended,
	)
	if spanErr != nil {
		return result, invalidInputHashCollectionSpanError(runErr, spanErr)
	}

	// Emulator 0.21 includes a fixed-point entry and pads the input's remaining
	// entry capacity with it. The node stores that same sequence in compressed
	// form: periodic entries are kept here, while the manager supplies the
	// completed status's canonical final root for persistence to append with
	// paddingRepetitions in place of the removed collector entry.
	result.periodicStateHashes = collected[:canonicalCount]
	result.paddingRepetitions = paddingRepetitions
	return result, runErr
}

// invalidInputHashCollectionSpanError makes collection integrity authoritative
// over a terminal execution status. The status remains useful context, but is
// deliberately not wrapped: callers must not persist a malformed collection as
// a completed exception or halt.
func invalidInputHashCollectionSpanError(runErr, spanErr error) error {
	err := errors.Join(
		ErrMachineInternal,
		fmt.Errorf("invalid collected state hash span: %w", spanErr),
	)
	if runErr != nil {
		err = fmt.Errorf("execution ended with %v and invalid hash span: %w", runErr, err)
	}
	return err
}

func executionResponseLimitError(
	reqType requestType,
	responseKind string,
	count int,
	capacity int,
	limitErr error,
) error {
	return fmt.Errorf(
		"%s %s count %d exceeds local operational capacity %d: %w",
		reqType, responseKind, count, capacity, limitErr,
	)
}

// canonicalInputHashCollectionShape validates the collector's raw count before
// optionally excluding a terminal fixed-point sample. Validating first prevents
// an overcollection of InputEntryCapacity+1 from being normalized into an
// apparently valid collection.
func canonicalInputHashCollectionShape(
	hashCount uint64,
	terminalHashAppended bool,
) (canonicalHashCount uint64, paddingRepetitions uint64, err error) {
	if _, err := inputHashCollectionPaddingRepetitions(hashCount); err != nil {
		return 0, 0, err
	}
	if terminalHashAppended {
		if hashCount == 0 {
			return 0, 0, errors.New("terminal fixed point was reported without a collected state hash")
		}
		hashCount--
	}
	paddingRepetitions, err = inputHashCollectionPaddingRepetitions(hashCount)
	if err != nil {
		return 0, 0, err
	}
	return hashCount, paddingRepetitions, nil
}

// executionCycleBounds applies the request-specific configured cycle span.
// Zero uses the machine's complete 2^48-cycle window. Every valid target uses
// the machine's saturating mcycle arithmetic: when start+span is not
// representable, MaxUint64 is both the local target and imcyclemax.
func (m *machineImpl) executionCycleBounds(
	ctx context.Context,
	reqType requestType,
) (executionBounds, error) {
	executionCycleSpan := m.configuredCycleSpan(reqType)
	if executionCycleSpan > model.MaxExecutionCycleSpan {
		return executionBounds{}, fmt.Errorf(
			"%s execution configured cycle span exceeds hard maximum: requested_span=%d maximum_span=%d: %w",
			reqType, executionCycleSpan, model.MaxExecutionCycleSpan, ErrReachedLimitMcycle,
		)
	}

	currentCycle, err := m.readMCycle(ctx)
	if err != nil {
		return executionBounds{}, err
	}

	configured := executionCycleSpan != 0
	if executionCycleSpan == 0 {
		// Emulator 0.21 arms imcyclemax = mcycle + 2^48 - 1 (saturating) at
		// input delivery and classifies reaching it as mcycle overflow with
		// priority over yield and halt. Mirror that exact window - endpoint
		// and saturation - so the node's target can never disagree with the
		// machine's own enforcement.
		executionCycleSpan = model.MaxExecutionCycleSpan
	}
	limit := ^uint64(0)
	if currentCycle <= ^uint64(0)-executionCycleSpan {
		limit = currentCycle + executionCycleSpan
	}
	return executionBounds{
		start: currentCycle, limit: limit, span: executionCycleSpan, configured: configured,
	}, nil
}

// validateExecutionCycleIncrement is a runtime defense for values loaded from
// persistent storage or supplied through a custom MachineConfig. process calls
// it before SendCmioResponse changes the candidate machine.
func (m *machineImpl) validateExecutionCycleIncrement(reqType requestType) error {
	if m.executionCycleIncrement(reqType) == 0 {
		return fmt.Errorf("%s execution increment must be greater than zero: %w", reqType, ErrMachineInternal)
	}
	return nil
}

func (m *machineImpl) configuredCycleSpan(reqType requestType) uint64 {
	if reqType == InspectStateRequest {
		return m.params.InspectMaxCycles
	}
	return m.params.AdvanceMaxCycles
}

func (m *machineImpl) executionCycleIncrement(reqType requestType) uint64 {
	if reqType == InspectStateRequest {
		return m.params.InspectIncCycles
	}
	return m.params.AdvanceIncCycles
}

// executionLimitError formats the resolved request window while preserving the
// emulator's typed imcyclemax origin when it—not a node target—stopped the run.
func executionLimitError(
	reqType requestType,
	bounds executionBounds,
	current uint64,
	origin error,
) error {
	targetSpan := bounds.limit - bounds.start
	executedCycles := uint64(0)
	if current >= bounds.start {
		executedCycles = current - bounds.start
	}
	source := "fixed"
	if bounds.configured {
		source = "configured"
	}
	progress := fmt.Sprintf(
		"start=%d requested_span=%d target_span=%d executed_cycles=%d",
		bounds.start, bounds.span, targetSpan, executedCycles,
	)

	// A machine overflow may stop execution before its local target. This most
	// commonly happens because an inspect does not re-arm imcyclemax and inherits
	// the ceiling from the preceding advance. Preserve the machine-origin
	// sentinel, but do not claim that a local span was exhausted.
	if current < bounds.limit {
		return fmt.Errorf(
			"%s execution stopped before %s cycle limit: %s: %w",
			reqType, source, progress, origin,
		)
	}
	if errors.Is(origin, ErrMcycleOverflow) && bounds.configured {
		// imcyclemax has precedence over reaching the requested run target. Even
		// when both cycles are equal, the machine-enforced ceiling—not the local
		// configured cap—stopped execution, so raising that cap cannot help.
		return fmt.Errorf(
			"%s execution stopped at machine imcyclemax coincident with configured target: %s: %w",
			reqType, progress, origin,
		)
	}

	if errors.Is(origin, ErrMcycleOverflow) {
		source += " (machine imcyclemax)"
	}
	return fmt.Errorf(
		"%s execution reached %s cycle limit without completing: %s: %w",
		reqType, source, progress, origin,
	)
}

// runIncrementInterval runs the machine for at most incrementLimit mcycles (or
// the distance left to limitCycle) and preserves the emulator's typed break
// reason for the request loop to classify.
func (m *machineImpl) runIncrementInterval(ctx context.Context,
	currentCycle Cycle,
	limitCycle Cycle,
	incrementLimit Cycle,
	hashCollectorState *HashCollectorState,
	timeout time.Duration,
) (incrementResult, error) {
	startingCycle := currentCycle

	// Return without calling the backend after an ordinary local target has
	// already been reached. MaxUint64 is different: saturating arithmetic can
	// make both the node target and the emulator's imcyclemax equal the current
	// cycle. Calling the emulator once at that fixed point preserves its
	// authoritative McycleOverflow reason instead of mislabeling the stop as an
	// operator-cap failure.
	atSaturatedEndpoint := currentCycle == ^uint64(0) && limitCycle == ^uint64(0)
	if currentCycle >= limitCycle && !atSaturatedEndpoint {
		return incrementResult{currentCycle: currentCycle}, ErrReachedLimitMcycle
	}

	// Calculates the increment.
	increment := min(incrementLimit, limitCycle-currentCycle)

	m.logger.Debug("machine step before run", "currentCycle", currentCycle, "increment", increment)

	// Runs the machine.
	breakReason, err := m.backendRun(currentCycle+increment, hashCollectorState, timeout)
	if err != nil {
		return incrementResult{currentCycle: currentCycle}, err
	}
	if hashCollectorState != nil && hashCollectorState.ConsoleIOError != "" {
		m.logger.Warn(
			"machine console I/O error while collecting computation-hash entries",
			"error", hashCollectorState.ConsoleIOError,
		)
		hashCollectorState.ConsoleIOError = ""
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

	if atSaturatedEndpoint && breakReason != McycleOverflow {
		// Emulator 0.21 gives mcycle overflow priority over target, yield, and
		// halt when mcycle == imcyclemax. Accepting any other reason here could
		// either hide that terminal origin or repeat zero-progress runs forever.
		return incrementResult{breakReason: breakReason, currentCycle: currentCycle}, fmt.Errorf(
			"machine returned break reason %d instead of mcycle overflow at MaxUint64: %w",
			breakReason, ErrMachineInternal,
		)
	}

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

func isFixedPointBreakReason(reason BreakReason) bool {
	return reason == YieldedManually || reason == Halted || reason == McycleOverflow
}

// Helper functions

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	err := ctx.Err()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ErrDeadlineExceeded
	case errors.Is(err, context.Canceled):
		return ErrCanceled
	default:
		return err
	}
}
