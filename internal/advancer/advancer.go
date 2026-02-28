// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

var (
	ErrInvalidMachines   = errors.New("machines must not be nil")
	ErrInvalidRepository = errors.New("repository must not be nil")

	ErrNoApp    = errors.New("no machine for application")
	ErrNoInputs = errors.New("no inputs")
)

// AdvancerRepository defines the repository interface needed by the Advancer service
type AdvancerRepository interface {
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination, descending bool) ([]*Input, uint64, error)
	GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error)
	StoreAdvanceResult(ctx context.Context, appID int64, ar *AdvanceResult) error
	UpdateEpochInputsProcessed(ctx context.Context, nameOrAddress string, epochIndex uint64) error
	UpdateEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64, proof *OutputsProof) error
	RepeatPreviousEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64) error
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	UpdateInputSnapshotURI(ctx context.Context, appId int64, inputIndex uint64, snapshotURI string) error
	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
	GetLastProcessedInput(ctx context.Context, appAddress string) (*Input, error)
}

func getUnprocessedEpochs(ctx context.Context, er AdvancerRepository, address string) ([]*Epoch, uint64, error) {
	f := repository.EpochFilter{Status: []EpochStatus{EpochStatus_Open, EpochStatus_Closed}}
	return er.ListEpochs(ctx, address, f, repository.Pagination{}, false)
}

// getUnprocessedInputs retrieves inputs that haven't been processed yet with pagination support.
func getUnprocessedInputs(
	ctx context.Context,
	repo AdvancerRepository,
	appAddress string,
	epochIndex uint64,
	batchSize uint64,
) ([]*Input, uint64, error) {
	f := repository.InputFilter{Status: Pointer(InputCompletionStatus_None), EpochIndex: &epochIndex}
	return repo.ListInputs(ctx, appAddress, f, repository.Pagination{Limit: batchSize}, false)
}

// Step performs one processing cycle of the advancer
// It updates machines, gets unprocessed inputs, processes them, and updates epochs
func (s *Service) Step(ctx context.Context) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	// Update the machine manager with any new or disabled applications
	err := s.machineManager.UpdateMachines(ctx)
	if err != nil {
		return err
	}

	// Get all applications with active machines
	apps := s.machineManager.Applications()

	// Process inputs for each application
	for _, app := range apps {
		appAddress := app.IApplicationAddress.String()

		epochs, _, err := getUnprocessedEpochs(ctx, s.repository, appAddress)
		if err != nil {
			return err
		}

		for _, epoch := range epochs {
			if err := s.processEpochInputs(ctx, app, epoch.Index); err != nil {
				return err
			}

			if epoch.Status == EpochStatus_Closed {
				if allProcessed, perr := s.isAllEpochInputsProcessed(app, epoch); perr == nil && allProcessed {
					err := s.handleEpochAfterInputsProcessed(ctx, app, epoch)
					if err != nil {
						return err
					}

					// Update epochs to mark inputs as processed
					err = s.repository.UpdateEpochInputsProcessed(ctx, appAddress, epoch.Index)
					if err != nil {
						return err
					}
					s.Logger.Info("Epoch updated to Inputs Processed", "application", app.Name, "epoch_index", epoch.Index)
				} else if perr != nil {
					return perr
				} else {
					break // some inputs were not processed yet, check next time
				}
			}
		}
	}

	return nil
}

// processEpochInputs fetches and processes unprocessed inputs for an epoch in batches.
// Processed inputs change status and drop out of the filter, so each batch fetches from offset 0.
func (s *Service) processEpochInputs(ctx context.Context, app *Application, epochIndex uint64) error {
	appAddress := app.IApplicationAddress.String()
	for {
		inputs, _, err := getUnprocessedInputs(ctx, s.repository, appAddress, epochIndex, s.inputBatchSize)
		if err != nil {
			return err
		}
		if len(inputs) == 0 {
			return nil
		}
		s.Logger.Debug("Processing inputs",
			"application", app.Name, "epoch_index", epochIndex, "count", len(inputs))
		if err := s.processInputs(ctx, app, inputs); err != nil {
			return err
		}
	}
}

