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
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)

	// ListInputs retrieves inputs based on filter criteria
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination, descending bool) ([]*Input, uint64, error)

	// GetLastSnapshot retrieves the most recent input with a snapshot for the given application
	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
}

// MachineManager manages the lifecycle of machine instances for applications
type MachineManager struct {
	mutex      sync.RWMutex
	machines   map[int64]MachineInstance
	repository MachineRepository
	checkHash  bool
	logger     *slog.Logger
}

// NewMachineManager creates a new machine manager
func NewMachineManager(
	ctx context.Context,
	repo MachineRepository,
	logger *slog.Logger,
	checkHash bool,
) *MachineManager {
	return &MachineManager{
		machines:   map[int64]MachineInstance{},
		repository: repo,
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

		// Check if we have a snapshot to load from
		var instance MachineInstance
		var err error

		// Find the latest snapshot for this application
		snapshot, err := m.repository.GetLastSnapshot(ctx, app.IApplicationAddress.String())
		if err != nil {
			m.logger.Error("Failed to find latest snapshot",
				"application", app.Name,
				"error", err)
			// Continue with template-based initialization
		}

		if snapshot != nil && snapshot.SnapshotURI != nil {
			// Create a machine instance from the snapshot
			m.logger.Info("Creating machine instance from snapshot",
				"application", app.Name,
				"snapshot", *snapshot.SnapshotURI)

			// Verify the snapshot path exists
			if _, err := os.Stat(*snapshot.SnapshotURI); os.IsNotExist(err) {
				m.logger.Error("Snapshot path does not exist",
					"application", app.Name,
					"snapshot", *snapshot.SnapshotURI,
					"error", err)
				// Fall back to template-based initialization
			} else {
				// Create a factory with the snapshot path and machine hash
				instance, err = NewMachineInstanceFromSnapshot(
					ctx, app, m.logger, m.checkHash, *snapshot.SnapshotURI, snapshot.MachineHash, snapshot.Index)

				if err != nil {
					m.logger.Error("Failed to create machine instance from snapshot",
						"application", app.Name,
						"snapshot", *snapshot.SnapshotURI,
						"error", err)
					// Fall back to template-based initialization
				} else {
					// If we loaded from a snapshot, we need to synchronize from the snapshot point
					// Get the inputs after the snapshot
					inputsAfterSnapshot, err := getInputsAfterSnapshot(ctx, m.repository, app, snapshot.Index)
					if err != nil {
						m.logger.Error("Failed to get inputs after snapshot",
							"application", app.Name,
							"snapshot_input_index", snapshot.Index,
							"error", err)
						instance.Close()
						continue
					}

					// Process each input to bring the machine to the current state
					for _, input := range inputsAfterSnapshot {
						m.logger.Info("Replaying input after snapshot",
							"application", app.Name,
							"epoch_index", input.EpochIndex,
							"input_index", input.Index)

						_, err := instance.Advance(ctx, input.RawData, input.Index)
						if err != nil {
							m.logger.Error("Failed to replay input after snapshot",
								"application", app.Name,
								"input_index", input.Index,
								"error", err)
							instance.Close()
							continue
						}
					}

					// Add the machine to the manager
					m.addMachine(app.ID, instance)
					continue
				}
			}
		}

		// If we didn't load from a snapshot, create a new machine instance from the template
		instance, err = NewMachineInstance(ctx, app, m.logger, m.checkHash)
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
	return repo.ListApplications(ctx, f, repository.Pagination{}, false)
}

// Helper function to get processed inputs
func getProcessedInputs(ctx context.Context, repo MachineRepository, appAddress string) ([]*Input, uint64, error) {
	f := repository.InputFilter{NotStatus: Pointer(InputCompletionStatus_None)}
	return repo.ListInputs(ctx, appAddress, f, repository.Pagination{}, false)
}

// Helper function to get inputs after a specific index
func getInputsAfterSnapshot(ctx context.Context, repo MachineRepository, app *Application, snapshotInputIndex uint64) ([]*Input, error) {
	// Get all processed inputs for this application
	inputs, _, err := getProcessedInputs(ctx, repo, app.IApplicationAddress.String())
	if err != nil {
		return nil, err
	}

	// Filter inputs to only include those after the snapshot
	for i, input := range inputs {
		if input.Index > snapshotInputIndex {
			return inputs[i:], nil
		}
	}
	return []*Input{}, nil
}
