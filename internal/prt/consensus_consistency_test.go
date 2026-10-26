// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

func TestInconsistentConsensusViewsRetryWithoutWritesAndReportPersistentFailure(t *testing.T) {
	for _, test := range inconsistentConsensusViews() {
		t.Run(test.name, func(t *testing.T) {
			f, client := tickFixture(t)
			var output bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&output, nil))
			f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Epoch{f.epoch}, uint64(1), nil).Times(6)
			view := resultTestSnapshot(f.epoch, true)
			test.mutate(&view)
			f.consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil).Times(6)
			f.consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil).Times(6)
			f.consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil).Times(6)
			ready := f.d.Ready()
			for _, head := range []uint64{101, 101, 102, 103, 104, 105} {
				tickHead(client, head)
				expectProtectionReads(t, f.factory, f.consensus, observerFacts(head), f.epoch)
				reschedule, err := f.d.Tick(t.Context())
				require.NoError(t, err, "inconsistent views retry without an ERROR from the tick loop")
				require.False(t, reschedule)
				require.Equal(t, head-100, uint64(f.d.observationFailures[f.app.ID].failedHeads))
				require.Equal(t, ready, f.d.Ready(), "application read failures do not change hard prerequisites")
				if head < 105 {
					require.NotContains(t, output.String(), "level=ERROR")
				}
			}
			require.Equal(t, 1, strings.Count(output.String(), "PRT tournament observation is stalled"))
			require.Contains(t, output.String(), test.want)
			require.Contains(t, output.String(), "application="+f.app.Name)
			require.Contains(t, output.String(), "address="+strings.ToLower(f.app.IApplicationAddress.Hex()))
			require.Empty(t, f.repo.windows, "no rejected snapshot can publish a tournament interval")
			require.Empty(t, f.backend.published)
			require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			f.repo.AssertNotCalled(t, "UpdateEpochReconciledStaged", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertExpectations(t)
			f.consensus.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestFinalizedStagingClockContradictionPreservesSavedStage(t *testing.T) {
	for _, block := range []uint64{9, 11} {
		t.Run(strconv.FormatUint(block, 10), func(t *testing.T) {
			f := newDeliveryFixture(t)
			f.epoch.Status, f.epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(10))
			view := resultTestSnapshot(f.epoch, true)
			view.sealed.StagingBlockNumber = block
			f.repo.On("UpdateEpochStagingBlock", mock.Anything, f.app.ID, f.epoch.Index, uint64(10), block).
				Return(nil).Maybe()
			f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, model.ApplicationStatus_Corrupted,
				mock.MatchedBy(func(reason *string) bool {
					return reason != nil && strings.Contains(*reason, "staging block inconsistent with DaveConsensus")
				})).Return(nil).Once()

			expectStoredProtection(t, f.repo.prtRepositoryMock, f.app, f.epoch)
			err := f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view)
			require.ErrorContains(t, err, "staging block inconsistent with DaveConsensus")
			require.Equal(t, model.ApplicationStatus_Corrupted, f.app.Status)
			require.Equal(t, uint64(10), *f.epoch.StagedAtBlock)
			f.repo.AssertNotCalled(t, "UpdateEpochStagingBlock", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			f.repo.AssertExpectations(t)
		})
	}
}