func (s *Service) isAllEpochInputsProcessed(app *Application, epoch *Epoch) (bool, error) {
	// epoch has no inputs
	if epoch.InputIndexLowerBound == epoch.InputIndexUpperBound {
		return true, nil
	}
	machine, exists := s.machineManager.GetMachine(app.ID)
	if !exists {
		return false, fmt.Errorf("%w: %d", ErrNoApp, app.ID)
	}
	if machine.ProcessedInputs() == epoch.InputIndexUpperBound {
		return true, nil
	}
	return false, nil
}

// processInputs handles the processing of inputs for an application
func (s *Service) processInputs(ctx context.Context, app *Application, inputs []*Input) error {
	// Skip if there are no inputs to process
	if len(inputs) == 0 {
		return nil
	}

	// Get the machine instance for this application
	machine, exists := s.machineManager.GetMachine(app.ID)
	if !exists {
		return fmt.Errorf("%w: %d", ErrNoApp, app.ID)
	}

	// Process each input sequentially
	for _, input := range inputs {
		// Check for context cancellation before processing each input
		if err := ctx.Err(); err != nil {
			return err
		}

		s.Logger.Info("Processing input",
			"application", app.Name,
			"epoch", input.EpochIndex,
			"index", input.Index)

		// Advance the machine with this input
		result, err := machine.Advance(ctx, input.RawData, input.EpochIndex, input.Index, app.IsDaveConsensus())
		if err != nil {
			// If there's an error, mark the application as inoperable
			s.Logger.Error("Error executing advance",
				"application", app.Name,
				"index", input.Index,
				"error", err)

			// If the error is due to context cancellation, don't mark as inoperable
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}

			reason := err.Error()
			updateErr := s.repository.UpdateApplicationState(ctx, app.ID, ApplicationState_Inoperable, &reason)
			if updateErr != nil {
				s.Logger.Error("Failed to update application state",
					"application", app.Name,
					"error", updateErr)
			}

			// Eagerly close the machine to release the child process.
			// The app is already inoperable, so no further operations will succeed.
			// Skip if the runtime was already destroyed inside the manager.
			if !errors.Is(err, manager.ErrMachineClosed) {
				if closeErr := machine.Close(); closeErr != nil {
					s.Logger.Warn("Failed to close machine after advance error",
						"application", app.Name,
						"error", closeErr)
				}
			}

			return err
		}
		// log advance result hashes
		s.Logger.Info("Processing input finished",
			"application", app.Name,
			"epoch", result.EpochIndex,
			"index", result.InputIndex,
			"status", result.Status,
			"outputs", len(result.Outputs),
			"reports", len(result.Reports),
			"hashes", len(result.Hashes),
			"remaining_cycles", result.RemainingMetaCycles,
		)

		// Store the result in the database
		err = s.repository.StoreAdvanceResult(ctx, input.EpochApplicationID, result)
		if err != nil {
			// Machine state is now ahead of the database. This desync is
			// unrecoverable without a restart — regardless of whether the
			// failure was a DB error or a context timeout. Shut down the
			// node so it can restart cleanly from the last snapshot.
			s.Logger.Error(
				"FATAL: failed to store advance result after machine state "+
					"was already updated — shutting down to prevent permanent desync",
				"application", app.Name,
				"epoch", input.EpochIndex,
				"index", input.Index,
				"error", err)
			s.Cancel() // triggers graceful shutdown of all services
			return err
		}

		// Create a snapshot if needed
		if result.Status == InputCompletionStatus_Accepted {
			err = s.handleSnapshot(ctx, app, machine, input)
			if err != nil {
				s.Logger.Error("Failed to create snapshot",
					"application", app.Name,
					"index", input.Index,
					"error", err)
				// Continue processing even if snapshot creation fails
			}
		}
	}

	return nil
}

