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

	"github.com/cartesi/rollups-node/internal/appstatus"
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
	UpdateEpochInputsProcessed(ctx context.Context, nameOrAddress string, epochIndex uint64, proof *StateProof) error
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
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

// Step performs one processing cycle of the advancer.
// It updates machines, processes one batch of inputs per application in round-robin
// order, and returns whether any application had work remaining.
//
// Per-app errors are accumulated so that a failure in one application does not block
// processing of other healthy applications. Context cancellation is always propagated
// immediately.
//
// The returned boolean indicates whether any app successfully processed inputs and
// potentially has more work. Callers use this to decide whether to re-tick immediately
// (via the Reschedule channel) or wait for the next timer/event.
func (s *Service) Step(ctx context.Context) (bool, error) {
	// Check for context cancellation or shutdown in progress.
	// The framework sets Stopping before calling Impl.Stop(), so this
	// prevents starting new work while the machine manager is being torn down.
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.IsStopping() {
		return false, nil
	}

	// Update the machine manager with any new or disabled applications
	updateErr := s.machineManager.UpdateMachines(ctx)
	if updateErr != nil && !manager.IsOnlyApplicationFailurePersistenceErrors(updateErr) {
		return false, updateErr
	}

	// Get all applications with active machines (returned sorted by ID).
	apps := s.machineManager.Applications()
	if len(apps) == 0 {
		return false, updateErr
	}
	anyWork := false
	errs := []error{updateErr}
	for _, app := range apps {
		hadWork, err := s.stepApp(ctx, app)
		if err != nil {
			// Context errors (cancellation or timeout) mean no further apps will
			// succeed — stop immediately instead of accumulating identical errors.
			if ctx.Err() != nil {
				return false, err
			}
			errs = append(errs, err)
			continue
		}
		if hadWork {
			anyWork = true
		}
	}

	return anyWork, errors.Join(errs...)
}

// stepApp processes unprocessed epochs for a single application, returning
// whether more work may remain. It enforces a global per-app budget of
// inputBatchSize inputs across all epochs, so no single app can monopolize
// the tick even when it has many small epochs.
func (s *Service) stepApp(ctx context.Context, app *Application) (bool, error) {
	appAddress := app.IApplicationAddress.String()
	if !s.machineManager.HasMachine(app.ID) {
		return false, fmt.Errorf("%w: %d", ErrNoApp, app.ID)
	}

	epochs, _, err := getUnprocessedEpochs(ctx, s.repository, appAddress)
	if err != nil {
		return false, err
	}

	budgetRemaining := s.inputBatchSize
	for _, epoch := range epochs {
		moreInputs, processed, terminal, err := s.processEpochInputs(
			ctx, app, epoch.Index, budgetRemaining,
		)
		if err != nil {
			return false, err
		}

		budgetRemaining -= processed
		if terminal {
			// The terminal result itself is durable, but its epoch remains CLOSED:
			// there is no accepted state from which to publish a validity proof.
			return false, nil
		}

		// More inputs remain in this epoch — reschedule immediately.
		if moreInputs {
			return true, nil
		}

		// All inputs in this epoch processed. Finalize if closed.
		if epoch.Status == EpochStatus_Closed {
			if err := s.finalizeEpoch(ctx, app, epoch); err != nil {
				return false, err
			}
		}

		// Budget exhausted — yield so other apps get a turn.
		if budgetRemaining == 0 {
			return true, nil
		}
	}

	return false, nil
}

// finalizeEpoch verifies all inputs are processed, handles snapshot/proof
// bookkeeping, and marks the epoch as inputs-processed in the database.
func (s *Service) finalizeEpoch(ctx context.Context, app *Application, epoch *Epoch) error {
	allProcessed, err := s.isAllEpochInputsProcessed(app, epoch)
	if err != nil {
		return err
	}
	if !allProcessed {
		return nil
	}

	proof, err := s.prepareEpochPublication(ctx, app, epoch)
	if err != nil {
		return err
	}

	appAddress := app.IApplicationAddress.String()
	if err := s.repository.UpdateEpochInputsProcessed(ctx, appAddress, epoch.Index, proof); err != nil {
		return fmt.Errorf(
			"publishing state proof for application %s epoch %d: %w",
			app.Name,
			epoch.Index,
			err,
		)
	}

	s.Logger.Info("Epoch updated to Inputs Processed",
		"application", app.Name, "epoch_index", epoch.Index)
	return nil
}

