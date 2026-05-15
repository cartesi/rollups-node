// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package appstatus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/model"
)

// maxReasonLength is the maximum length of a reason string stored in the
// database. The DB column is VARCHAR(4096); we leave margin to avoid
// constraint violations from deeply-nested error chains.
const maxReasonLength = 4000

// Repository is the minimal interface needed to update application status.
type Repository interface {
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
}

// SetFailed marks an application as FAILED (recoverable).
// Use for machine runtime errors that can be resolved by operator intervention
// (e.g., OOM kill, process crash). The operator can re-enable the application
// after fixing the root cause.
//
// Recovery assumptions — FAILED is safe to re-enable only when:
//   - The failure was a machine runtime error (not a DB desync).
//   - The last snapshot is consistent with the database state.
//   - Synchronize() will correctly replay inputs from the snapshot point.
//
// The reason parameter must be a pre-formatted string describing the failure.
// Returns the database error if the status update fails; returns nil on success.
func SetFailed(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reason string,
) error {
	return setApplicationStatus(ctx, logger, repo, app, ApplicationStatus_Failed, reason)
}

// SetFailedf marks an application as FAILED with a formatted reason string.
// Returns the database error if the status update fails; returns nil on success.
// Unlike [SetDivergedf] and [SetCorruptedf], this intentionally returns nil on success because
// FAILED is recoverable — callers typically continue with their own error.
func SetFailedf(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reasonFmt string,
	args ...any,
) error {
	return SetFailed(ctx, logger, repo, app, fmt.Sprintf(reasonFmt, args...))
}

// SetDiverged marks an application DIVERGED (terminal): the node's computed
// claim disagrees with what the chain accepted. The application produces no
// more claims, but observation continues (it is gated on the enabled flag),
// so withdrawals and other post-foreclosure events keep being indexed.
//
// The reason must be a pre-formatted string describing the disagreement.
// Always returns a non-nil error containing the reason because DIVERGED is
// terminal and callers should stop processing the application.
//
// Logging contract: both the reason and any DB write error are logged at
// ERROR level via slog before the function returns. Callers that don't need
// to propagate the failure upward (e.g. best-effort loops over multiple
// applications) may discard the returned error with `_ =` without losing
// operator visibility.
func SetDiverged(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reason string,
) error {
	return setTerminalStatus(ctx, logger, repo, app, ApplicationStatus_Diverged, reason)
}

// SetDivergedf marks an application DIVERGED with a formatted reason string.
func SetDivergedf(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reasonFmt string,
	args ...any,
) error {
	return SetDiverged(ctx, logger, repo, app, fmt.Sprintf(reasonFmt, args...))
}

// SetCorrupted marks an application CORRUPTED (terminal): local state is
// missing or inconsistent and cannot be resolved by restarting. As with
// DIVERGED, observation continues and only claim work stops.
//
// The reason must be a pre-formatted string. Always returns a non-nil error
// because CORRUPTED is terminal and callers should stop.
func SetCorrupted(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reason string,
) error {
	return setTerminalStatus(ctx, logger, repo, app, ApplicationStatus_Corrupted, reason)
}

// SetCorruptedf marks an application CORRUPTED with a formatted reason string.
func SetCorruptedf(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reasonFmt string,
	args ...any,
) error {
	return SetCorrupted(ctx, logger, repo, app, fmt.Sprintf(reasonFmt, args...))
}

// setTerminalStatus persists a terminal health status (DIVERGED or CORRUPTED)
// and returns a non-nil error containing the reason (joined with the DB error
// if the update failed), so callers always have a value to propagate or log.
func setTerminalStatus(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	status ApplicationStatus,
	reason string,
) error {
	reason = truncateReason(reason)
	dbErr := setApplicationStatus(ctx, logger, repo, app, status, reason)
	reasonErr := errors.New(reason)
	if dbErr != nil {
		return errors.Join(reasonErr, dbErr)
	}
	return reasonErr
}

// truncateReason truncates a reason string to maxReasonLength to avoid
// exceeding the database VARCHAR(4096) constraint.
func truncateReason(reason string) string {
	if len(reason) > maxReasonLength {
		return reason[:maxReasonLength] + "... (truncated)"
	}
	return reason
}

func setApplicationStatus(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	status ApplicationStatus,
	reason string,
) error {
	reason = truncateReason(reason)

	switch status {
	case ApplicationStatus_Failed:
		logger.Warn("marking application as failed (recoverable)",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"reason", reason)
	case ApplicationStatus_Diverged:
		logger.Error("marking application as diverged (terminal)",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"reason", reason)
	case ApplicationStatus_Corrupted:
		logger.Error("marking application as corrupted (terminal)",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"reason", reason)
	default:
		logger.Error("marking application with unexpected status",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"status", status,
			"reason", reason)
	}

	err := repo.UpdateApplicationStatus(ctx, app.ID, status, &reason)
	if err != nil {
		logger.Error("failed to update application status",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"target_status", status, "error", err)
		return err
	}

	// Only update in-memory status when the DB write succeeds to keep
	// the in-memory Application consistent with the database.
	app.Status = status
	app.Reason = &reason
	return nil
}
