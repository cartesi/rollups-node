// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machines

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"

	nm "github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/cartesi/rollups-node/pkg/emulator"
	rm "github.com/cartesi/rollups-node/pkg/rollupsmachine"
	cm "github.com/cartesi/rollups-node/pkg/rollupsmachine/cartesimachine"
)

type MachinesRepository interface {
	// GetMachineConfigurations retrieves a machine configuration for each application.
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination) ([]*Application, error)

	// GetProcessedInputs retrieves the processed inputs of an application with indexes greater or
	// equal to the given input index.
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination) ([]*Input, error)
}

// AdvanceMachine masks nodemachine.NodeMachine to only expose methods required by the Advancer.
type AdvanceMachine interface {
	Advance(_ context.Context, input []byte, index uint64) (*AdvanceResult, error)
}

// InspectMachine masks nodemachine.NodeMachine to only expose methods required by the Inspector.
type InspectMachine interface {
	Inspect(_ context.Context, query []byte) (*InspectResult, error)
}

// Machines is a thread-safe type that manages the pool of cartesi machines being used by the node.
// It contains a map of applications to machines.
type Machines struct {
	mutex      sync.RWMutex
	machines   map[int64]*nm.NodeMachine
	repository MachinesRepository
	verbosity  cm.ServerVerbosity
	Logger     *slog.Logger
}

func getAllRunningApplications(ctx context.Context, mr MachinesRepository) ([]*Application, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled)}
	return mr.ListApplications(ctx, f, repository.Pagination{})
}

// Load initializes the cartesi machines.
// Load advances a machine to the last processed input stored in the database.
//
// Load does not fail when one of those machines fail to initialize.
// It stores the error to be returned later and continues to initialize the other machines.
func Load(
	ctx context.Context,
	repo MachinesRepository,
	verbosity cm.ServerVerbosity,
	logger *slog.Logger,
) (*Machines, error) {
	apps, err := getAllRunningApplications(ctx, repo)
	if err != nil {
		return nil, err
	}

	machines := map[int64]*nm.NodeMachine{}
	var errs error

	for _, app := range apps {
		// Creates the machine.
		machine, err := createMachine(ctx, verbosity, app, logger)
		if err != nil {
			err = fmt.Errorf("failed to create machine from snapshot %s (%s): %w", app.TemplateURI, app.Name, err)
			errs = errors.Join(errs, err)
			continue
		}

		// Advances the machine until it catches up with the state of the database (if necessary).
		err = catchUp(ctx, repo, app, machine, logger)
		if err != nil {
			err = fmt.Errorf("failed to advance cartesi machine (%v): %w", app, err)
			errs = errors.Join(errs, err, machine.Close())
			continue
		}

		machines[app.ID] = machine
	}

	return &Machines{
		machines:   machines,
		repository: repo,
		verbosity:  verbosity,
		Logger:     logger,
	}, errs
}

func (m *Machines) UpdateMachines(ctx context.Context) error {
	apps, err := getAllRunningApplications(ctx, m.repository)
	if err != nil {
		return err
	}

	for _, app := range apps {
		if m.Exists(app.ID) {
			continue
		}

		machine, err := createMachine(ctx, m.verbosity, app, m.Logger)
		if err != nil {
			m.Logger.Error("Failed to create machine", "application", app.IApplicationAddress, "error", err)
			continue
		}

		err = catchUp(ctx, m.repository, app, machine, m.Logger)
		if err != nil {
			m.Logger.Error("Failed to sync the machine", "application", app.IApplicationAddress, "error", err)
			machine.Close()
			continue
		}

		m.Add(app.ID, machine)
	}

	m.RemoveAbsent(apps)

	return nil
}

// GetAdvanceMachine gets the machine associated with the application from the map.
func (m *Machines) GetAdvanceMachine(appId int64) (AdvanceMachine, bool) {
	return m.getMachine(appId)
}

// GetInspectMachine gets the machine associated with the application from the map.
func (m *Machines) GetInspectMachine(appId int64) (InspectMachine, bool) {
	return m.getMachine(appId)
}

// Add maps a new application to a machine.
// It does nothing if the application is already mapped to some machine.
// It returns true if it was able to add the machine and false otherwise.
func (m *Machines) Add(appId int64, machine *nm.NodeMachine) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.machines[appId]; ok {
		return false
	} else {
		m.machines[appId] = machine
		return true
	}
}

func (m *Machines) Exists(appId int64) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	_, exists := m.machines[appId]
	return exists
}

func (m *Machines) RemoveAbsent(apps []*Application) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	configMap := make(map[int64]bool)
	for _, app := range apps {
		configMap[app.ID] = true
	}
	for id, machine := range m.machines {
		if !configMap[id] {
			m.Logger.Info("Application was disabled, shutting down machine", "application", machine.Application.Name)
			machine.Close()
			delete(m.machines, id)
		}
	}
}

