// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cartesi/rollups-node/internal/manager/pmutex"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/semaphore"
)

var (
	ErrMachineClosed          = errors.New("machine is closed")
	ErrInvalidInputIndex      = errors.New("invalid input index")
	ErrInvalidSnapshotPoint   = errors.New("invalid snapshot point")
	ErrInvalidApplication     = errors.New("application must not be nil")
	ErrInvalidAdvanceTimeout  = errors.New("advance timeout must not be negative")
	ErrInvalidInspectTimeout  = errors.New("inspect timeout must not be negative")
	ErrInvalidConcurrentLimit = errors.New("maximum concurrent inspects must not be zero")
	ErrInspectAtCapacity      = errors.New("application inspect at capacity")
)

// MachineInstanceImpl represents a running Cartesi machine for an application.
//
// Concurrency protocol:
//   - runtime:          Protected by PMutex. Written under HLock, read under LLock.
//   - processedInputs:  atomic.Uint64. Written under HLock (together with runtime swap,
//     so writers see a consistent pair). Read lock-free via Load() —
//     this is safe because only one advance runs at a time (advanceMutex)
//     and the atomic store is visible to all goroutines immediately.
//   - advanceMutex:     Serializes all Advance calls. Only one input is processed at a time.
//   - mutex (PMutex):   HLock for advance/snapshot/hash/proof (may destroy runtime on error).
//     LLock for inspect (read-only fork). HLock starves LLock by design.
//   - inspectSemaphore: Bounds concurrent inspect operations.
type MachineInstanceImpl struct {
	application *Application
	runtime     machine.Machine

	// How many inputs were processed by the machine.
	// Written under HLock (together with runtime swap — the two MUST be updated
	// atomically from the perspective of readers). Read without locks via Load().
	processedInputs atomic.Uint64

	// Timeouts for operations
	advanceTimeout time.Duration
	inspectTimeout time.Duration

	// Concurrency control
	maxConcurrentInspects uint32
	mutex                 *pmutex.PMutex
	advanceMutex          sync.Mutex
	inspectSemaphore      *semaphore.Weighted

	// Timeout for draining in-flight inspects during Close
	closeTimeout time.Duration

	// Factory for creating machine runtimes
	runtimeFactory MachineRuntimeFactory

	// Logger
	logger *slog.Logger
}

var (
	ErrInvalidLogger = errors.New("logger must not be nil")
)

// NewMachineInstance creates a new machine instance for an application
func NewMachineInstance(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
) (MachineInstance, error) {
	return NewMachineInstanceWithFactory(ctx, app, 0, logger, checkHash, defaultFactory)
}

// NewMachineInstanceFromSnapshot creates a new machine instance from a snapshot
func NewMachineInstanceFromSnapshot(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
	snapshotPath string,
	machineHash *common.Hash,
	inputIndex uint64,
) (MachineInstance, error) {
	factory := &SnapshotMachineRuntimeFactory{
		SnapshotPath: snapshotPath,
		MachineHash:  machineHash,
	}
	return NewMachineInstanceWithFactory(ctx, app, inputIndex+1, logger, checkHash, factory)
}

// NewMachineInstanceWithFactory creates a new machine instance with a custom factory
func NewMachineInstanceWithFactory(
	ctx context.Context,
	app *Application,
	processedInputs uint64,
	logger *slog.Logger,
	checkHash bool,
	factory MachineRuntimeFactory,
) (MachineInstance, error) {
	// Validate parameters
	if app == nil {
		return nil, ErrInvalidApplication
	}
	if logger == nil {
		return nil, ErrInvalidLogger
	}
	if factory == nil {
		return nil, errors.New("factory must not be nil")
	}

	// Validate timeouts and limits
	if app.ExecutionParameters.AdvanceMaxDeadline < 0 {
		return nil, ErrInvalidAdvanceTimeout
	}
	if app.ExecutionParameters.InspectMaxDeadline < 0 {
		return nil, ErrInvalidInspectTimeout
	}
	if app.ExecutionParameters.MaxConcurrentInspects == 0 {
		return nil, ErrInvalidConcurrentLimit
	}

	// Create the machine server and runtime
	runtime, err := factory.CreateMachineRuntime(ctx, app, logger, checkHash)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMachineCreation, err)
	}

	// Create the machine instance
	instance := &MachineInstanceImpl{
		application:           app,
		runtime:               runtime,
		advanceTimeout:        app.ExecutionParameters.AdvanceMaxDeadline,
		inspectTimeout:        app.ExecutionParameters.InspectMaxDeadline,
		maxConcurrentInspects: app.ExecutionParameters.MaxConcurrentInspects,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(int64(app.ExecutionParameters.MaxConcurrentInspects)),
		closeTimeout:          defaultCloseTimeout,
		runtimeFactory:        factory,
		logger:                logger.With("application", app.Name),
	}
	instance.processedInputs.Store(processedInputs)

	return instance, nil
}