func (s *Service) isEpochLastInput(ctx context.Context, app *Application, input *Input) (bool, error) {
	if app == nil || input == nil {
		return false, fmt.Errorf("application and input must not be nil")
	}
	// Get the epoch for this input
	epoch, err := s.repository.GetEpoch(ctx, app.IApplicationAddress.String(), input.EpochIndex)
	if err != nil {
		return false, fmt.Errorf("failed to get epoch: %w", err)
	}

	// Skip if the epoch is still open
	if epoch.Status == EpochStatus_Open {
		return false, nil
	}

	// Check if this is the last input of the epoch
	lastInput, err := s.repository.GetLastInput(ctx, app.IApplicationAddress.String(), input.EpochIndex)
	if err != nil {
		return false, fmt.Errorf("failed to get last input: %w", err)
	}

	// If this is the last input and the epoch is closed, return true
	if lastInput != nil && lastInput.Index == input.Index {
		return true, nil
	}

	return false, nil
}

// handleEpochAfterInputsProcessed handles the snapshot creation after when an epoch is closed after an input was processed
func (s *Service) handleEpochAfterInputsProcessed(ctx context.Context, app *Application, epoch *Epoch) error {
	// if epoch has inputs, all data is updated after advance, just check for snapshot
	if epoch.InputIndexLowerBound != epoch.InputIndexUpperBound {
		// Get the machine instance for this application
		machine, exists := s.machineManager.GetMachine(app.ID)
		if !exists {
			return fmt.Errorf("%w: %d", ErrNoApp, app.ID)
		}

		// Check if this is the last processed input
		lastProcessedInput, err := s.repository.GetLastProcessedInput(ctx, app.IApplicationAddress.String())
		if err != nil {
			return fmt.Errorf("failed to get last input: %w", err)
		}

		// Check if the application has a epoch snapshot policy
		if lastProcessedInput != nil && app.ExecutionParameters.SnapshotPolicy == SnapshotPolicy_EveryEpoch {
			// Handle the snapshot
			return s.handleSnapshot(ctx, app, machine, lastProcessedInput)
		}

		return nil
	}

	// if epoch has no inputs, we need to copy previous epoch Outputs Proof
	// first epoch we need to get it from the template
	if epoch.Index == 0 {
		// Get the machine instance for this application
		machine, exists := s.machineManager.GetMachine(app.ID)
		if !exists {
			return fmt.Errorf("%w: %d", ErrNoApp, app.ID)
		}
		outputsProof, err := machine.OutputsProof(ctx)
		if err != nil {
			// If the runtime was destroyed (e.g., child process crashed),
			// mark the app inoperable to avoid an infinite retry loop.
			if errors.Is(err, manager.ErrMachineClosed) {
				reason := err.Error()
				_ = s.repository.UpdateApplicationState(ctx, app.ID,
					ApplicationState_Inoperable, &reason)
			}
			return fmt.Errorf("failed to get outputs proof from machine: %w", err)
		}
		err = s.repository.UpdateEpochOutputsProof(ctx, app.ID, epoch.Index, outputsProof)
		if err != nil {
			return fmt.Errorf("failed to store outputs proof for epoch 0: %w", err)
		}
	} else {
		err := s.repository.RepeatPreviousEpochOutputsProof(ctx, app.ID, epoch.Index)
		if err != nil {
			return fmt.Errorf("failed to repeat previous epoch outputs proof: %w", err)
		}
	}

	return nil
}

