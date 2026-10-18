// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestJoinRefusalWarningStateExpiresAndDoesNotSuppressANewEpoch(t *testing.T) {
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	err := &joinRefusal{epoch: 3, tournament: *f.epoch.TournamentAddress, reason: "unsupported root geometry"}
	f.d.warnJoinPlanningRefusal(f.app, f.block, err)
	err.epoch++
	f.d.warnJoinPlanningRefusal(f.app, f.block, err)
	require.Equal(t, 2, strings.Count(logs.String(), "PRT cannot join tournament"))
	f.d.diagnostics.joinRefusals[99] = joinRefusalWarning{warnedAt: time.Now().Add(-2 * publicationWarningInterval)}
	f.d.warnJoinPlanningRefusal(f.app, f.block, err)
	require.NotContains(t, f.d.diagnostics.joinRefusals, int64(99))
	require.Len(t, f.d.diagnostics.joinRefusals, 1)
	// Ordinary no-work and changed eligibility do not become missed-protection alarms.
	f.d.warnJoinPlanningRefusal(f.app, f.block, txsender.ErrIneligible)
	require.Equal(t, 2, strings.Count(logs.String(), "PRT cannot join tournament"))
}

func TestJoinPlanningRefusalsWarnWithApplicationAndTournamentContext(t *testing.T) {
	for _, test := range []struct{ name, reason string }{
		{"geometry", "unsupported root geometry"}, {"expired deadline", "join deadline leaves no block"},
		{"closed window", "no longer accepts joins"}, {"registration link", "outputs validator differs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			refusal := test.name
			f := newDeliveryFixture(t)
			backend, snapshot := f.backend, f.block
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			f.epoch.LastBlock = 95
			f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, mock.Anything).Return((*uint64)(nil), nil)
			view := resultTestSnapshot(f.epoch, false)
			view.stage.IsFinished = false
			f.snapshot(view)
			adapter := &tournamentAdapterMock{}
			adapter.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).Return(CommitmentStanding{}, nil)
			descriptor := TournamentDescriptor{
				Height: model.Log2EpochComputationHashLeafCount, StartInstant: 100, Allowance: 30,
			}
			if refusal == "geometry" {
				descriptor.Height++
			}
			if refusal == "expired deadline" {
				descriptor.Allowance = 10
			}
			adapter.On("Descriptor", mock.Anything).Return(descriptor, nil)
			adapter.On("Standing", mock.Anything).Return(TournamentStanding{AcceptsJoins: refusal != "closed window"}, nil)
			adapter.On("BondValue", mock.Anything).Return(big.NewInt(50), nil)
			f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(adapter, nil)
			if refusal == "registration link" {
				backend.wrongConsensus = true
			}
			facts := txsender.BlockRef{Number: 100}
			for range 3 {
				candidates, err := f.d.candidates(t.Context(), snapshot, facts)
				require.NoError(t, err, "a refused join must not block other applications")
				require.Empty(t, candidates)
			}
			message := logs.String()
			require.Equal(t, 1, strings.Count(message, "PRT cannot join tournament"))
			for _, context := range []string{f.app.Name, strings.ToLower(f.app.IApplicationAddress.Hex()),
				strings.ToLower(f.epoch.TournamentAddress.Hex()), "epoch=3", "block=110", "deadline="} {
				require.Contains(t, strings.ToLower(message), strings.ToLower(context))
			}
			require.Contains(t, message, test.reason)
			require.Empty(t, backend.published)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			warning := f.d.diagnostics.joinRefusals[f.app.ID]
			warning.warnedAt = warning.warnedAt.Add(-publicationWarningInterval)
			f.d.diagnostics.joinRefusals[f.app.ID] = warning
			_, err := f.d.candidates(t.Context(), snapshot, facts)
			require.NoError(t, err)
			require.Equal(t, 2, strings.Count(logs.String(), "PRT cannot join tournament"))
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
