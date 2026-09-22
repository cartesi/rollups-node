// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cartesi/rollups-node/internal/manager/pmutex"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/semaphore"
)

var (
	ErrMachineClosed          = errors.New("machine is closed")
	ErrInvalidInputIndex      = errors.New("invalid input index")
	ErrIncompleteAdvance      = errors.New("machine advance returned no completed result")
	ErrIncompleteInspect      = errors.New("machine inspect returned no completed result")
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
	application *model.Application
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
	app *model.Application,
	logger *slog.Logger,
	checkTemplateHash bool,
) (MachineInstance, error) {
	factory := &DefaultMachineRuntimeFactory{CheckTemplateHash: checkTemplateHash}
	return NewMachineInstanceWithFactory(ctx, app, 0, logger, factory)
}

// NewMachineInstanceFromSnapshot creates a new machine instance from a snapshot
func NewMachineInstanceFromSnapshot(
	ctx context.Context,
	app *model.Application,
	logger *slog.Logger,
	snapshotPath string,
	expectedHash common.Hash,
	inputIndex uint64,
) (MachineInstance, error) {
	return newMachineInstanceFromSnapshot(
		ctx, app, logger, snapshotPath, expectedHash, inputIndex, nil,
	)
}

func newMachineInstanceFromSnapshot(
	ctx context.Context,
	app *model.Application,
	logger *slog.Logger,
	snapshotPath string,
	expectedHash common.Hash,
	inputIndex uint64,
	loader machineLoader,
) (MachineInstance, error) {
	if inputIndex == math.MaxUint64 {
		return nil, fmt.Errorf("%w: snapshot input index cannot be incremented", ErrInvalidSnapshotPoint)
	}
	factory := &SnapshotMachineRuntimeFactory{
		SnapshotPath: snapshotPath,
		ExpectedHash: expectedHash,
		loader:       loader,
	}
	return NewMachineInstanceWithFactory(ctx, app, inputIndex+1, logger, factory)
}

