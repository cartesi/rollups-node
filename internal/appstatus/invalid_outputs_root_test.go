// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package appstatus

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

func TestSetInvalidOutputsRoot(t *testing.T) {
	for _, initial := range []model.ApplicationStatus{model.ApplicationStatus_OK, model.ApplicationStatus_Failed} {
		t.Run(initial.String(), func(t *testing.T) {
			app := newTestApp()
			app.Status = initial
			app.Reason = new("previous operational failure")
			repo := &mockRepo{}
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))

			err := SetInvalidOutputsRootf(t.Context(), logger, repo, app,
				"epoch %d: declared outputs root does not match calculated root", 7)

			reason := "epoch 7: declared outputs root does not match calculated root"
			require.EqualError(t, err, reason)
			require.Equal(t, 1, repo.callCount)
			require.Equal(t, app.ID, repo.lastAppID)
			require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, repo.lastStatus)
			require.Equal(t, reason, *repo.lastReason)
			require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, app.Status)
			require.Equal(t, reason, *app.Reason)
			require.Contains(t, logs.String(), `level=ERROR msg="marking application with invalid outputs root (terminal)"`)
		})
	}
}

func TestSetInvalidOutputsRootNormalizesReason(t *testing.T) {
	app := newTestApp()
	repo := &mockRepo{}
	reason := strings.Repeat("r", maxReasonLength+1)

	err := SetInvalidOutputsRoot(t.Context(), slog.Default(), repo, app, reason)

	require.EqualError(t, err, NormalizeReason(reason))
	require.Equal(t, NormalizeReason(reason), *repo.lastReason)
	require.Equal(t, *repo.lastReason, *app.Reason)
}

func TestSetInvalidOutputsRootPreservesInMemoryStateOnWriteFailure(t *testing.T) {
	app := newTestApp()
	app.Status = model.ApplicationStatus_Failed
	app.Reason = new("original failure")
	dbErr := errors.New("database unavailable")
	repo := &mockRepo{err: dbErr}

	err := SetInvalidOutputsRoot(t.Context(), slog.Default(), repo, app, "root mismatch")

	require.ErrorIs(t, err, dbErr)
	require.ErrorContains(t, err, "root mismatch")
	require.Equal(t, 1, repo.callCount)
	require.Equal(t, model.ApplicationStatus_Failed, app.Status)
	require.Equal(t, "original failure", *app.Reason)
}

func TestSetInvalidOutputsRootPreservesExistingTerminals(t *testing.T) {
	for _, current := range model.ApplicationStatusAllValues {
		if !current.IsTerminal() {
			continue
		}
		t.Run(current.String(), func(t *testing.T) {
			app := newTestApp()
			app.Status = current
			app.Reason = new("original terminal reason")
			repo := &mockRepo{}

			err := SetInvalidOutputsRoot(t.Context(), slog.Default(), repo, app, "later root mismatch")

			require.ErrorContains(t, err, "later root mismatch")
			require.Zero(t, repo.callCount)
			require.Equal(t, current, app.Status)
			require.Equal(t, "original terminal reason", *app.Reason)
		})
	}
}

func TestInvalidOutputsRootDoesNotEscalate(t *testing.T) {
	for _, requested := range []model.ApplicationStatus{
		model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged,
		model.ApplicationStatus_Corrupted,
		model.ApplicationStatus_InvalidOutputsRoot,
	} {
		t.Run(requested.String(), func(t *testing.T) {
			app := newTestApp()
			app.Status = model.ApplicationStatus_InvalidOutputsRoot
			app.Reason = new("original root mismatch")
			repo := &mockRepo{}

			if requested == model.ApplicationStatus_Failed {
				require.NoError(t, SetFailed(t.Context(), slog.Default(), repo, app, "later failure"))
			} else {
				require.Error(t, setTerminalStatus(t.Context(), slog.Default(), repo, app, requested, "later finding"))
			}

			require.Zero(t, repo.callCount)
			require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, app.Status)
			require.Equal(t, "original root mismatch", *app.Reason)
		})
	}
}
