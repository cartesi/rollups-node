// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/manager/pmutex"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/semaphore"
)

var (
	ErrMachineClosed          = errors.New("machine is closed")
	ErrInvalidInputIndex      = errors.New("invalid input index")
	ErrInvalidApplication     = errors.New("application must not be nil")
	ErrInvalidAdvanceTimeout  = errors.New("advance timeout must not be negative")
	ErrInvalidInspectTimeout  = errors.New("inspect timeout must not be negative")
	ErrInvalidConcurrentLimit = errors.New("maximum concurrent inspects must not be zero")
)

// MachineInstanceImpl represents a running Cartesi machine for an application
type MachineInstanceImpl struct {
	application *Application
	runtime     rollupsmachine.RollupsMachine

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

	// Logger
	logger *slog.Logger
}

var (
	ErrInvalidLogger = errors.New("logger must not be nil")
)

// NewMachineInstance creates a new machine instance for an application
func NewMachineInstance(
	ctx context.Context,
	verbosity MachineLogLevel,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
) (MachineInstance, error) {
	// Validate parameters
	if app == nil {
		return nil, ErrInvalidApplication
	}
	if logger == nil {
		return nil, ErrInvalidLogger
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
	runtime, err := createMachineRuntime(ctx, verbosity, app, logger, checkHash)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMachineCreation, err)
	}

	// Create the machine instance
	instance := &MachineInstanceImpl{
		application:           app,
		runtime:               runtime,
		processedInputs:       0,
		advanceTimeout:        app.ExecutionParameters.AdvanceMaxDeadline,
		inspectTimeout:        app.ExecutionParameters.InspectMaxDeadline,
		maxConcurrentInspects: app.ExecutionParameters.MaxConcurrentInspects,
		mutex:                 pmutex.New(),
		inspectSemaphore:      semaphore.NewWeighted(int64(app.ExecutionParameters.MaxConcurrentInspects)),
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
	m.logger.Info("Synchronizing machine with processed inputs",
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
func (m *MachineInstanceImpl) forkForAdvance(ctx context.Context, index uint64) (rollupsmachine.RollupsMachine, error) {
	m.mutex.HLock()
	defer m.mutex.Unlock()

	if m.runtime == nil {
		return nil, ErrMachineClosed
	}

	// Verify input index
	if m.processedInputs != index {
		return nil, ErrInvalidInputIndex
	}

	// Fork the machine
	return m.runtime.Fork(ctx)
}

// Advance processes an input and advances the machine state
func (m *MachineInstanceImpl) Advance(ctx context.Context, input []byte, index uint64) (*AdvanceResult, error) {
	// Only one advance can be active at a time
	m.advanceMutex.Lock()
	defer m.advanceMutex.Unlock()

	var fork rollupsmachine.RollupsMachine
	var err error

	// Fork the machine
	fork, err = m.forkForAdvance(ctx, index)
	if err != nil {
		return nil, err
	}

	// Get the machine state before processing
	prevMachineHash, err := fork.Hash(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close(ctx))
	}

	prevOutputsHash, err := fork.OutputsHash(ctx)
	if err != nil {
		return nil, errors.Join(err, fork.Close(ctx))
	}

	// Create a timeout context for the advance operation
	advanceCtx, cancel := context.WithTimeout(ctx, m.advanceTimeout)
	defer cancel()

	// Process the input
	accepted, outputs, reports, outputsHash, err := fork.Advance(advanceCtx, input)
	status, err := toInputStatus(accepted, err)
	if err != nil {
		return nil, errors.Join(err, fork.Close(ctx))
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
			return nil, errors.Join(err, fork.Close(ctx))
		}
		result.MachineHash = (*common.Hash)(&machineHash)

		// Replace the current machine with the fork
		m.mutex.HLock()
		if err = m.runtime.Close(ctx); err != nil {
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
		err = fork.Close(ctx)

		// Update the processed inputs counter
		m.mutex.HLock()
		m.processedInputs++
		m.mutex.Unlock()
	}

	return result, err
}

// forkForInspect creates a copy of the machine for inspect operations
// It returns the forked machine and the current processed inputs count
func (m *MachineInstanceImpl) forkForInspect(ctx context.Context) (rollupsmachine.RollupsMachine, uint64, error) {
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
	closeErr := fork.Close(ctx)

	// If there was an error closing the fork, return it directly
	// as it's more serious than an inspection error
	if closeErr != nil {
		return nil, closeErr
	}

	// Return the result without an error, since the inspection error
	// is already included in the result
	return result, nil
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

	err := m.runtime.Close(ctx)
	m.runtime = nil
	return err
}

// Variable holding the function to create a machine runtime
var createMachineRuntime = func(
	ctx context.Context,
	verbosity MachineLogLevel,
	app *Application,
	logger *slog.Logger,
	checkHash bool,
) (rollupsmachine.RollupsMachine, error) {
	if logger == nil {
		return nil, ErrInvalidLogger
	}

	appAddress := app.IApplicationAddress.String()
	logger.Info("Creating machine runtime",
		"application", app.Name,
		"address", appAddress,
		"template-path", app.TemplateURI)

	// Start the machine server
	address, err := cartesimachine.StartServer(logger, verbosity, 0, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Load the machine template
	logger.Info("Loading machine on server",
		"application", app.Name,
		"address", appAddress,
		"remote-machine", address,
		"template-path", app.TemplateURI)

	// Create the machine from the template
	machine, err := cartesimachine.Load(ctx, app.TemplateURI, address, nil)
	if err != nil {
		return nil, errors.Join(err, cartesimachine.StopServer(address, logger))
	}

	logger.Debug("Machine loaded on server",
		"application", app.Name,
		"address", appAddress,
		"remote-machine", address,
		"template-path", app.TemplateURI)

	// Verify the machine hash if required
	if checkHash {
		logger.Debug("Verifying machine hash",
			"application", app.Name,
			"address", appAddress)

		machineHash, err := machine.ReadHash(ctx)
		if err != nil {
			return nil, errors.Join(err, cartesimachine.StopServer(address, logger))
		}

		if machineHash != app.TemplateHash {
			logger.Error("Machine hash mismatch",
				"application", app.Name,
				"address", appAddress,
				"machine-hash", common.Hash(machineHash).Hex(),
				"expected-hash", app.TemplateHash.Hex())

			err = fmt.Errorf("machine hash mismatch: expected %s, got %s",
				app.TemplateHash, machineHash)
			return nil, errors.Join(err, machine.Close(ctx))
		}
	}

	// Create the rollups machine
	runtime, err := rollupsmachine.New(ctx,
		machine,
		app.ExecutionParameters.AdvanceIncCycles,
		app.ExecutionParameters.AdvanceMaxCycles,
		logger,
	)
	if err != nil {
		return nil, errors.Join(err, machine.Close(ctx))
	}

	return runtime, nil
}

// Helper function to convert machine response to input status
func toInputStatus(accepted bool, err error) (status InputCompletionStatus, _ error) {
	if err == nil {
		if accepted {
			return InputCompletionStatus_Accepted, nil
		} else {
			return InputCompletionStatus_Rejected, nil
		}
	}

	if errors.Is(err, cartesimachine.ErrTimedOut) {
		return InputCompletionStatus_TimeLimitExceeded, nil
	}

	switch {
	case errors.Is(err, rollupsmachine.ErrException):
		return InputCompletionStatus_Exception, nil
	case errors.Is(err, rollupsmachine.ErrHalted):
		return InputCompletionStatus_MachineHalted, nil
	case errors.Is(err, rollupsmachine.ErrOutputsLimitExceeded):
		return InputCompletionStatus_OutputsLimitExceeded, nil
	case errors.Is(err, rollupsmachine.ErrCycleLimitExceeded):
		return InputCompletionStatus_CycleLimitExceeded, nil
	case errors.Is(err, rollupsmachine.ErrPayloadLengthLimitExceeded):
		return InputCompletionStatus_PayloadLengthLimitExceeded, nil
	case errors.Is(err, cartesimachine.ErrCartesiMachine),
		errors.Is(err, rollupsmachine.ErrProgress),
		errors.Is(err, rollupsmachine.ErrSoftYield):
		fallthrough
	default:
		return status, err
	}
}