// NewMachineInstanceWithFactory creates a new machine instance with a custom factory
func NewMachineInstanceWithFactory(
	ctx context.Context,
	app *model.Application,
	processedInputs uint64,
	logger *slog.Logger,
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
	runtime, err := factory.CreateMachineRuntime(ctx, app, logger)
	if err != nil {
		if runtime != nil {
			err = errors.Join(err, runtime.Close())
		}
		return nil, fmt.Errorf("%w: %w", ErrMachineCreation, err)
	}
	if runtime == nil {
		return nil, fmt.Errorf("%w: runtime factory returned nil runtime", ErrMachineCreation)
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

func (m *MachineInstanceImpl) Application() *model.Application {
	return m.application
}

func (m *MachineInstanceImpl) ProcessedInputs() uint64 {
	return m.processedInputs.Load()
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

// Advance treats a machine fork as the execution transaction for one input.
// It executes on the fork, selects the canonical root from the typed completion
// status, and advances processedInputs exactly once for every completed input.
// Accepted completions adopt the post-run fork, rejected inputs keep the
// predecessor runtime, and terminal completions dispose both runtimes after
// collecting the post-run proof. Incomplete execution returns an error and
// adopts neither the fork nor a canonical result. Disposing a terminal runtime
// here prevents inspect from observing it before the durable application status
// is committed by the advancer.
func (m *MachineInstanceImpl) Advance(
	ctx context.Context,
	input []byte,
	epochIndex uint64,
	index uint64,
	computeHashes bool,
) (*model.AdvanceResult, error) {
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

	// Every input starts from the accepted state that can receive it. Keep its
	// proof as the canonical result if the input rejects and the fork is
	// discarded.
	prevMachineProof, err := fork.StateProof(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}
	if err := machine.ValidateAcceptedState(prevMachineProof); err != nil {
		return nil, errors.Join(err, fork.Close())
	}
	prevProof, err := stateProofFromMachine(prevMachineProof)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	// Create a timeout context for the advance operation
	advanceCtx, cancel := context.WithTimeout(ctx, m.advanceTimeout)
	defer cancel()

	// Process the input
	advanceResp, err := fork.Advance(advanceCtx, input, prevProof.MachineHash, computeHashes)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}
	if advanceResp == nil {
		return nil, errors.Join(ErrIncompleteAdvance, fork.Close())
	}
	if err := validateCompletionExceptionData(advanceResp.Status, advanceResp.ExceptionData); err != nil {
		return nil, errors.Join(ErrIncompleteAdvance, err, fork.Close())
	}
	status, err := toInputStatus(advanceResp.Status)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	// Create the result
	result := &model.AdvanceResult{
		EpochIndex:          epochIndex,
		InputIndex:          index,
		Status:              status,
		ExceptionData:       advanceResp.ExceptionData,
		PeriodicStateHashes: advanceResp.PeriodicStateHashes,
		PaddingRepetitions:  advanceResp.PaddingRepetitions,
		IsDaveConsensus:     computeHashes,
	}

	// Resolve the canonical result and fork disposition once. Validation below
	// must succeed before the selected disposition mutates the live instance.
	adoptFork := false
	terminalCompletion := false
	switch result.Status {
	case model.InputCompletionStatus_Accepted:
		postMachineProof, proofErr := fork.StateProof(ctx)
		if proofErr != nil {
			return nil, errors.Join(proofErr, fork.Close())
		}
		if proofErr := machine.ValidateAcceptedState(postMachineProof); proofErr != nil {
			return nil, errors.Join(proofErr, fork.Close())
		}
		postProof, proofErr := stateProofFromMachine(postMachineProof)
		if proofErr != nil {
			return nil, errors.Join(proofErr, fork.Close())
		}
		result.StateProof = *postProof
		result.Outputs = advanceResp.Outputs
		result.Reports = advanceResp.Reports
		adoptFork = true
	case model.InputCompletionStatus_Rejected:
		// Rejected execution has no canonical state transition or effects.
		result.StateProof = *prevProof
	case model.InputCompletionStatus_Exception,
		model.InputCompletionStatus_MachineHalted,
		model.InputCompletionStatus_Overflow,
		model.InputCompletionStatus_UnexpectedYield:
		// Terminal execution preserves and proves the actual post-run state.
		postMachineProof, proofErr := fork.StateProof(ctx)
		if proofErr != nil {
			return nil, errors.Join(proofErr, fork.Close())
		}
		postProof, proofErr := stateProofFromMachine(postMachineProof)
		if proofErr != nil {
			return nil, errors.Join(proofErr, fork.Close())
		}
		result.StateProof = *postProof
		terminalCompletion = true
	case model.InputCompletionStatus_None:
		return nil, errors.Join(
			fmt.Errorf("cannot resolve advance result for status %q", result.Status),
			fork.Close(),
		)
	default:
		return nil, errors.Join(
			fmt.Errorf("cannot resolve advance result for unknown status %q", result.Status),
			fork.Close(),
		)
	}

	if computeHashes {
		err = validateCanonicalInputHashCollectionSpan(
			uint64(len(result.PeriodicStateHashes)),
			result.PaddingRepetitions,
		)
		if err != nil {
			return nil, errors.Join(err, fork.Close())
		}
	}

	switch {
	case terminalCompletion:
		// A completed terminal input has no runnable successor. Make the
		// runtime unavailable before returning so concurrent inspect requests
		// cannot fork the terminal state while its database transaction is
		// waiting to commit. The complete post-run proof above remains in the
		// returned result; if persistence fails, the advancer shuts down and a
		// restart reconstructs execution from persisted state.
		m.mutex.HLock()
		oldRuntime := m.runtime
		m.runtime = nil
		m.processedInputs.Add(1)
		m.mutex.Unlock()

		if err := oldRuntime.Close(); err != nil {
			m.logger.Warn("Failed to close predecessor machine runtime after terminal completion", "error", err)
		}
		if err := fork.Close(); err != nil {
			m.logger.Warn("Failed to close terminal machine runtime", "error", err)
		}
	case adoptFork:
		// Replace the current machine with the fork
		m.mutex.HLock()
		oldRuntime := m.runtime
		m.runtime = fork
		m.processedInputs.Add(1)
		m.mutex.Unlock()

		if err := oldRuntime.Close(); err != nil {
			m.logger.Warn("Failed to close old machine runtime", "error", err)
		}
	default:

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

func validateCanonicalInputHashCollectionSpan(
	hashCount uint64,
	paddingRepetitions uint64,
) error {
	if err := machine.ValidateInputHashCollectionSpan(
		hashCount, paddingRepetitions,
	); err != nil {
		return fmt.Errorf("invalid canonical input hash collection span: %w", err)
	}
	if paddingRepetitions == 0 {
		return errors.New("canonical input hash collection requires a positive final repetition tail")
	}
	return nil
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
	inspectResponse, inspectErr := fork.Inspect(inspectCtx, query)

	// Create the result
	result := &InspectResult{
		ProcessedInputs: processedInputs,
	}
	if inspectResponse != nil {
		result.Status = inspectResponse.Status
		result.ExceptionData = inspectResponse.ExceptionData
		result.Reports = inspectResponse.Reports
	}

	// An execution error takes precedence over any status supplied by a Machine
	// implementation. Inspection did not complete, but reports emitted before
	// the failure remain useful to the caller.
	if inspectErr != nil {
		result.Status = machine.CompletionStatusUnknown
		result.Error = inspectErr
	} else if inspectResponse == nil || !result.Status.IsCompleted() {
		result.Status = machine.CompletionStatusUnknown
		result.Error = errors.Join(ErrIncompleteInspect, machine.ErrMachineInternal)
	} else if err := validateCompletionExceptionData(result.Status, result.ExceptionData); err != nil {
		result.Status = machine.CompletionStatusUnknown
		result.Error = errors.Join(ErrIncompleteInspect, err)
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

func (m *MachineInstanceImpl) StateProof(ctx context.Context) (*model.StateProof, error) {
	// Acquire the advance mutex to ensure no advance operations are in progress
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	// Acquire HLock since this operation may destroy the runtime on failure.
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return nil, ErrMachineClosed
	}

	m.logger.Debug("Retrieving accepted machine state proof")

	proofCtx, cancel := context.WithTimeout(ctx, m.application.ExecutionParameters.LoadDeadline)
	defer cancel()

	// The runtime is a local child process — errors here indicate the process
	// crashed, ran out of resources, or is otherwise unrecoverable.
	// Close the runtime to avoid leaving a broken process alive.
	machineProof, err := m.runtime.StateProof(proofCtx)
	if err != nil {
		return nil, m.destroyRuntime(fmt.Errorf("failed to get machine state proof: %w", err))
	}
	if err := machine.ValidateAcceptedState(machineProof); err != nil {
		return nil, m.destroyRuntime(fmt.Errorf("machine is not at an accepted state: %w", err))
	}
	proof, err := stateProofFromMachine(machineProof)
	if err != nil {
		return nil, m.destroyRuntime(err)
	}

	m.logger.Debug("Accepted machine state proof retrieved successfully",
		"hash", "0x"+hex.EncodeToString(proof.MachineHash[:]))
	return proof, nil
}

func stateProofFromMachine(machineProof *machine.StateProof) (*model.StateProof, error) {
	if machineProof == nil {
		return nil, fmt.Errorf(
			"machine returned no state proof: %w", machine.ErrInvalidMachineProof,
		)
	}
	proof := &model.StateProof{
		MachineHash:         machineProof.MachineHash,
		TxBufferDataBlock:   machineProof.TxBufferProof.DataBlock,
		TxBufferProof:       machineProof.TxBufferProof.Siblings,
		IflagsYDataBlock:    machineProof.IflagsYProof.DataBlock,
		IflagsYProof:        machineProof.IflagsYProof.Siblings,
		HtifTohostDataBlock: machineProof.HtifTohostProof.DataBlock,
		HtifTohostProof:     machineProof.HtifTohostProof.Siblings,
	}
	if !proof.IsComplete() {
		return nil, fmt.Errorf(
			"machine returned an incomplete state proof: %w",
			machine.ErrInvalidMachineProof,
		)
	}
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
	// Cancellation is checked before backend calls and does not imply that the
	// child process is unhealthy. Preserve it for graceful shutdown/retry.
	if errors.Is(cause, machine.ErrCanceled) {
		return cause
	}
	if m.runtime == nil {
		return errors.Join(ErrMachineClosed, cause)
	}
	closeErr := m.runtime.Close()
	m.runtime = nil
	return errors.Join(ErrMachineClosed, cause, closeErr)
}

// MachineRuntimeFactory defines an interface for creating machine runtimes
type MachineRuntimeFactory interface {
	CreateMachineRuntime(
		ctx context.Context,
		app *model.Application,
		logger *slog.Logger,
	) (machine.Machine, error)
}

type machineLoader func(
	ctx context.Context,
	logger *slog.Logger,
	config *machine.MachineConfig,
) (machine.Machine, error)

// createMachineRuntimeCommon contains the shared logic for creating machine runtimes
func createMachineRuntimeCommon(
	ctx context.Context,
	app *model.Application,
	logger *slog.Logger,
	verifyExpectedHash bool,
	machinePath string,
	sourceType string,
	expectedHash common.Hash,
	loader machineLoader,
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
	if loader == nil {
		loader = machine.Load
	}
	m, err := loader(ctx, logger, config)
	if err != nil {
		// Preserve a partially created runtime so the caller can close it.
		return m, err
	}
	if m == nil {
		return nil, errors.New("machine loader returned nil runtime without an error")
	}

	logger.Debug(fmt.Sprintf("Machine loaded from %s", sourceType),
		"application", app.Name,
		"address", appAddress,
		"remote-machine", m.Address(),
		"path", machinePath)

	// Verify the machine hash if required
	if verifyExpectedHash {
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

// DefaultMachineRuntimeFactory is the standard template implementation of
// MachineRuntimeFactory. Template verification remains configurable for
// backwards compatibility; snapshot verification is always mandatory.
type DefaultMachineRuntimeFactory struct {
	CheckTemplateHash bool
	loader            machineLoader
}

// CreateMachineRuntime creates a new machine runtime for an application
func (f *DefaultMachineRuntimeFactory) CreateMachineRuntime(
	ctx context.Context,
	app *model.Application,
	logger *slog.Logger,
) (machine.Machine, error) {
	return createMachineRuntimeCommon(
		ctx,
		app,
		logger,
		f.CheckTemplateHash,
		app.TemplateURI,
		"template",
		app.TemplateHash,
		f.loader,
	)
}

// SnapshotMachineRuntimeFactory creates machine runtimes from snapshots
type SnapshotMachineRuntimeFactory struct {
	SnapshotPath string
	ExpectedHash common.Hash
	loader       machineLoader
}

// CreateMachineRuntime creates a new machine runtime from a snapshot
func (f *SnapshotMachineRuntimeFactory) CreateMachineRuntime(
	ctx context.Context,
	app *model.Application,
	logger *slog.Logger,
) (machine.Machine, error) {
	return createMachineRuntimeCommon(
		ctx,
		app,
		logger,
		true,
		f.SnapshotPath,
		"snapshot",
		f.ExpectedHash,
		f.loader,
	)
}

// toInputStatus converts only completed, deterministic machine statuses to
// canonical input statuses. Infrastructure interruptions never reach here.
func toInputStatus(status machine.CompletionStatus) (model.InputCompletionStatus, error) {
	switch status {
	case machine.CompletionStatusAccepted:
		return model.InputCompletionStatus_Accepted, nil
	case machine.CompletionStatusRejected:
		return model.InputCompletionStatus_Rejected, nil
	case machine.CompletionStatusException:
		return model.InputCompletionStatus_Exception, nil
	case machine.CompletionStatusHalted:
		return model.InputCompletionStatus_MachineHalted, nil
	case machine.CompletionStatusOverflow:
		return model.InputCompletionStatus_Overflow, nil
	case machine.CompletionStatusUnexpectedYield:
		return model.InputCompletionStatus_UnexpectedYield, nil
	case machine.CompletionStatusUnknown:
		// Intentionally empty.
	}
	return model.InputCompletionStatus_None, fmt.Errorf(
		"unknown completed machine status %d: %w",
		status,
		ErrIncompleteAdvance,
	)
}

func validateCompletionExceptionData(status machine.CompletionStatus, data []byte) error {
	switch {
	case status == machine.CompletionStatusException && data == nil:
		return fmt.Errorf("completed exception has no exception data: %w", machine.ErrMachineInternal)
	case status != machine.CompletionStatusException && data != nil:
		return fmt.Errorf(
			"completion status %d unexpectedly has exception data: %w",
			status,
			machine.ErrMachineInternal,
		)
	default:
		return nil
	}
}
