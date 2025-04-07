// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination) ([]*Input, uint64, error)
	StoreAdvanceResult(ctx context.Context, appID int64, ar *AdvanceResult) error
	UpdateEpochsInputsProcessed(ctx context.Context, nameOrAddress string) (int64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
}

// Service is the main advancer service that processes inputs through Cartesi machines
type Service struct {
	service.Service
	repository     AdvancerRepository
	machineManager manager.MachineProvider
	inspector      *inspect.Inspector
	HTTPServer     *http.Server
	HTTPServerFunc func() error
}

// CreateInfo contains the configuration for creating an advancer service
type CreateInfo struct {
	service.CreateInfo
	Config     config.Config
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
		c.Config.RemoteMachineLogLevel,
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
	return repo.ListInputs(ctx, appAddress, f, repository.Pagination{})
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
	}

	return nil
}