func (m *MachineInstanceImpl) Application() *Application {
	return m.application
}

func (m *MachineInstanceImpl) ProcessedInputs() uint64 {
	return m.processedInputs.Load()
}

// Synchronize brings the machine up to date with processed inputs.
// It handles both template-based instances (processedInputs == 0, replays all)
// and snapshot-based instances (processedInputs > 0, replays only remaining).
// Inputs are fetched in batches to bound memory usage.
func (m *MachineInstanceImpl) Synchronize(ctx context.Context, repo MachineRepository, batchSize uint64) error {
	appAddress := m.application.IApplicationAddress.String()
	currentProcessed := m.processedInputs.Load()
	m.logger.Info("Synchronizing machine with processed inputs",
		"address", appAddress,
		"app_processed_inputs", m.application.ProcessedInputs,
		"machine_processed_inputs", currentProcessed)

	initialProcessedInputs := currentProcessed
	replayed := uint64(0)
	toReplay := uint64(0)

	for {
		p := repository.Pagination{
			Limit:  batchSize,
			Offset: initialProcessedInputs + replayed,
		}
		inputs, totalCount, err := getProcessedInputs(ctx, repo, appAddress, p)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrMachineSynchronization, err)
		}

		// Validate count on the first batch
		if replayed == 0 {
			if totalCount != m.application.ProcessedInputs {
				errorMsg := fmt.Sprintf(
					"processed inputs count mismatch: expected %d, got %d",
					m.application.ProcessedInputs, totalCount)
				m.logger.Error(errorMsg, "address", appAddress)
				return fmt.Errorf("%w: %s", ErrMachineSynchronization, errorMsg)
			}
			if currentProcessed > totalCount {
				return fmt.Errorf(
					"%w: machine has processed %d inputs but DB only has %d",
					ErrMachineSynchronization, currentProcessed, totalCount)
			}
			toReplay = totalCount - currentProcessed
			if toReplay == 0 {
				m.logger.Info("No inputs to replay during synchronization",
					"address", appAddress)
				return nil
			}
		}

		for _, input := range inputs {
			m.logger.Info("Replaying input during synchronization",
				"address", appAddress,
				"epoch_index", input.EpochIndex,
				"input_index", input.Index,
				"progress", fmt.Sprintf("%d/%d", replayed+1, toReplay))

			_, err := m.Advance(ctx, input.RawData, input.EpochIndex, input.Index, false)
			if err != nil {
				return fmt.Errorf("%w: failed to replay input %d: %w",
					ErrMachineSynchronization, input.Index, err)
			}
			replayed++
		}

		if replayed >= toReplay {
			break
		}
		if len(inputs) == 0 {
			return fmt.Errorf(
				"%w: expected to replay %d inputs but only replayed %d",
				ErrMachineSynchronization, toReplay, replayed)
		}
	}

	return nil
}

// forkForAdvance creates a copy of the machine for advance operations
// It verifies the input index and returns a forked machine
func (m *MachineInstanceImpl) forkForAdvance(ctx context.Context, index uint64) (machine.Machine, error) {
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return nil, ErrMachineClosed
	}

	// Verify input index
	current := m.processedInputs.Load()
	if current != index {
		return nil, fmt.Errorf("%w: processed inputs is %d and index is %d",
			ErrInvalidInputIndex, current, index)
	}

	// Fork the machine
	return m.runtime.Fork(ctx)
}