// handleSnapshot creates a snapshot based on the application's snapshot policy
func (s *Service) handleSnapshot(ctx context.Context, app *Application, machine manager.MachineInstance, input *Input) error {
	policy := app.ExecutionParameters.SnapshotPolicy

	// Skip if snapshot policy is NONE
	if policy == SnapshotPolicy_None {
		return nil
	}

	// For EVERY_INPUT policy, create a snapshot for every input
	if policy == SnapshotPolicy_EveryInput {
		return s.createSnapshot(ctx, app, machine, input)
	}

	// For EVERY_EPOCH policy, check if this is the last input of the epoch
	if policy == SnapshotPolicy_EveryEpoch {
		// If this is the last input and the epoch is closed, create a snapshot
		isLastInput, err := s.isEpochLastInput(ctx, app, input)
		if err != nil {
			return err
		}
		if isLastInput {
			return s.createSnapshot(ctx, app, machine, input)
		}
	}

	return nil
}

// createSnapshot creates a snapshot and updates the input record with the snapshot URI
func (s *Service) createSnapshot(ctx context.Context, app *Application, machine manager.MachineInstance, input *Input) error {
	if input.SnapshotURI != nil {
		s.Logger.Debug("Skipping snapshot, input already has a snapshot",
			"application", app.Name,
			"epoch", input.EpochIndex,
			"input", input.Index,
			"path", *input.SnapshotURI)
		return nil
	}

	// Generate a snapshot path with a simpler structure
	// Use app name and input index only, avoiding deep directory nesting
	snapshotName := fmt.Sprintf("%s_epoch%d_input%d", app.Name, input.EpochIndex, input.Index)
	snapshotPath := filepath.Join(s.snapshotsDir, snapshotName)

	s.Logger.Info("Creating snapshot",
		"application", app.Name,
		"epoch", input.EpochIndex,
		"input", input.Index,
		"path", snapshotPath)

	// Ensure the parent directory exists
	if err := os.MkdirAll(s.snapshotsDir, 0755); err != nil { //nolint: mnd
		return fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	// Create the snapshot
	err := machine.CreateSnapshot(ctx, input.Index+1, snapshotPath)
	if err != nil {
		return err
	}

	// Get previous snapshot BEFORE writing the new one so the query does not
	// return the snapshot we just created — that would cause self-deletion.
	previousSnapshot, err := s.repository.GetLastSnapshot(ctx, app.IApplicationAddress.String())
	if err != nil {
		s.Logger.Error("Failed to get previous snapshot",
			"application", app.Name,
			"error", err)
		// Continue even if we can't get the previous snapshot
	}

	// Update the input record with the snapshot URI
	input.SnapshotURI = &snapshotPath

	// Update the input's snapshot URI in the database
	err = s.repository.UpdateInputSnapshotURI(ctx, input.EpochApplicationID, input.Index, snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to update input snapshot URI: %w", err)
	}

	// Remove previous snapshot if it exists
	if previousSnapshot != nil && previousSnapshot.SnapshotURI != nil {
		if err := s.removeSnapshot(*previousSnapshot.SnapshotURI, app.Name); err != nil {
			s.Logger.Error("Failed to remove previous snapshot",
				"application", app.Name,
				"snapshot", *previousSnapshot.SnapshotURI,
				"error", err)
			// Continue even if we can't remove the previous snapshot
		}
	}

	return nil
}

// removeSnapshot safely removes a previous snapshot
func (s *Service) removeSnapshot(snapshotPath string, appName string) error {
	// Safety check: canonicalize paths to prevent directory traversal via ".." sequences
	cleanPath := filepath.Clean(snapshotPath)
	cleanDir := filepath.Clean(s.snapshotsDir)
	if !strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) ||
		!strings.HasPrefix(filepath.Base(cleanPath), appName+"_") {
		return fmt.Errorf("invalid snapshot path: %s", snapshotPath)
	}

	s.Logger.Debug("Removing previous snapshot", "application", appName, "path", snapshotPath)

	// Check if the path exists before attempting to remove it
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		s.Logger.Warn("Snapshot path does not exist, nothing to remove",
			"application", appName,
			"path", snapshotPath)
		return nil
	}

	// Use os.RemoveAll to remove the snapshot directory or file
	return os.RemoveAll(snapshotPath)
}