// processEpochInputs fetches and processes up to `budget` unprocessed inputs
// for an epoch. Returns whether more unprocessed inputs remain in this epoch
// and the number of inputs actually processed.
func (s *Service) processEpochInputs(
	ctx context.Context, app *Application, epochIndex uint64, budget uint64,
) (bool, uint64, bool, error) {
	appAddress := app.IApplicationAddress.String()
	inputs, total, err := getUnprocessedInputs(ctx, s.repository, appAddress, epochIndex, budget)
	if err != nil {
		return false, 0, false, err
	}
	if len(inputs) == 0 {
		return false, 0, false, nil
	}
	s.Logger.Debug("Processing inputs",
		"application", app.Name, "epoch_index", epochIndex,
		"count", len(inputs), "total", total)
	processed, terminal, err := s.processInputs(ctx, app, inputs)
	if err != nil {
		return false, processed, false, err
	}
	// More work remains if total exceeds what we just processed.
	return total > processed, processed, terminal, nil
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

// processInputs stores the completed prefix of inputs and stops immediately
// after a terminal completion. Rejected inputs are nonterminal and therefore
// do not truncate the batch.
func (s *Service) processInputs(
	ctx context.Context,
	app *Application,
	inputs []*Input,
) (uint64, bool, error) {
	// Skip if there are no inputs to process
	if len(inputs) == 0 {
		return 0, false, nil
	}

	// Get the machine instance for this application
	machine, exists := s.machineManager.GetMachine(app.ID)
	if !exists {
		return 0, false, fmt.Errorf("%w: %d", ErrNoApp, app.ID)
	}

	var processed uint64
	// Process each input sequentially
	for _, input := range inputs {
		// Check for context cancellation before processing each input
		if err := ctx.Err(); err != nil {
			return processed, false, err
		}

		s.Logger.Info("Processing input",
			"application", app.Name,
			"epoch", input.EpochIndex,
			"index", input.Index)

		// Advance the machine with this input
		result, err := machine.Advance(ctx, input.RawData, input.EpochIndex, input.Index, app.IsDaveConsensus())
		input.RawData = nil // allow GC to collect payload while batch continues
		if err != nil {
			// Cancellation of this service context is the normal shutdown path,
			// so it does not change the application's status. A returned error
			// that happens to include context.Canceled is still a failure while
			// the service context remains active.
			if errors.Is(ctx.Err(), context.Canceled) {
				s.Logger.Debug("Advance stopped because the service is shutting down",
					"application", app.Name,
					"index", input.Index,
					"error", err)
				return processed, false, err
			}

			// Anything else, including a deadline, is an execution failure.
			s.Logger.Error("Error executing advance",
				"application", app.Name,
				"index", input.Index,
				"error", err)

			s.markApplicationFailed(ctx, app, err.Error())

			// Eagerly close the machine to release the child process.
			// The app has failed, so no further operations will succeed.
			// Skip if the runtime was already destroyed inside the manager.
			if !errors.Is(err, manager.ErrMachineClosed) {
				if closeErr := machine.Close(); closeErr != nil {
					s.Logger.Warn("Failed to close machine after advance error",
						"application", app.Name,
						"error", closeErr)
				}
			}

			return processed, false, err
		}
		// log advance result hashes
		s.Logger.Info("Processing input finished",
			"application", app.Name,
			"epoch", result.EpochIndex,
			"index", result.InputIndex,
			"status", result.Status,
			"outputs", len(result.Outputs),
			"reports", len(result.Reports),
			"periodic_state_hashes", len(result.PeriodicStateHashes),
			"padding_repetitions", result.PaddingRepetitions,
		)

		// Store the result in the database
		err = s.repository.StoreAdvanceResult(ctx, input.EpochApplicationID, result)
		if err != nil {
			if errors.Is(err, repository.ErrApplicationNotRunnable) {
				// Another service durably fenced this application after the
				// machine began its advance. Discard only this stale runtime; the
				// database already contains the authoritative app-local outcome.
				s.Logger.Warn("Advance result lost an application status race; closing stale machine",
					"application", app.Name,
					"epoch", input.EpochIndex,
					"index", input.Index,
					"error", err)
				closeErr := machine.Close()
				if closeErr != nil {
					s.Logger.Error("Could not close stale machine after application status race",
						"application", app.Name,
						"error", closeErr)
				}
				return processed, false, errors.Join(err, closeErr)
			}

			// Advance has already changed the live machine, but the transaction
			// did not confirm that its result was saved. The database may still
			// show this input as pending. Reusing this machine could then execute
			// the input again from the wrong state, so StoreAdvanceResult is not
			// retried against this live machine.
			s.Logger.Error(
				"Could not confirm that the advance result was saved; "+
					"the live machine has already advanced, so services will stop; "+
					"after the node is restarted, execution will use persisted state",
				"application", app.Name,
				"epoch", input.EpochIndex,
				"index", input.Index,
				"error", err)

			// Try to close the machine now so the already-advanced runtime cannot
			// be used again. Cancel services even if Close fails. After the node
			// is restarted, the machine is rebuilt from persisted state, and the
			// database decides whether this input is still pending and needs a
			// safe retry.
			closeErr := machine.Close()
			s.Cancel() // triggers graceful shutdown of all services
			if closeErr != nil {
				s.Logger.Error("Could not close the machine after its advance result "+
					"was not confirmed saved; service shutdown is still required",
					"application", app.Name,
					"error", closeErr)
			}
			return processed, false, errors.Join(err, closeErr)
		}
		processed++
		if result.Status.IsTerminal() {
			applicationStatus, _ := result.Status.TerminalApplicationStatus()
			s.Logger.Error("Application execution terminated",
				"application", app.Name,
				"address", app.IApplicationAddress,
				"epoch", result.EpochIndex,
				"input", result.InputIndex,
				"completion_status", result.Status,
				"application_status", applicationStatus,
			)
			return processed, true, nil
		}

		// Create a snapshot if needed
		if result.Status == InputCompletionStatus_Accepted {
			err = s.handleSnapshot(ctx, app, machine, input)
			if err != nil {
				switch {
				case errors.Is(ctx.Err(), context.Canceled):
					s.Logger.Debug("Snapshot creation cancelled due to shutdown",
						"application", app.Name,
						"index", input.Index)
					return processed, false, err
				case errors.Is(err, manager.ErrMachineClosed):
					s.Logger.Error("Snapshot failure destroyed the machine runtime",
						"application", app.Name,
						"index", input.Index,
						"error", err)
					s.markApplicationFailed(ctx, app, err.Error())
					return processed, false, err
				default:
					s.Logger.Error("Failed to create snapshot",
						"application", app.Name,
						"index", input.Index,
						"error", err)
					// Continue processing even if snapshot creation fails
				}
			}
		}
	}

	return processed, false, nil
}

// markApplicationFailed persists FAILED or installs a local fence when the
// status write cannot be confirmed. Keeping those operations together prevents
// a failed application from being handed more work while durability is retried.
func (s *Service) markApplicationFailed(ctx context.Context, app *Application, reason string) {
	if err := appstatus.SetFailed(ctx, s.Logger, s.repository, app, reason); err != nil {
		s.machineManager.FenceApplicationFailure(app, reason)
		s.Logger.Error(
			"Could not persist FAILED application status; the application remains fenced until the write is retried",
			"application", app.Name,
			"db_error", err,
		)
	}
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

// prepareEpochPublication acquires a fresh proof from the resident accepted
// machine state for every epoch, including empty and rejection-only epochs,
// and performs any epoch-boundary snapshot before the proof is published.
func (s *Service) prepareEpochPublication(
	ctx context.Context,
	app *Application,
	epoch *Epoch,
) (*StateProof, error) {
	machine, exists := s.machineManager.GetMachine(app.ID)
	if !exists {
		return nil, fmt.Errorf("%w: %d", ErrNoApp, app.ID)
	}

	proof, err := machine.StateProof(ctx)
	if err != nil {
		// If the runtime was destroyed (e.g., child process crashed), mark the
		// app as failed to avoid an infinite retry loop.
		if errors.Is(err, manager.ErrMachineClosed) {
			s.markApplicationFailed(ctx, app, err.Error())
		}
		return nil, fmt.Errorf("failed to get final state proof from machine: %w", err)
	}
	if !proof.IsComplete() {
		return nil, fmt.Errorf(
			"machine returned an incomplete final state proof: %w",
			repository.ErrInvalidStateProof,
		)
	}

	if epoch.InputIndexLowerBound == epoch.InputIndexUpperBound ||
		app.ExecutionParameters.SnapshotPolicy != SnapshotPolicy_EveryEpoch {
		return proof, nil
	}

	lastProcessedInput, err := s.repository.GetLastProcessedInput(
		ctx, app.IApplicationAddress.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get last input: %w", err)
	}
	if lastProcessedInput != nil {
		if err := s.handleSnapshot(ctx, app, machine, lastProcessedInput); err != nil {
			return nil, err
		}
	}
	return proof, nil
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

	// Guard against partial store by cleaning up before a retry
	if _, err := os.Stat(snapshotPath); err == nil {
		if err := s.removeSnapshot(snapshotPath, app.Name); err != nil {
			return fmt.Errorf("failed to clean stale snapshot directory: %w", err)
		}
	}

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
		if errors.Is(err, context.Canceled) {
			s.Logger.Debug("GetLastSnapshot cancelled due to shutdown",
				"application", app.Name)
		} else {
			s.Logger.Error("Failed to get previous snapshot",
				"application", app.Name,
				"error", err)
			// Continue even if we can't get the previous snapshot
		}
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
