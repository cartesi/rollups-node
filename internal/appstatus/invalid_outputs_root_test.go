// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package appstatus

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestInvalidOutputsRootAllowsOnlyIntegrityEscalation(t *testing.T) {
	for _, requested := range []model.ApplicationStatus{model.ApplicationStatus_Failed,
		model.ApplicationStatus_Diverged, model.ApplicationStatus_Corrupted} {
		t.Run(requested.String(), func(t *testing.T) {
			app := newTestApp()
			app.Status = model.ApplicationStatus_InvalidOutputsRoot
			app.Reason = new("original root mismatch")
			repo := &mockRepo{}
			if requested == model.ApplicationStatus_Failed {
				require.NoError(t, SetFailed(t.Context(), slog.Default(), repo, app, "later failure"))
				require.Zero(t, repo.callCount)
				require.Equal(t, model.ApplicationStatus_InvalidOutputsRoot, app.Status)
			} else {
				require.Error(t, setTerminalStatus(t.Context(), slog.Default(), repo, app, requested, "integrity finding"))
				require.Equal(t, 1, repo.callCount)
				require.Equal(t, requested, app.Status)
			}
		})
	}
}
