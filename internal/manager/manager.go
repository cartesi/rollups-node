// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
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
	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)

	// ListInputs retrieves inputs based on filter criteria
	ListInputs(ctx context.Context, nameOrAddress string, f repository.InputFilter, p repository.Pagination, descending bool) ([]*Input, uint64, error)

	// GetLastSnapshot retrieves the most recent input with a snapshot for the given application
	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
}

// MachineInstanceFactory creates MachineInstance values from applications.
// Implementations decide whether to load from a template or snapshot.
type MachineInstanceFactory interface {
	NewFromTemplate(ctx context.Context, app *Application, logger *slog.Logger, checkHash bool) (MachineInstance, error)
	NewFromSnapshot(ctx context.Context, app *Application, logger *slog.Logger, checkHash bool,
		snapshotPath string, machineHash *common.Hash, inputIndex uint64) (MachineInstance, error)
}

// DefaultMachineInstanceFactory delegates to NewMachineInstance / NewMachineInstanceFromSnapshot.
type DefaultMachineInstanceFactory struct{}

func (f *DefaultMachineInstanceFactory) NewFromTemplate(
	ctx context.Context, app *Application, logger *slog.Logger, checkHash bool,
) (MachineInstance, error) {
	return NewMachineInstance(ctx, app, logger, checkHash)
}

func (f *DefaultMachineInstanceFactory) NewFromSnapshot(
	ctx context.Context, app *Application, logger *slog.Logger, checkHash bool,
	snapshotPath string, machineHash *common.Hash, inputIndex uint64,
) (MachineInstance, error) {
	return NewMachineInstanceFromSnapshot(ctx, app, logger, checkHash, snapshotPath, machineHash, inputIndex)
}

// MachineManager manages the lifecycle of machine instances for applications
type MachineManager struct {
	mutex           sync.RWMutex
	machines        map[int64]MachineInstance
	closed          bool
	repository      MachineRepository
	checkHash       bool
	inputBatchSize  uint64
	logger          *slog.Logger
	instanceFactory MachineInstanceFactory
}

// Option configures a MachineManager.
type Option func(*MachineManager)

// WithInstanceFactory overrides the default MachineInstanceFactory.
func WithInstanceFactory(f MachineInstanceFactory) Option {
	return func(m *MachineManager) { m.instanceFactory = f }
}

// NewMachineManager creates a new machine manager.
func NewMachineManager(
	repo MachineRepository,
	logger *slog.Logger,
	checkHash bool,
	inputBatchSize uint64,
	opts ...Option,
) *MachineManager {
	m := &MachineManager{
		machines:        map[int64]MachineInstance{},
		repository:      repo,
		checkHash:       checkHash,
		inputBatchSize:  inputBatchSize,
		logger:          logger,
		instanceFactory: &DefaultMachineInstanceFactory{},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// UpdateMachines refreshes the list of machines based on applications that
// still need local machine work.
func (m *MachineManager) UpdateMachines(ctx context.Context) error {
	apps, _, err := getMachineApplications(ctx, m.repository)
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
			"address", app.IApplicationAddress)

		// Check if we have a snapshot to load from
		var instance MachineInstance

		// Find the latest snapshot for this application
		snapshot, err := m.repository.GetLastSnapshot(ctx, app.IApplicationAddress.String())
		if err != nil {
			// Shutdown cancels the ctx mid-query; downgrade to Debug so
			// operators don't see spurious ERR lines during a graceful
			// stop. DeadlineExceeded would still flow through the Error
			// branch and demand investigation.
			if errors.Is(err, context.Canceled) {
				m.logger.Debug("GetLastSnapshot canceled during shutdown",
					"application", app.Name,
					"error", err)
			} else {
				m.logger.Error("Failed to find latest snapshot",
					"application", app.Name,
					"error", err)
			}
			// Continue with template-based initialization
		}

		if snapshot != nil && snapshot.SnapshotURI != nil {
			// Verify the snapshot path exists
			if _, statErr := os.Stat(*snapshot.SnapshotURI); statErr == nil {
				m.logger.Info("Creating machine instance from snapshot",
					"application", app.Name,
					"snapshot", *snapshot.SnapshotURI)

				instance, err = m.instanceFactory.NewFromSnapshot(
					ctx, app, m.logger, m.checkHash,
					*snapshot.SnapshotURI, snapshot.MachineHash, snapshot.Index)
				if err != nil {
					m.logger.Error("Failed to create machine instance from snapshot",
						"application", app.Name,
						"snapshot", *snapshot.SnapshotURI,
						"error", err)
					// Fall back to template-based initialization below
				}
			} else if errors.Is(statErr, os.ErrNotExist) {
				m.logger.Warn("Snapshot path does not exist",
					"application", app.Name,
					"snapshot", *snapshot.SnapshotURI)
			} else {
				m.logger.Error("Failed to access snapshot path",
					"application", app.Name,
					"snapshot", *snapshot.SnapshotURI,
					"error", statErr)
			}
		}

		// Fall back to template if snapshot loading failed or was unavailable
		if instance == nil {
			instance, err = m.instanceFactory.NewFromTemplate(ctx, app, m.logger, m.checkHash)
			if err != nil {
				// Shutdown cancels the ctx mid-spawn; the partially
				// constructed machine is torn down by NewFromTemplate
				// itself. Downgrade to Debug for the graceful-stop case
				// so the noise doesn't drown out real spawn failures.
				if errors.Is(err, context.Canceled) {
					m.logger.Debug("NewFromTemplate canceled during shutdown",
						"application", app.IApplicationAddress,
						"error", err)
				} else {
					m.logger.Error("Failed to create machine instance",
						"application", app.IApplicationAddress,
						"error", err)
				}
				continue
			}
		}

		// Synchronize the machine with processed inputs.
		// For template instances (processedInputs=0) this replays all inputs.
		// For snapshot instances (processedInputs=snapshotIndex+1) this replays
		// only inputs after the snapshot.
		err = instance.Synchronize(ctx, m.repository, m.inputBatchSize)
		if err != nil {
			m.logger.Error("Failed to synchronize machine",
				"application", app.IApplicationAddress,
				"error", err)
			if err := instance.Close(); err != nil {
				m.logger.Warn("Failed to close machine after synchronization failure",
					"application", app.Name, "error", err)
			}
			continue
		}

		// Add the machine to the manager; close if it fails
		if !m.addMachine(app.ID, instance) {
			if err := instance.Close(); err != nil {
				m.logger.Warn("Failed to close duplicate machine instance",
					"application", app.Name, "error", err)
			}
		}
	}

	// Remove machines for non-enabled applications (disabled, failed, etc.)
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

// addMachine adds a machine to the manager.
// Returns false if the manager is closed or the appID already exists.
func (m *MachineManager) addMachine(appID int64, machine MachineInstance) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return false
	}

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
				m.logger.Info("Application is no longer executable, shutting down machine",
					"application", machine.Application().Name)
			}
			if err := machine.Close(); err != nil && m.logger != nil {
				m.logger.Warn("Failed to close machine for non-executable application",
					"application", machine.Application().Name, "error", err)
			}
			delete(m.machines, id)
		}
	}
}