// Advance processes an input and advances the machine state
func (m *MachineInstanceImpl) Advance(ctx context.Context, input []byte, epochIndex uint64, index uint64, computeHashes bool) (*AdvanceResult, error) {
	// Only one advance can be active at a time
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	var fork machine.Machine
	var err error

	// Fork the machine
	fork, err = m.forkForAdvance(ctx, index)
	if err != nil {
		return nil, err
	}

	// Get the machine state before processing
	prevMachineHash, err := fork.Hash(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	prevOutputsHash, err := fork.OutputsHash(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	prevOutputsHashProof, err := fork.OutputsHashProof(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	// Create a timeout context for the advance operation
	advanceCtx, cancel := context.WithTimeout(ctx, m.advanceTimeout)
	defer cancel()

	if computeHashes {
		// write the checkpoint hash before processing
		err = fork.WriteCheckpointHash(advanceCtx, prevMachineHash)
		if err != nil {
			return nil, errors.Join(err, fork.Close())
		}
	}

	// Process the input
	advanceResp, err := fork.Advance(advanceCtx, input, computeHashes)
	status, err := toInputStatus(advanceResp.Accepted, err)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	// Create the result
	result := &AdvanceResult{
		EpochIndex:          epochIndex,
		InputIndex:          index,
		Status:              status,
		Outputs:             advanceResp.Outputs,
		Reports:             advanceResp.Reports,
		Hashes:              advanceResp.Hashes,
		RemainingMetaCycles: advanceResp.RemainingCycles,
		IsDaveConsensus:     computeHashes,
	}

	// If the input was accepted, update the machine state
	if result.Status == InputCompletionStatus_Accepted {
		// Get the machine hash after processing
		result.MachineHash, err = fork.Hash(ctx)
		if err != nil {
			return nil, errors.Join(err, fork.Close())
		}
		result.OutputsHash = advanceResp.OutputsHash
		result.OutputsHashProof, err = fork.OutputsHashProof(ctx)
		if err != nil {
			return nil, errors.Join(err, fork.Close())
		}

		// Replace the current machine with the fork
		m.mutex.HLock()
		oldRuntime := m.runtime
		m.runtime = fork
		m.processedInputs.Add(1)
		m.mutex.Unlock()

		if err := oldRuntime.Close(); err != nil {
			m.logger.Warn("Failed to close old machine runtime", "error", err)
		}
	} else {
		// Use the previous state for rejected inputs
		result.MachineHash = prevMachineHash
		result.OutputsHash = prevOutputsHash
		result.OutputsHashProof = prevOutputsHashProof

		// Close the fork since we're not using it
		if err := fork.Close(); err != nil {
			m.logger.Warn("Failed to close fork machine runtime", "error", err)
		}

		// Update the processed inputs counter
		m.mutex.HLock()
		m.processedInputs.Add(1)
		m.mutex.Unlock()
	}

	return result, nil
}

// forkForInspect creates a copy of the machine for inspect operations
// It returns the forked machine and the current processed inputs count
func (m *MachineInstanceImpl) forkForInspect(ctx context.Context) (machine.Machine, uint64, error) {
	m.mutex.LLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return nil, 0, ErrMachineClosed
	}

	// Fork the machine
	fork, err := m.runtime.Fork(ctx)
	if err != nil {
		return nil, 0, err
	}

	return fork, m.processedInputs.Load(), nil
}

// Inspect queries the machine state without modifying it
func (m *MachineInstanceImpl) Inspect(ctx context.Context, query []byte) (*InspectResult, error) {
	// Limit concurrent inspects. TryAcquire is non-blocking so that a
	// saturated application fails fast and releases its HTTP admission
	// permit, preventing one app from starving others on the same node.
	if !m.inspectSemaphore.TryAcquire(1) {
		return nil, ErrInspectAtCapacity
	}
	defer m.inspectSemaphore.Release(1)

	// Fork the machine (without index validation)
	fork, processedInputs, err := m.forkForInspect(ctx)
	if err != nil {
		return nil, err
	}

	// Create a timeout context for the inspect operation
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()

	// Process the query
	accepted, reports, inspectErr := fork.Inspect(inspectCtx, query)

	// Create the result
	result := &InspectResult{
		ProcessedInputs: processedInputs,
		Accepted:        accepted,
		Reports:         reports,
		Error:           inspectErr,
	}

	// Close the fork
	closeErr := fork.Close()

	// If there was an error closing the fork, return it directly
	// as it's more serious than an inspection error
	if closeErr != nil {
		return nil, closeErr
	}

	// Return the result without an error, since the inspection error
	// is already included in the result
	return result, nil
}

// CreateSnapshot creates a snapshot of the machine's current state
func (m *MachineInstanceImpl) CreateSnapshot(ctx context.Context, processedInputs uint64, path string) error {
	// Acquire the advance mutex to ensure no advance operations are in progress
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	// Acquire HLock since this operation may destroy the runtime on failure.
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return ErrMachineClosed
	}

	// Verify processed inputs
	current := m.processedInputs.Load()
	if current != processedInputs {
		return fmt.Errorf("%w: machine processed inputs is %d and expected is %d",
			ErrInvalidSnapshotPoint, current, processedInputs)
	}

	m.logger.Debug("Creating machine snapshot", "path", path, "processed_inputs", current)

	// Create a context with a timeout for the store operation
	storeCtx, cancel := context.WithTimeout(ctx, m.application.ExecutionParameters.StoreDeadline)
	defer cancel()

	// Store the machine state to the specified path.
	// A Store failure on a local child process indicates an unrecoverable
	// condition (disk full, process crash, etc.) — destroy the runtime.
	err := m.runtime.Store(storeCtx, path)
	if err != nil {
		m.logger.Error("Failed to create snapshot, destroying runtime", "path", path, "error", err)
		return m.destroyRuntime(fmt.Errorf("failed to create snapshot: %w", err))
	}

	m.logger.Debug("Snapshot created successfully", "path", path)
	return nil
}

func (m *MachineInstanceImpl) Hash(ctx context.Context) ([32]byte, error) {
	// Acquire the advance mutex to ensure no advance operations are in progress
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	// Acquire HLock since this operation may destroy the runtime on failure.
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return [32]byte{}, ErrMachineClosed
	}

	m.logger.Debug("Retrieving machine root hash")

	storeCtx, cancel := context.WithTimeout(ctx, m.application.ExecutionParameters.LoadDeadline)
	defer cancel()

	hash, err := m.runtime.Hash(storeCtx)
	if err != nil {
		m.logger.Error("Failed to retrieve machine root hash, destroying runtime", "error", err)
		return [32]byte{}, m.destroyRuntime(fmt.Errorf("failed to retrieve machine root hash: %w", err))
	}

	m.logger.Debug("Machine root hash retrieved successfully", "hash", "0x"+hex.EncodeToString(hash[:]))
	return hash, nil
}

