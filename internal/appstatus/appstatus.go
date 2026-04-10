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

// Repository is the minimal interface needed to update application state.
type Repository interface {
	UpdateApplicationHealth(ctx context.Context, appID int64, state ApplicationHealth, reason *string) error
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
// Returns the database error if the state update fails; returns nil on success.
func SetFailed(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reason string,
) error {
	return setApplicationHealth(ctx, logger, repo, app, ApplicationHealth_Failed, reason)
}

// SetFailedf marks an application as FAILED with a formatted reason string.
// Returns the database error if the state update fails; returns nil on success.
// Unlike [SetInoperablef], this intentionally returns nil on success because
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

// SetInoperable marks an application as INOPERABLE (irrecoverable).
// Use for data corruption, state mismatches, invariant violations, and
// on-chain disagreements that cannot be resolved by restarting.
//
// The reason parameter must be a pre-formatted string describing the failure.
// Always returns a non-nil error containing the reason because INOPERABLE is
// a terminal state and callers should always stop processing the application.
func SetInoperable(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reason string,
) error {
	reason = truncateReason(reason)
	dbErr := setApplicationHealth(ctx, logger, repo, app, ApplicationHealth_Inoperable, reason)
	reasonErr := errors.New(reason)
	if dbErr != nil {
		return errors.Join(reasonErr, dbErr)
	}
	return reasonErr
}

// SetInoperablef marks an application as INOPERABLE with a formatted reason string.
// It logs the transition, persists the state, and returns a non-nil error containing
// the reason (joined with the DB error if the update failed).
// This function always returns a non-nil error because INOPERABLE is a terminal state
// and callers should always stop processing the application.
func SetInoperablef(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	reasonFmt string,
	args ...any,
) error {
	return SetInoperable(ctx, logger, repo, app, fmt.Sprintf(reasonFmt, args...))
}

// truncateReason truncates a reason string to maxReasonLength to avoid
// exceeding the database VARCHAR(4096) constraint.
func truncateReason(reason string) string {
	if len(reason) > maxReasonLength {
		return reason[:maxReasonLength] + "... (truncated)"
	}
	return reason
}

func setApplicationHealth(
	ctx context.Context,
	logger *slog.Logger,
	repo Repository,
	app *Application,
	state ApplicationHealth,
	reason string,
) error {
	reason = truncateReason(reason)

	switch state { //nolint:exhaustive
	case ApplicationHealth_Failed:
		logger.Warn("marking application as failed (recoverable)",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"reason", reason)
	case ApplicationHealth_Inoperable:
		logger.Error("marking application as inoperable (irrecoverable)",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"reason", reason)
	default:
		logger.Error("marking application with unexpected state",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"state", state,
			"reason", reason)
	}

	err := repo.UpdateApplicationHealth(ctx, app.ID, state, &reason)
	if err != nil {
		logger.Error("failed to update application state",
			"application", app.Name,
			"address", app.IApplicationAddress.String(),
			"target_state", state, "error", err)
		return err
	}

	// Only update in-memory state when the DB write succeeds to keep
	// the in-memory Application consistent with the database.
	app.Health = state
	app.Reason = &reason
	return nil
}
