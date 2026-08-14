// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"sync"

	"github.com/cartesi/rollups-node/internal/appstatus"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/replay"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrApplicationNotFound = errors.New("application not found")
	ErrMachineCreation     = errors.New("failed to create machine")
)

// MachineRepository defines the repository interface needed by the MachineManager
type MachineRepository interface {
	repository.ReplayRepository

	// ListApplications retrieves applications based on filter criteria
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)
	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)

	// GetLastSnapshot retrieves the most recent input with a snapshot for the given application
	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
	GetApplication(ctx context.Context, nameOrAddress string) (*Application, error)

	// UpdateApplicationStatus persists an application's health status.
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
}

// MachineInstanceFactory creates MachineInstance values from applications.
// Implementations decide whether to load from a template or snapshot.
type MachineInstanceFactory interface {
	NewFromTemplate(ctx context.Context, app *Application, logger *slog.Logger, checkTemplateHash bool) (MachineInstance, error)
	NewFromSnapshot(ctx context.Context, app *Application, logger *slog.Logger,
		snapshotPath string, expectedHash common.Hash, inputIndex uint64) (MachineInstance, error)
}

// DefaultMachineInstanceFactory delegates to NewMachineInstance / NewMachineInstanceFromSnapshot.
type DefaultMachineInstanceFactory struct{}

func (f *DefaultMachineInstanceFactory) NewFromTemplate(
	ctx context.Context, app *Application, logger *slog.Logger, checkTemplateHash bool,
) (MachineInstance, error) {
	return NewMachineInstance(ctx, app, logger, checkTemplateHash)
}

func (f *DefaultMachineInstanceFactory) NewFromSnapshot(
	ctx context.Context, app *Application, logger *slog.Logger,
	snapshotPath string, expectedHash common.Hash, inputIndex uint64,
) (MachineInstance, error) {
	return NewMachineInstanceFromSnapshot(ctx, app, logger, snapshotPath, expectedHash, inputIndex)
}

// MachineManager manages the lifecycle of machine instances for applications
type MachineManager struct {
	mutex                      sync.RWMutex
	machines                   map[int64]MachineInstance
	pendingApplicationFailures map[int64]*pendingApplicationFailure
	closed                     bool
	repository                 MachineRepository
	checkTemplateHash          bool
	inputBatchSize             uint64
	logger                     *slog.Logger
	instanceFactory            MachineInstanceFactory
	replayRun                  func(context.Context, repository.ReplayRepository, replay.Executor, replay.Options) (replay.Result, error)
}

// pendingApplicationFailure fences an application after a FAILED status write
// cannot be confirmed. It is deliberately private: this is a short-lived retry
// queue, not process-local application health state.
type pendingApplicationFailure struct {
	application *Application
	reason      string
}

// Option configures a MachineManager.
type Option func(*MachineManager)

// WithInstanceFactory overrides the default MachineInstanceFactory.
func WithInstanceFactory(f MachineInstanceFactory) Option {
	return func(m *MachineManager) { m.instanceFactory = f }
}

// withReplayRun overrides replay execution for manager policy tests.
func withReplayRun(
	run func(context.Context, repository.ReplayRepository, replay.Executor, replay.Options) (replay.Result, error),
) Option {
	return func(m *MachineManager) { m.replayRun = run }
}

func snapshotProcessedInputs(app *Application, snapshot *Input) (uint64, error) {
	if snapshot.Index == math.MaxUint64 {
		return 0, fmt.Errorf("%w: snapshot input index cannot be incremented", ErrInvalidSnapshotPoint)
	}
	processedInputs := snapshot.Index + 1
	if processedInputs > app.ProcessedInputs {
		return 0, fmt.Errorf(
			"%w: snapshot represents %d processed inputs, application has %d",
			ErrInvalidSnapshotPoint, processedInputs, app.ProcessedInputs,
		)
	}
	return processedInputs, nil
}

func closeMachineCandidate(
	logger *slog.Logger,
	candidate MachineInstance,
	failureMessage string,
) {
	if candidate == nil {
		return
	}
	if err := candidate.Close(); err != nil {
		logger.Warn(failureMessage, "error", err)
	}
}

