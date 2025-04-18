// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/manager/pmutex"
	. "github.com/cartesi/rollups-node/internal/model"
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
)

// MachineInstanceImpl represents a running Cartesi machine for an application
type MachineInstanceImpl struct {
	application *Application
	runtime     machine.Machine

	// How many inputs were processed by the machine
	processedInputs uint64

	// Timeouts for operations
	advanceTimeout time.Duration
	inspectTimeout time.Duration

	// Concurrency control
	maxConcurrentInspects uint32
	mutex                 *pmutex.PMutex
	advanceMutex          sync.Mutex
	inspectSemaphore      *semaphore.Weighted

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
		processedInputs:       processedInputs,
		advanceTimeout:        app.ExecutionParameters.AdvanceMaxDeadline,
		inspectTimeout:        app.ExecutionParameters.InspectMaxDeadline,
		maxConcurrentInspects: app.ExecutionParameters.MaxConcurrentInspects,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(int64(app.ExecutionParameters.MaxConcurrentInspects)),
		runtimeFactory:        factory,
		logger:                logger.With("application", app.Name),
	}

	return instance, nil
}

func (m *MachineInstanceImpl) Application() *Application {
	return m.application
}

// Synchronize brings the machine up to date with processed inputs
func (m *MachineInstanceImpl) Synchronize(ctx context.Context, repo MachineRepository) error {
	appAddress := m.application.IApplicationAddress.String()
	m.logger.Info("Synchronizing machine processed inputs",
		"address", appAddress,
		"processed_inputs", m.application.ProcessedInputs)

	// Get all processed inputs for this application
	inputs, _, err := getProcessedInputs(ctx, repo, appAddress)
	if err != nil {
		return err
	}

	// Verify that the number of inputs matches what's expected
	if uint64(len(inputs)) != m.application.ProcessedInputs {
		errorMsg := fmt.Sprintf("processed inputs count mismatch: expected %d, got %d",
			m.application.ProcessedInputs, len(inputs))
		m.logger.Error(errorMsg, "address", appAddress)
		return fmt.Errorf("%w: %s", ErrMachineSynchronization, errorMsg)
	}

	if len(inputs) == 0 {
		m.logger.Info("No previous processed inputs to synchronize", "address", appAddress)
		return nil
	}

	// Process each input to bring the machine to the current state
	for _, input := range inputs {
		m.logger.Info("Replaying input during synchronization",
			"address", appAddress,
			"epoch_index", input.EpochIndex,
			"input_index", input.Index)

		_, err := m.Advance(ctx, input.RawData, input.Index)
		if err != nil {
			return fmt.Errorf("%w: failed to replay input %d: %v",
				ErrMachineSynchronization, input.Index, err)
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
	if m.processedInputs != index {
		return nil, fmt.Errorf("%w: processed inputs is %d and index is %d", ErrInvalidInputIndex, m.processedInputs, index)
	}

	// Fork the machine
	return m.runtime.Fork(ctx)
}

// Advance processes an input and advances the machine state
func (m *MachineInstanceImpl) Advance(ctx context.Context, input []byte, index uint64) (*AdvanceResult, error) {
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

	// Create a timeout context for the advance operation
	advanceCtx, cancel := context.WithTimeout(ctx, m.advanceTimeout)
	defer cancel()

	// Process the input
	accepted, outputs, reports, outputsHash, err := fork.Advance(advanceCtx, input)
	status, err := toInputStatus(accepted, err)
	if err != nil {
		return nil, errors.Join(err, fork.Close())
	}

	// Create the result
	result := &AdvanceResult{
		InputIndex:  index,
		Status:      status,
		Outputs:     outputs,
		Reports:     reports,
		OutputsHash: outputsHash,
	}

	// If the input was accepted, update the machine state
	if result.Status == InputCompletionStatus_Accepted {
		// Get the machine hash after processing
		machineHash, err := fork.Hash(ctx)
		if err != nil {
			return nil, errors.Join(err, fork.Close())
		}
		result.MachineHash = (*common.Hash)(&machineHash)

		// Replace the current machine with the fork
		m.mutex.HLock()
		if err = m.runtime.Close(); err != nil {
			m.mutex.Unlock()
			return nil, err
		}
		m.runtime = fork
		m.processedInputs++
		m.mutex.Unlock()
	} else {
		// Use the previous state for rejected inputs
		result.MachineHash = (*common.Hash)(&prevMachineHash)
		result.OutputsHash = prevOutputsHash

		// Close the fork since we're not using it
		err = fork.Close()

		// Update the processed inputs counter
		m.mutex.HLock()
		m.processedInputs++
		m.mutex.Unlock()
	}

	return result, err
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

	return fork, m.processedInputs, nil
}

// Inspect queries the machine state without modifying it
func (m *MachineInstanceImpl) Inspect(ctx context.Context, query []byte) (*InspectResult, error) {
	// Limit concurrent inspects
	err := m.inspectSemaphore.Acquire(ctx, 1)
	if err != nil {
		return nil, err
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

	// Acquire a read lock on the machine
	m.mutex.LLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return ErrMachineClosed
	}

	// Verify processed inputs
	if m.processedInputs != processedInputs {
		return fmt.Errorf("%w: machine processed inputs is %d and expected is %d", ErrInvalidSnapshotPoint, m.processedInputs, processedInputs)
	}

	m.logger.Debug("Creating machine snapshot", "path", path, "processed_inputs", m.processedInputs)

	// Create a context with a timeout for the store operation
	storeCtx, cancel := context.WithTimeout(ctx, m.application.ExecutionParameters.StoreDeadline)
	defer cancel()

	// Store the machine state to the specified path
	err := m.runtime.Store(storeCtx, path)
	if err != nil {
		m.logger.Error("Failed to create snapshot", "path", path, "error", err)
		return err
	}

	m.logger.Debug("Snapshot created successfully", "path", path)
	return nil
}

// Close shuts down the machine instance
func (m *MachineInstanceImpl) Close() error {
	// Acquire all locks to ensure no operations are in progress
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	ctx := context.Background()
	for range int(m.maxConcurrentInspects) {
		_ = m.inspectSemaphore.Acquire(ctx, 1)
		defer m.inspectSemaphore.Release(1)
	}

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
