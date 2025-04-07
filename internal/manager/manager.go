// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

var (
	ErrApplicationNotFound    = errors.New("application not found")
	ErrMachineCreation        = errors.New("failed to create machine")
	ErrMachineSynchronization = errors.New("failed to synchronize machine")
)

// MachineRepository defines the repository interface needed by the MachineManager
type MachineRepository interface {
	// ListApplications retrieves applications based on filter criteria
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination) ([]*Application, uint64, error)

	// ListInputs retrieves inputs based on filter criteria
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination) ([]*Input, uint64, error)
}

// MachineManager manages the lifecycle of machine instances for applications
type MachineManager struct {
	mutex      sync.RWMutex
	machines   map[int64]MachineInstance
	repository MachineRepository
	verbosity  MachineLogLevel
	checkHash  bool
	logger     *slog.Logger
}

// NewMachineManager creates a new machine manager
func NewMachineManager(
	ctx context.Context,
	repo MachineRepository,
	verbosity MachineLogLevel,
	logger *slog.Logger,
	checkHash bool,
) *MachineManager {
	return &MachineManager{
		machines:   map[int64]MachineInstance{},
		repository: repo,
		verbosity:  verbosity,
		checkHash:  checkHash,
		logger:     logger,
	}
}

// UpdateMachines refreshes the list of machines based on enabled applications
func (m *MachineManager) UpdateMachines(ctx context.Context) error {
	// Get all enabled applications
	apps, _, err := getEnabledApplications(ctx, m.repository)
	if err != nil {
		return err
	}

	// Create machines for new applications
	for _, app := range apps {
		if m.HasMachine(app.ID) {
			continue
		}

		m.logger.Info("Creating new machine instance",
			"application", app.Name,
			"address", app.IApplicationAddress.String())

		// Create a new machine instance
		instance, err := NewMachineInstance(ctx, m.verbosity, app, m.logger, m.checkHash)
		if err != nil {
			m.logger.Error("Failed to create machine instance",
				"application", app.IApplicationAddress,
				"error", err)
			continue
		}

		// Synchronize the machine with processed inputs
		err = instance.Synchronize(ctx, m.repository)
		if err != nil {
			m.logger.Error("Failed to synchronize machine",
				"application", app.IApplicationAddress,
				"error", err)
			instance.Close()
			continue
		}

		// Add the machine to the manager
		m.addMachine(app.ID, instance)
	}

	// Remove machines for disabled applications
	m.removeMachines(apps)

	return nil
}

// GetMachine retrieves a machine instance for an application
func (m *MachineManager) GetMachine(appID int64) (MachineInstance, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	machine, exists := m.machines[appID]
	return machine, exists
}

// HasMachine checks if a machine exists for an application
func (m *MachineManager) HasMachine(appID int64) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	_, exists := m.machines[appID]
	return exists
}

// AddMachine adds a machine to the manager
func (m *MachineManager) addMachine(appID int64, machine MachineInstance) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.machines[appID]; exists {
		return false
	}

	m.machines[appID] = machine
	return true
}

// RemoveMachines removes machines for applications not in the provided list
func (m *MachineManager) removeMachines(apps []*Application) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Create a map of active application IDs
	activeApps := make(map[int64]struct{})
	for _, app := range apps {
		activeApps[app.ID] = struct{}{}
	}

	// Remove machines for applications not in the active list
	for id, machine := range m.machines {
		if _, present := activeApps[id]; !present {
			if m.logger != nil {
				m.logger.Info("Application was disabled, shutting down machine",
					"application", machine.Application().Name)
			}
			machine.Close()
			delete(m.machines, id)
		}
	}
}

// Applications returns the list of applications with active machines
func (m *MachineManager) Applications() []*Application {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	apps := make([]*Application, 0, len(m.machines))
	for _, machine := range m.machines {
		apps = append(apps, machine.Application())
	}
	return apps
}

// Close shuts down all machine instances
func (m *MachineManager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errs []error
	for id, machine := range m.machines {
		if err := machine.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close machine for app %d: %w", id, err))
		}
		delete(m.machines, id)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Helper function to get enabled applications
func getEnabledApplications(ctx context.Context, repo MachineRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled)}
	return repo.ListApplications(ctx, f, repository.Pagination{})
}

// Helper function to get processed inputs
func getProcessedInputs(ctx context.Context, repo MachineRepository, appAddress string) ([]*Input, uint64, error) {
	f := repository.InputFilter{NotStatus: Pointer(InputCompletionStatus_None)}
	return repo.ListInputs(ctx, appAddress, f, repository.Pagination{})
}
