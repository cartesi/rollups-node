// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/inspect"
	"github.com/cartesi/rollups-node/internal/manager"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
)

var (
	ErrInvalidMachines   = errors.New("machines must not be nil")
	ErrInvalidRepository = errors.New("repository must not be nil")

	ErrNoApp    = errors.New("no machine for application")
	ErrNoInputs = errors.New("no inputs")
)

// AdvancerRepository defines the repository interface needed by the Advancer service
type AdvancerRepository interface {
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination, descending bool) ([]*Input, uint64, error)
	GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error)
	StoreAdvanceResult(ctx context.Context, appID int64, ar *AdvanceResult) error
	UpdateEpochsInputsProcessed(ctx context.Context, nameOrAddress string) (int64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	UpdateInputSnapshotURI(ctx context.Context, appId int64, inputIndex uint64, snapshotURI string) error
	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
	GetLastProcessedInput(ctx context.Context, appAddress string) (*Input, error)
}

// Service is the main advancer service that processes inputs through Cartesi machines
type Service struct {
	service.Service
	snapshotsDir   string
	repository     AdvancerRepository
	machineManager manager.MachineProvider
	inspector      *inspect.Inspector
	HTTPServer     *http.Server
	HTTPServerFunc func() error
}

// CreateInfo contains the configuration for creating an advancer service
type CreateInfo struct {
	service.CreateInfo
	Config     config.AdvancerConfig
	Repository repository.Repository
}

// Create initializes a new advancer service
func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on advancer service Create is nil")
	}

	// Create the machine manager
	manager := manager.NewMachineManager(
		ctx,
		c.Repository,
		s.Logger,
		c.Config.FeatureMachineHashCheckEnabled,
	)
	s.machineManager = manager

	// Initialize the inspect service if enabled
	if c.Config.FeatureInspectEnabled {
		s.inspector, s.HTTPServer, s.HTTPServerFunc = inspect.NewInspector(
			c.Repository,
			manager,
			c.Config.InspectAddress,
			c.LogLevel,
			c.LogColor,
		)
	}

	s.snapshotsDir = c.Config.SnapshotsDir

	return s, nil
}

// Service interface implementation
func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }
func (s *Service) Tick() []error {
	if err := s.Step(s.Context); err != nil {
		return []error{err}
	}
	return []error{}
}
func (s *Service) Stop(b bool) []error {
	return nil
}
func (s *Service) Serve() error {
	if s.inspector != nil && s.HTTPServerFunc != nil {
		go s.HTTPServerFunc()
	}
	return s.Service.Serve()
}
func (s *Service) String() string {
	return s.Name
}

// getUnprocessedInputs retrieves inputs that haven't been processed yet
func getUnprocessedInputs(ctx context.Context, repo AdvancerRepository, appAddress string) ([]*Input, uint64, error) {
	f := repository.InputFilter{Status: Pointer(InputCompletionStatus_None)}
	return repo.ListInputs(ctx, appAddress, f, repository.Pagination{}, false)
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

		err := s.handleEpochSnapshotAfterInputProcessed(ctx, app)
		if err != nil {
			return err
		}

		// Get unprocessed inputs for this application
		s.Logger.Debug("Querying for unprocessed inputs", "application", app.Name)
		inputs, _, err := getUnprocessedInputs(ctx, s.repository, appAddress)
		if err != nil {
			return err
		}

		// Process the inputs
		s.Logger.Debug("Processing inputs", "application", app.Name, "count", len(inputs))
		err = s.processInputs(ctx, app, inputs)
		if err != nil {
			return err
		}

		// Update epochs to mark inputs as processed
		rows, err := s.repository.UpdateEpochsInputsProcessed(ctx, appAddress)
		if err != nil {
			return err
		}
		if rows > 0 {
			s.Logger.Info("Epochs updated to Inputs Processed", "application", app.Name, "count", rows)
		}
	}

	return nil
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
		result, err := machine.Advance(ctx, input.RawData, input.Index)
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

			return err
		}

		// Store the result in the database
		err = s.repository.StoreAdvanceResult(ctx, input.EpochApplicationID, result)
		if err != nil {
			s.Logger.Error("Failed to store advance result",
				"application", app.Name,
				"index", input.Index,
				"error", err)
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

// handleEpochSnapshotAfterInputProcessed handles the snapshot creation after when an epoch is closed after an input was processed
func (s *Service) handleEpochSnapshotAfterInputProcessed(ctx context.Context, app *Application) error {
	// Check if the application has a epoch snapshot policy
	if app.ExecutionParameters.SnapshotPolicy != SnapshotPolicy_EveryEpoch {
		return nil
	}

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

	if lastProcessedInput == nil {
		return nil
	}

	// Handle the snapshot
	return s.handleSnapshot(ctx, app, machine, lastProcessedInput)
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
		// Get the epoch for this input
		epoch, err := s.repository.GetEpoch(ctx, app.IApplicationAddress.String(), input.EpochIndex)
		if err != nil {
			return fmt.Errorf("failed to get epoch: %w", err)
		}

		// Skip if the epoch is still open
		if epoch.Status == EpochStatus_Open {
			return nil
		}

		// Check if this is the last input of the epoch
		lastInput, err := s.repository.GetLastInput(ctx, app.IApplicationAddress.String(), input.EpochIndex)
		if err != nil {
			return fmt.Errorf("failed to get last input: %w", err)
		}

		// If this is the last input and the epoch is closed, create a snapshot
		if lastInput != nil && lastInput.Index == input.Index {
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
	snapshotPath := path.Join(s.snapshotsDir, snapshotName)

	s.Logger.Info("Creating snapshot",
		"application", app.Name,
		"epoch", input.EpochIndex,
		"input", input.Index,
		"path", snapshotPath)

	// Ensure the parent directory exists
	if _, err := os.Stat(s.snapshotsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(s.snapshotsDir, 0755); err != nil { // nolint: mnd
			return fmt.Errorf("failed to create snapshots directory: %w", err)
		}
	}

	// Remove previous snapshot if it exists
	previousSnapshot, err := s.repository.GetLastSnapshot(ctx, app.IApplicationAddress.String())
	if err != nil {
		s.Logger.Error("Failed to get previous snapshot",
			"application", app.Name,
			"error", err)
		// Continue even if we can't get the previous snapshot
	}

	// Create the snapshot
	err = machine.CreateSnapshot(ctx, input.Index+1, snapshotPath)
	if err != nil {
		return err
	}

	// Update the input record with the snapshot URI
	input.SnapshotURI = &snapshotPath

	// Update the input's snapshot URI in the database
	err = s.repository.UpdateInputSnapshotURI(ctx, input.EpochApplicationID, input.Index, snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to update input snapshot URI: %w", err)
	}

	// Remove previous snapshot if it exists
	if previousSnapshot != nil && previousSnapshot.Index != input.Index && previousSnapshot.SnapshotURI != nil {
		// Only remove if it's a different snapshot than the one we just created
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
	// Safety check: ensure the path contains the application name and is in the snapshots directory
	if !strings.HasPrefix(snapshotPath, s.snapshotsDir) || !strings.Contains(snapshotPath, appName) {
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