func (m *MachineInstanceImpl) OutputsProof(ctx context.Context) (*OutputsProof, error) {
	// Acquire the advance mutex to ensure no advance operations are in progress
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	// Acquire HLock since this operation may destroy the runtime on failure.
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return nil, ErrMachineClosed
	}

	m.logger.Debug("Retrieving machine hash, outputs merkle root and outputs merkle proof")

	proofCtx, cancel := context.WithTimeout(ctx, m.application.ExecutionParameters.LoadDeadline)
	defer cancel()

	// The runtime is a local child process — errors here indicate the process
	// crashed, ran out of resources, or is otherwise unrecoverable.
	// Close the runtime to avoid leaving a broken process alive.
	machineHash, err := m.runtime.Hash(proofCtx)
	if err != nil {
		return nil, m.destroyRuntime(fmt.Errorf("failed to get machine hash: %w", err))
	}

	outputsHash, err := m.runtime.OutputsHash(proofCtx)
	if err != nil {
		return nil, m.destroyRuntime(fmt.Errorf("failed to get outputs hash: %w", err))
	}

	outputsHashProof, err := m.runtime.OutputsHashProof(proofCtx)
	if err != nil {
		return nil, m.destroyRuntime(fmt.Errorf("failed to get outputs hash proof: %w", err))
	}

	proof := &OutputsProof{
		MachineHash:      machineHash,
		OutputsHash:      outputsHash,
		OutputsHashProof: outputsHashProof,
	}

	m.logger.Debug("Machine hash, outputs merkle root and outputs merkle proof retrieved successfully",
		"hash", "0x"+hex.EncodeToString(machineHash[:]))
	return proof, nil
}

// defaultCloseTimeout is how long Close waits for in-flight inspects to drain
// before forcibly closing the runtime.
const defaultCloseTimeout = 30 * time.Second

// Close shuts down the machine instance
func (m *MachineInstanceImpl) Close() error {
	// Acquire all locks to ensure no operations are in progress
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.closeTimeout)
	defer cancel()

	acquired := 0
	for range int(m.maxConcurrentInspects) {
		if err := m.inspectSemaphore.Acquire(ctx, 1); err != nil {
			m.logger.Warn("Timed out waiting for in-flight inspects to drain; closing anyway",
				"still_in_flight", int(m.maxConcurrentInspects)-acquired,
				"drained", acquired)
			break
		}
		acquired++
	}
	defer m.inspectSemaphore.Release(int64(acquired))

	// Close the runtime
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return nil
	}

	err := m.runtime.Close()
	m.runtime = nil
	return err
}

// destroyRuntime closes the runtime and nils it out so that subsequent calls
// fail fast with ErrMachineClosed instead of talking to a broken process.
// Must be called while holding the appropriate locks.
func (m *MachineInstanceImpl) destroyRuntime(cause error) error {
	if m.runtime == nil {
		return cause
	}
	closeErr := m.runtime.Close()
	m.runtime = nil
	return errors.Join(cause, closeErr)
}