// Delete deletes an application from the map.
// It returns the associated machine, if any.
func (m *Machines) Delete(appId int64) *nm.NodeMachine {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if machine, ok := m.machines[appId]; ok {
		return nil
	} else {
		delete(m.machines, appId)
		return machine
	}
}

// Apps returns the addresses of the applications for which there are machines.
func (m *Machines) Apps() []*Application {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]*Application, len(m.machines))
	i := 0
	for _, v := range m.machines {
		keys[i] = v.Application
		i++
	}
	return keys
}

// Close closes all the machines and erases them from the map.
func (m *Machines) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	err := closeMachines(m.machines)
	if err != nil {
		m.Logger.Error(fmt.Sprintf("failed to close some machines: %v", err))
	}
	return err
}

// ------------------------------------------------------------------------------------------------

func (m *Machines) getMachine(appId int64) (*nm.NodeMachine, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	machine, exists := m.machines[appId]
	return machine, exists
}

func closeMachines(machines map[int64]*nm.NodeMachine) (err error) {
	for _, machine := range machines {
		err = errors.Join(err, machine.Close())
	}
	for app := range machines {
		delete(machines, app)
	}
	return
}

func createMachine(ctx context.Context,
	verbosity cm.ServerVerbosity,
	app *Application,
	logger *slog.Logger,
) (*nm.NodeMachine, error) {
	appAddress := app.IApplicationAddress.String()
	logger.Info("creating machine", "application", app.Name, "address", appAddress,
		"template-path", app.TemplateURI)
	logger.Debug("instantiating remote machine server", "application", app.Name, "address", appAddress)
	// Starts the server.
	address, err := cm.StartServer(logger, verbosity, 0, os.Stdout, os.Stderr)
	if err != nil {
		return nil, err
	}

	logger.Info("loading machine on server", "application", app.Name, "address", appAddress,
		"remote-machine", address, "template-path", app.TemplateURI)
	// Creates a CartesiMachine from the snapshot.
	runtimeConfig := &emulator.MachineRuntimeConfig{}
	cartesiMachine, err := cm.Load(ctx, app.TemplateURI, address, runtimeConfig)
	if err != nil {
		return nil, errors.Join(err, cm.StopServer(address, logger))
	}

	logger.Debug("machine loaded on server", "application", app.Name, "address", appAddress,
		"remote-machine", address, "template-path", app.TemplateURI)

	// Creates a RollupsMachine from the CartesiMachine.
	rollupsMachine, err := rm.New(ctx,
		cartesiMachine,
		app.ExecutionParameters.AdvanceIncCycles,
		app.ExecutionParameters.AdvanceMaxCycles,
		logger,
	)
	if err != nil {
		return nil, errors.Join(err, cartesiMachine.Close(ctx))
	}

	// Creates a NodeMachine from the RollupsMachine.
	nodeMachine, err := nm.NewNodeMachine(
		app,
		rollupsMachine,
		0,
		app.ExecutionParameters.AdvanceMaxDeadline,
		app.ExecutionParameters.InspectMaxDeadline,
		app.ExecutionParameters.MaxConcurrentInspects)
	if err != nil {
		return nil, errors.Join(err, rollupsMachine.Close(ctx))
	}

	return nodeMachine, err
}

func getProcessedInputs(ctx context.Context, mr MachinesRepository, appAddress string, index uint64) ([]*Input, error) {
	f := repository.InputFilter{InputIndex: Pointer(index), NotStatus: Pointer(InputCompletionStatus_None)}
	return mr.ListInputs(ctx, appAddress, f, repository.Pagination{})
}

func catchUp(ctx context.Context,
	repo MachinesRepository,
	app *Application,
	machine *nm.NodeMachine,
	logger *slog.Logger,
) error {
	appAddress := app.IApplicationAddress.String()
	logger.Info("catching up processed inputs",
		"application", app.Name,
		"address", appAddress,
		"processed_inputs", app.ProcessedInputs,
	)

	inputs, err := getProcessedInputs(ctx, repo, appAddress, 0)
	if err != nil {
		return err
	}

	if uint64(len(inputs)) != app.ProcessedInputs {
		errorMsg := fmt.Sprintf("processed inputs do not match: expected %d, got %d", len(inputs), app.ProcessedInputs)
		logger.Error(errorMsg, "application", app.Name, "address", appAddress)
		return errors.New(errorMsg)
	}

	for _, input := range inputs {
		logger.Info("advancing", "application", app.Name, "address", appAddress,
			"epochIndex", input.EpochIndex, "input_index", input.Index)
		_, err := machine.Advance(ctx, input.RawData, input.Index)
		if err != nil {
			return err
		}
	}

	return nil
}
