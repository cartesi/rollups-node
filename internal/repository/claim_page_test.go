// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaimCursorKeepsEachApplicationsEpochPosition(t *testing.T) {
	first := &model.Application{ID: 1}
	second := &model.Application{ID: 2}
	page := ClaimPage{ApplicationID: 1, NextEpoch: new(uint64(900)), Work: []ClaimWork{
		{Application: first, Epoch: &model.Epoch{Index: 300}}, {Application: first, Epoch: &model.Epoch{Index: 900}},
	}}
	var cursor ClaimCursor
	cursor.Advance(page.Work[0], page)
	require.NotContains(t, cursor.Epochs, int64(1), "inspecting only the oldest does not skip the uninspected tail")
	cursor.Advance(page.Work[1], page)
	require.Equal(t, uint64(900), cursor.Epochs[1])
	other := ClaimPage{ApplicationID: 2, NextEpoch: new(uint64(4)), Work: []ClaimWork{
		{Application: second, Epoch: &model.Epoch{Index: 1}}, {Application: second, Epoch: &model.Epoch{Index: 4}},
	}}
	for _, work := range other.Work {
		cursor.Advance(work, other)
	}
	require.Equal(t, uint64(900), cursor.Epochs[1])
	require.Equal(t, uint64(4), cursor.Epochs[2])
	require.Equal(t, int64(2), cursor.AfterApplicationID)
	other.NextEpoch = nil
	cursor.Advance(other.Work[1], other)
	require.NotContains(t, cursor.Epochs, int64(2), "the next app visit restarts its finite tail")
	require.Equal(t, uint64(900), cursor.Epochs[1])
}