// MachineRuntimeFactory defines an interface for creating machine runtimes
type MachineRuntimeFactory interface {
	CreateMachineRuntime(
		ctx context.Context,
		app *Application,
		logger *slog.Logger,
		checkHash bool,
	) (machine.Machine, error)
}

// createMachineRuntimeCommon contains the shared logic for creating machine runtimes
func createMachineRuntimeCommon(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
	machinePath string,
	sourceType string,
	expectedHash common.Hash,
) (machine.Machine, error) {
	if logger == nil {
		return nil, ErrInvalidLogger
	}

	appAddress := app.IApplicationAddress.String()

	// Load the machine
	logger.Info(fmt.Sprintf("Loading machine runtime from %s", sourceType),
		"application", app.Name,
		"address", appAddress,
		"path", machinePath)

	// Create machine configuration
	config := machine.DefaultConfig(machinePath)
	config.ExecutionParameters = app.ExecutionParameters

	// Create the machine
	m, err := machine.Load(ctx, logger, config)
	if err != nil {
		return nil, err
	}

	logger.Debug(fmt.Sprintf("Machine loaded from %s", sourceType),
		"application", app.Name,
		"address", appAddress,
		"remote-machine", m.Address(),
		"path", machinePath)

	// Verify the machine hash if required
	if checkHash {
		logger.Debug("Verifying machine hash",
			"application", app.Name,
			"address", appAddress)

		machineHash, err := m.Hash(ctx)
		if err != nil {
			return nil, errors.Join(err, m.Close())
		}

		if common.Hash(machineHash) != expectedHash {
			logger.Error("Machine hash mismatch",
				"application", app.Name,
				"address", appAddress,
				"machine-hash", common.Hash(machineHash).Hex(),
				"expected-hash", expectedHash.Hex())

			err = fmt.Errorf("machine hash mismatch: expected %s, got %s",
				expectedHash, machineHash)
			return nil, errors.Join(err, m.Close())
		}
	}

	return m, nil
}

// DefaultMachineRuntimeFactory is the standard implementation of MachineRuntimeFactory
type DefaultMachineRuntimeFactory struct{}

// CreateMachineRuntime creates a new machine runtime for an application
func (f *DefaultMachineRuntimeFactory) CreateMachineRuntime(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
) (machine.Machine, error) {
	return createMachineRuntimeCommon(
		ctx,
		app,
		logger,
		checkHash,
		app.TemplateURI,
		"template",
		app.TemplateHash,
	)
}

// SnapshotMachineRuntimeFactory creates machine runtimes from snapshots
type SnapshotMachineRuntimeFactory struct {
	SnapshotPath string
	MachineHash  *common.Hash // The hash to check against (from the input's machine_hash)
}

// CreateMachineRuntime creates a new machine runtime from a snapshot
func (f *SnapshotMachineRuntimeFactory) CreateMachineRuntime(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
) (machine.Machine, error) {
	// Determine which hash to check against
	expectedHash := app.TemplateHash
	if f.MachineHash != nil {
		expectedHash = *f.MachineHash
	}

	return createMachineRuntimeCommon(
		ctx,
		app,
		logger,
		checkHash,
		f.SnapshotPath,
		"snapshot",
		expectedHash,
	)
}

// Default factory instance
var defaultFactory MachineRuntimeFactory = &DefaultMachineRuntimeFactory{}

// Helper function to convert machine response to input status
func toInputStatus(accepted bool, err error) (status InputCompletionStatus, _ error) {
	if err == nil {
		if accepted {
			return InputCompletionStatus_Accepted, nil
		} else {
			return InputCompletionStatus_Rejected, nil
		}
	}

	switch {
	case errors.Is(err, machine.ErrException):
		return InputCompletionStatus_Exception, nil
	case errors.Is(err, machine.ErrHalted):
		return InputCompletionStatus_MachineHalted, nil
	case errors.Is(err, machine.ErrOutputsLimitExceeded):
		return InputCompletionStatus_OutputsLimitExceeded, nil
	case errors.Is(err, machine.ErrReportsLimitExceeded):
		return InputCompletionStatus_ReportsLimitExceeded, nil
	case errors.Is(err, machine.ErrReachedTargetMcycle):
		return InputCompletionStatus_CycleLimitExceeded, nil
	case errors.Is(err, machine.ErrPayloadLengthLimitExceeded):
		return InputCompletionStatus_PayloadLengthLimitExceeded, nil
	case errors.Is(err, machine.ErrDeadlineExceeded):
		return InputCompletionStatus_TimeLimitExceeded, nil
	case errors.Is(err, machine.ErrMachineInternal):
		fallthrough
	default:
		return status, err
	}
}
