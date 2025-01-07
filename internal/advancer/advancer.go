// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cartesi/rollups-node/internal/advancer/machines"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/inspect"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
	"github.com/cartesi/rollups-node/pkg/service"
)

var (
	ErrInvalidMachines   = errors.New("machines must not be nil")
	ErrInvalidRepository = errors.New("repository must not be nil")

	ErrNoApp    = errors.New("no machine for application")
	ErrNoInputs = errors.New("no inputs")
)

type IAdvancerRepository interface {
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination) ([]*Input, error)
	StoreAdvanceResult(ctx context.Context, appID int64, ar *AdvanceResult) error
	UpdateEpochsInputsProcessed(ctx context.Context, nameOrAddress string) error
}

type IAdvancerMachines interface {
	GetAdvanceMachine(appId int64) (machines.AdvanceMachine, bool)
	UpdateMachines(ctx context.Context) error
	Apps() []*Application
}

type Service struct {
	service.Service
	repository     IAdvancerRepository
	machines       IAdvancerMachines
	inspector      *inspect.Inspector
	HTTPServer     *http.Server
	HTTPServerFunc func() error
}

type CreateInfo struct {
	service.CreateInfo
	AdvancerPollingInterval time.Duration
	PostgresEndpoint        config.Redacted[string]
	PostgresSslMode         bool
	Repository              repository.Repository
	MachineServerVerbosity  config.Redacted[cartesimachine.ServerVerbosity]
	Machines                *machines.Machines
	MaxStartupTime          time.Duration
	InspectAddress          string
	InspectServeMux         *http.ServeMux
}

func (c *CreateInfo) LoadEnv() {
	c.PostgresEndpoint.Value = config.GetPostgresEndpoint()
	c.PollInterval = config.GetAdvancerPollingInterval()
	c.MachineServerVerbosity.Value =
		cartesimachine.ServerVerbosity(config.GetMachineServerVerbosity())
	c.LogLevel = service.LogLevel(config.GetLogLevel())
	c.LogPretty = config.GetLogPrettyEnabled()
	c.MaxStartupTime = config.GetMaxStartupTime()
	c.InspectAddress = config.GetInspectAddress()
}

func Create(c *CreateInfo, s *Service) error {
	err := service.Create(&c.CreateInfo, &s.Service)
	if err != nil {
		return err
	}

	return service.WithTimeout(c.MaxStartupTime, func() error {
		if s.repository == nil {
			if c.Repository == nil {
				c.Repository, err = factory.NewRepositoryFromConnectionString(s.Context, c.PostgresEndpoint.Value)
				if err != nil {
					return err
				}
			}
			s.repository = c.Repository
		}

		if s.machines == nil {
			if c.Machines == nil {
				c.Machines, err = machines.Load(s.Context,
					c.Repository, c.MachineServerVerbosity.Value, s.Logger)
				if err != nil {
					return err
				}
			}
			s.machines = c.Machines
		}

		// allow partial construction for testing
		if c.Machines != nil {
			s.inspector, s.HTTPServer, s.HTTPServerFunc = inspect.NewInspector(
				c.Repository,
				c.Machines,
				c.InspectAddress,
				c.LogLevel,
				c.LogPretty,
			)
			go s.HTTPServerFunc()
		}
		return nil
	})
}

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

func (s *Service) Start(context context.Context, ready chan<- struct{}) error {
	ready <- struct{}{}
	return s.Serve()
}
func (v *Service) String() string {
	return v.Name
}

func getUnprocessedInputs(ctx context.Context, mr IAdvancerRepository, appAddress string) ([]*Input, error) {
	f := repository.InputFilter{Status: Pointer(InputCompletionStatus_None)}
	return mr.ListInputs(ctx, appAddress, f, repository.Pagination{})
}

// Step steps the Advancer for one processing cycle.
// It gets unprocessed inputs from the repository,
// runs them through the cartesi machine,
// and updates the repository with the outputs.
func (advancer *Service) Step(ctx context.Context) error {
	// Dynamically updates the list of machines
	err := advancer.machines.UpdateMachines(ctx)
	if err != nil {
		return err
	}

	apps := advancer.machines.Apps()

	// Updates the status of the epochs.
	for _, app := range apps {
		// Gets the unprocessed inputs (of all apps) from the repository.
		advancer.Logger.Debug("querying for unprocessed inputs")

		inputs, err := getUnprocessedInputs(ctx, advancer.repository, app.IApplicationAddress.String())
		if err != nil {
			return err
		}

		// Processes each set of inputs.
		advancer.Logger.Debug(fmt.Sprintf("processing %d input(s) from %s", len(inputs), app.Name))
		err = advancer.process(ctx, app, inputs)
		if err != nil {
			return err
		}

		err = advancer.repository.UpdateEpochsInputsProcessed(ctx, app.IApplicationAddress.String())
		if err != nil {
			return err
		}
	}

	return nil
}

// process sequentially processes inputs from the the application.
func (advancer *Service) process(ctx context.Context, app *Application, inputs []*Input) error {
	// Asserts that the app has an associated machine.
	machine, exists := advancer.machines.GetAdvanceMachine(app.ID)
	if !exists {
		return fmt.Errorf("%w %d", ErrNoApp, app.ID)
	}

	if len(inputs) <= 0 {
		return nil
	}

	// FIXME if theres a change in epoch id call update epochs
	for _, input := range inputs {
		advancer.Logger.Info("Processing input", "application", app.Name, "index", input.Index)

		// Sends the input to the cartesi machine.
		res, err := machine.Advance(ctx, input.RawData, input.Index)
		if err != nil {
			return err
		}

		// Stores the result in the database.
		err = advancer.repository.StoreAdvanceResult(ctx, input.EpochApplicationID, res)
		if err != nil {
			return err
		}
	}

	return nil
}
