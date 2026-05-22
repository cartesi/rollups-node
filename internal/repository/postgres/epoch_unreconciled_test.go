// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/assert"
)

// TestUnreconciledEpochStatusesAreNonTerminal pins unreconciledEpochStatuses to
// exactly the non-terminal epoch statuses. HasUnreconciledClaimsBeforeBlock and
// the "epoch_unreconciled_idx" partial index share this set; if a new
// EpochStatus is added and not classified here, the index predicate and the
// query would silently drift and the query would fall back to a full scan.
func TestUnreconciledEpochStatusesAreNonTerminal(t *testing.T) {
	terminal := map[model.EpochStatus]bool{
		model.EpochStatus_ClaimAccepted:   true,
		model.EpochStatus_ClaimRejected:   true,
		model.EpochStatus_ClaimForeclosed: true,
	}

	var want []model.EpochStatus
	for _, s := range model.EpochStatusAllValues {
		if !terminal[s] {
			want = append(want, s)
		}
	}

	assert.ElementsMatch(t, want, unreconciledEpochStatuses,
		"unreconciledEpochStatuses must be exactly the non-terminal epoch statuses. "+
			"If you added an EpochStatus, classify it here AND in the epoch_unreconciled_idx "+
			"predicate in 000001_create_initial_schema.up.sql.")
}