func (m *MachineManager) tryLoadSnapshotInstance(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
) MachineInstance {
	snapshot, err := m.repository.GetLastSnapshot(ctx, app.IApplicationAddress.String())
	if err != nil {
		// Shutdown cancels the query mid-flight; deadlines and repository
		// failures still require operator attention.
		if errors.Is(err, context.Canceled) {
			logger.Debug("GetLastSnapshot canceled during shutdown", "error", err)
		} else {
			logger.Error("Failed to find latest snapshot", "error", err)
		}
		return nil
	}
	if snapshot == nil || snapshot.SnapshotURI == nil {
		return nil
	}

	snapshotLogger := logger.With(
		"snapshot", *snapshot.SnapshotURI,
		"input_index", snapshot.Index,
	)
	if snapshot.MachineHash == nil {
		snapshotLogger.Warn("Snapshot input has no persisted machine hash; falling back to template")
		return nil
	}

	expectedProcessedInputs, err := snapshotProcessedInputs(app, snapshot)
	if err != nil {
		snapshotLogger.Warn(
			"Snapshot input is not a valid application starting point; falling back to template",
			"app_processed_inputs", app.ProcessedInputs,
			"error", err,
		)
		return nil
	}

	if _, err := os.Stat(*snapshot.SnapshotURI); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			snapshotLogger.Warn("Snapshot path does not exist")
		} else {
			snapshotLogger.Error("Failed to access snapshot path", "error", err)
		}
		return nil
	}

	snapshotLogger.Info("Creating machine instance from snapshot")
	candidate, err := m.instanceFactory.NewFromSnapshot(
		ctx, app, m.logger,
		*snapshot.SnapshotURI, *snapshot.MachineHash, snapshot.Index,
	)
	if err != nil {
		snapshotLogger.Error("Failed to create machine instance from snapshot", "error", err)
		closeMachineCandidate(snapshotLogger, candidate, "Failed to close partial snapshot machine")
		return nil
	}
	if candidate == nil {
		snapshotLogger.Error("Snapshot factory returned no machine instance")
		return nil
	}
	if candidate.ProcessedInputs() != expectedProcessedInputs {
		snapshotLogger.Error("Snapshot machine has an invalid processed-input count",
			"expected_processed_inputs", expectedProcessedInputs,
			"actual_processed_inputs", candidate.ProcessedInputs(),
		)
		closeMachineCandidate(snapshotLogger, candidate, "Failed to close invalid snapshot machine")
		return nil
	}

	// The default snapshot factory verifies this hash while loading. Verify it
	// again at the manager boundary because tests and alternative factories are
	// explicit injection seams and are not required to duplicate that policy.
	actualHash, err := candidate.Hash(ctx)
	if err != nil {
		snapshotLogger.Error("Failed to verify snapshot machine hash", "error", err)
		closeMachineCandidate(snapshotLogger, candidate, "Failed to close unverifiable snapshot machine")
		return nil
	}
	if common.Hash(actualHash) != *snapshot.MachineHash {
		snapshotLogger.Error("Snapshot machine hash mismatch",
			"expected_hash", snapshot.MachineHash.Hex(),
			"actual_hash", common.Hash(actualHash).Hex(),
		)
		closeMachineCandidate(snapshotLogger, candidate, "Failed to close mismatched snapshot machine")
		return nil
	}
	return candidate
}

func (m *MachineManager) tryLoadTemplateInstance(
	ctx context.Context,
	app *Application,
	logger *slog.Logger,
) MachineInstance {
	candidate, err := m.instanceFactory.NewFromTemplate(ctx, app, m.logger, m.checkTemplateHash)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Debug("NewFromTemplate canceled during shutdown", "error", err)
		} else {
			logger.Error("Failed to create machine instance", "error", err)
		}
		closeMachineCandidate(logger, candidate, "Failed to close partial template machine")
		return nil
	}
	if candidate == nil {
		logger.Error("Template factory returned no machine instance")
		return nil
	}
	return candidate
}

func replayContradictionReason(err error) string {
	reason := replay.ErrContradiction.Error()
	var detail *replay.ContradictionError
	if errors.As(err, &detail) {
		// ContradictionError represents byte values only as lengths and
		// digests, so its text is safe to persist for operator diagnostics.
		reason = detail.Error()
	}
	return reason
}

func classifyReplayFailure(err error) (reason, logMessage string, shouldFence bool) {
	switch {
	case errors.Is(err, replay.ErrContradiction):
		// A contradiction proves that this local reconstruction is incompatible
		// with the stored result, but not that the database itself is corrupt. A
		// repaired template or runtime may recover it, so fence it as FAILED. An
		// unresolved contradiction is detected and fenced again on re-enable.
		return replayContradictionReason(err),
			"Machine replay contradicts stored input result; marking application failed", true
	case machine.IsExecutionLimitError(err):
		return fmt.Sprintf("machine replay reached execution limit: %v", err),
			"Machine replay reached an execution limit; marking application failed", true
	case errors.Is(err, machine.ErrDeadlineExceeded):
		return fmt.Sprintf("machine replay exceeded execution deadline: %v", err),
			"Machine replay exceeded its execution deadline; marking application failed", true
	case errors.Is(err, ErrIncompleteAdvance):
		return fmt.Sprintf("machine replay returned an incomplete advance result: %v", err),
			"Machine replay returned an incomplete advance result; marking application failed", true
	case errors.Is(err, machine.ErrMachineInternal):
		return fmt.Sprintf("machine replay failed internally: %v", err),
			"Machine replay failed internally; marking application failed", true
	default:
		return "", "Failed to replay machine", false
	}
}