// Applications returns the list of applications with active machines,
// sorted by ID for deterministic iteration order.
func (m *MachineManager) Applications() []*Application {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	apps := make([]*Application, 0, len(m.machines))
	for _, machine := range m.machines {
		apps = append(apps, machine.Application())
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
	return apps
}

// Close shuts down all machine instances in parallel.
// After Close returns, no new machines can be added.
func (m *MachineManager) Close() error {
	// Mark as closed and take ownership of the machines map under the lock,
	// then release it so readers (GetMachine, HasMachine, Applications)
	// aren't blocked during the potentially slow parallel shutdown.
	m.mutex.Lock()
	m.closed = true
	machines := m.machines
	m.machines = make(map[int64]MachineInstance)
	m.mutex.Unlock()

	type closeResult struct {
		id  int64
		err error
	}

	var wg sync.WaitGroup
	results := make(chan closeResult, len(machines))

	for id, machine := range machines {
		wg.Go(func() {
			results <- closeResult{id: id, err: machine.Close()}
		})
	}

	wg.Wait()
	close(results)

	var errs []error
	for r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("failed to close machine for app %d: %w", r.id, r.err))
		}
	}

	return errors.Join(errs...)
}

func getMachineApplications(ctx context.Context, repo MachineRepository) ([]*Application, uint64, error) {
	apps, total, err := repo.ListApplications(ctx, repository.ExecutableApplicationsFilter(), repository.Pagination{}, false)
	if err != nil {
		return nil, 0, err
	}

	foreclosedApps, foreclosedTotal, err := repo.ListApplications(ctx, foreclosedMachineDrainFilter(), repository.Pagination{}, false)
	if err != nil {
		return nil, 0, err
	}
	total += foreclosedTotal
	for _, app := range foreclosedApps {
		if app.ForecloseBlock == 0 {
			continue
		}
		// Bootstrap-readiness guard, shared with the claimer and PRT. Until the
		// historical L1 scan reaches foreclose_block the inputs/epochs table is
		// incomplete, so HasUndrainedEpochsBeforeBlock would return false on an
		// empty table and the machine would be torn down before the pre-
		// foreclosure inputs it still needs to advance are even ingested. Keep
		// the machine until the scan has caught up.
		if !app.ForeclosureScanCaughtUp() {
			apps = append(apps, app)
			continue
		}
		undrained, err := repo.HasUndrainedEpochsBeforeBlock(ctx, app.ID, app.ForecloseBlock)
		if err != nil {
			return nil, 0, err
		}
		if undrained {
			apps = append(apps, app)
		}
	}
	return apps, total, nil
}

func foreclosedMachineDrainFilter() repository.ApplicationFilter {
	return repository.ApplicationFilter{
		Enabled:             new(true),
		Status:              new(ApplicationStatus_OK),
		ForeclosureRecorded: new(true),
	}
}

// getProcessedInputs retrieves processed inputs with pagination support.
func getProcessedInputs(
	ctx context.Context,
	repo MachineRepository,
	appAddress string,
	p repository.Pagination,
) ([]*Input, uint64, error) {
	f := repository.InputFilter{NotStatus: Pointer(InputCompletionStatus_None)}
	return repo.ListInputs(ctx, appAddress, f, p, false)
}