// NewMachineManager creates a new machine manager.
func NewMachineManager(
	repo MachineRepository,
	logger *slog.Logger,
	checkTemplateHash bool,
	inputBatchSize uint64,
	opts ...Option,
) *MachineManager {
	m := &MachineManager{
		machines:                   map[int64]MachineInstance{},
		pendingApplicationFailures: map[int64]*pendingApplicationFailure{},
		repository:                 repo,
		checkTemplateHash:          checkTemplateHash,
		inputBatchSize:             inputBatchSize,
		logger:                     logger,
		instanceFactory:            &DefaultMachineInstanceFactory{},
		replayRun:                  replay.Run,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// UpdateMachines refreshes the list of machines based on applications that
// still need local machine work.
func (m *MachineManager) UpdateMachines(ctx context.Context) error {
	// Retry failed status writes before doing any more machine work. Every app
	// returned here remains fenced for this entire update, including when the
	// retry succeeds but a repository read observes a stale executable row.
	fenced, persistenceErr := m.persistPendingApplicationFailures(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(persistenceErr, ctxErr)
	}
	if persistenceErr != nil && !IsOnlyApplicationFailurePersistenceErrors(persistenceErr) {
		return persistenceErr
	}

	apps, err := getMachineApplications(ctx, m.repository)
	if err != nil {
		return errors.Join(persistenceErr, err)
	}

	// Create machines for new applications
	for _, app := range apps {
		if m.HasMachine(app.ID) {
			continue
		}
		if _, pending := fenced[app.ID]; pending {
			continue
		}

		appLogger := m.logger.With(
			"application", app.Name,
			"address", app.IApplicationAddress,
		)
		appLogger.Info("Creating new machine instance")

		// Prefer a verified snapshot and fall back to the template whenever the
		// snapshot is absent, stale, inaccessible, or invalid.
		instance := m.tryLoadSnapshotInstance(ctx, app, appLogger)
		if instance == nil {
			instance = m.tryLoadTemplateInstance(ctx, app, appLogger)
		}
		if instance == nil {
			continue
		}

		// Canonical verification is the normal reconstruction policy: replay all
		// completed inputs after the candidate's starting point and compare each
		// status, exception payload, machine root, and cumulative outputs root.
		_, err = m.replayRun(ctx, m.repository, instance, replay.Options{
			Application:      app,
			FromInput:        instance.ProcessedInputs(),
			ToInputExclusive: app.ProcessedInputs,
			BatchSize:        m.inputBatchSize,
			Verification:     repository.ReplayVerificationCanonical,
		})
		if err != nil {
			var appPersistenceErr error
			reason, logMessage, shouldFence := classifyReplayFailure(err)
			appLogger.Error(logMessage, "error", err)
			if shouldFence {
				m.FenceApplicationFailure(app, reason)
				fenced[app.ID] = struct{}{}
				appPersistenceErr = m.persistApplicationFailure(ctx, app.ID)
			}
			if err := instance.Close(); err != nil {
				appLogger.Warn("Failed to close machine after replay failure", "error", err)
			}
			if appPersistenceErr != nil {
				persistenceErr = errors.Join(persistenceErr, appPersistenceErr)
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return errors.Join(persistenceErr, ctxErr)
			}
			continue
		}

		// Add the machine to the manager; close if it fails
		if !m.addMachine(app.ID, instance) {
			if err := instance.Close(); err != nil {
				appLogger.Warn("Failed to close duplicate machine instance", "error", err)
			}
		}
	}

	// Remove machines for non-enabled applications (disabled, failed, etc.)
	m.removeMachines(excludeFencedApplications(apps, fenced))

	return persistenceErr
}

func excludeFencedApplications(apps []*Application, fenced map[int64]struct{}) []*Application {
	if len(fenced) == 0 {
		return apps
	}
	active := make([]*Application, 0, len(apps))
	for _, app := range apps {
		if _, excluded := fenced[app.ID]; !excluded {
			active = append(active, app)
		}
	}
	return active
}

// FenceApplicationFailure immediately fences app and queues the exact
// normalized FAILED reason for a later durability retry. Callers use this only
// after their initial status write failed; this method does not write the
// repository itself.
func (m *MachineManager) FenceApplicationFailure(app *Application, reason string) {
	reason = appstatus.NormalizeReason(reason)
	pending := &pendingApplicationFailure{
		application: app,
		reason:      reason,
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.pendingApplicationFailures[app.ID]; !exists {
		m.pendingApplicationFailures[app.ID] = pending
	}
}

// persistPendingApplicationFailures retries every pending FAILED write and
// returns the application IDs that must remain fenced for this update.
func (m *MachineManager) persistPendingApplicationFailures(
	ctx context.Context,
) (map[int64]struct{}, error) {
	m.mutex.RLock()
	appIDs := make([]int64, 0, len(m.pendingApplicationFailures))
	for appID := range m.pendingApplicationFailures {
		appIDs = append(appIDs, appID)
	}
	m.mutex.RUnlock()
	sort.Slice(appIDs, func(i, j int) bool { return appIDs[i] < appIDs[j] })

	fenced := make(map[int64]struct{}, len(appIDs))
	var persistenceErrors []error
	for _, appID := range appIDs {
		fenced[appID] = struct{}{}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fenced, errors.Join(errors.Join(persistenceErrors...), ctxErr)
		}
		if err := m.persistApplicationFailure(ctx, appID); err != nil {
			persistenceErrors = append(persistenceErrors, err)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fenced, errors.Join(errors.Join(persistenceErrors...), ctxErr)
		}
	}
	return fenced, errors.Join(persistenceErrors...)
}

func (m *MachineManager) persistApplicationFailure(ctx context.Context, appID int64) error {
	m.mutex.RLock()
	pending := m.pendingApplicationFailures[appID]
	m.mutex.RUnlock()
	if pending == nil {
		return nil
	}

	if writeErr := appstatus.SetFailed(ctx, m.logger, m.repository, pending.application, pending.reason); writeErr != nil {
		if errors.Is(writeErr, repository.ErrNotFound) {
			m.deletePendingApplicationFailure(appID, pending)
			return nil
		}
		// Another process may have installed a stronger terminal status, or the
		// write may have committed even though its result was lost. Retire the
		// fence only when the durable row proves one of those outcomes. An
		// unrelated FAILED reason remains fenced: it does not prove that this
		// application failure was recorded. Unknown/read failures and OK
		// (including disabled+OK) also remain pending.
		current, readErr := m.repository.GetApplication(
			ctx, pending.application.IApplicationAddress.String(),
		)
		retire := errors.Is(readErr, repository.ErrNotFound)
		if readErr == nil {
			retire = current == nil ||
				current.ID != pending.application.ID ||
				applicationFailureAlreadyResolved(current, pending.reason)
		}
		if retire {
			m.deletePendingApplicationFailure(appID, pending)
			return nil
		}
		return &ApplicationFailurePersistenceError{
			ApplicationID: appID,
			WriteErr:      writeErr,
			ReadErr:       readErr,
		}
	}

	m.deletePendingApplicationFailure(appID, pending)
	return nil
}

func applicationFailureAlreadyResolved(app *Application, reason string) bool {
	if app == nil {
		return false
	}
	switch app.Status {
	case ApplicationStatus_Diverged, ApplicationStatus_Corrupted:
		return true
	case ApplicationStatus_Failed:
		return app.Reason != nil && *app.Reason == reason
	case ApplicationStatus_OK:
		return false
	default:
		return false
	}
}

func (m *MachineManager) deletePendingApplicationFailure(
	appID int64,
	pending *pendingApplicationFailure,
) {
	m.mutex.Lock()
	// Remove only the pending record that this persistence attempt observed.
	if m.pendingApplicationFailures[appID] == pending {
		delete(m.pendingApplicationFailures, appID)
	}
	m.mutex.Unlock()
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

// HasPendingApplicationFailures reports whether any local machine work is
// fenced while its durable application status remains unconfirmed.
func (m *MachineManager) HasPendingApplicationFailures() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.pendingApplicationFailures) != 0
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

func getMachineApplications(ctx context.Context, repo MachineRepository) ([]*Application, error) {
	apps, _, err := repo.ListApplications(ctx, repository.ExecutableApplicationsFilter(), repository.Pagination{}, false)
	if err != nil {
		return nil, err
	}

	foreclosedApps, _, err := repo.ListApplications(ctx, foreclosedMachineDrainFilter(), repository.Pagination{}, false)
	if err != nil {
		return nil, err
	}
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
			return nil, err
		}
		if undrained {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func foreclosedMachineDrainFilter() repository.ApplicationFilter {
	return repository.ApplicationFilter{
		Enabled:             new(true),
		Status:              new(ApplicationStatus_OK),
		ForeclosureRecorded: new(true),
	}
}
