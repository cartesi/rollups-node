// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPRTForeclosureRootWaitPreservesIndependentExit(t *testing.T) {
	const activeRootWait = "root still has active matches"
	for _, waiting := range []string{"missing", "future coverage", activeRootWait} {
		t.Run(waiting, func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtForeclosedApp(1, 100)
			admitTestDeployment(s, repo, app)
			epoch := checkpointEpoch(0, "0x100")
			epoch.ApplicationID = app.ID
			open := &model.Epoch{ApplicationID: app.ID, Index: 1, FirstBlock: 90, Status: model.EpochStatus_Open}
			var root *model.Tournament
			if waiting != "missing" {
				root = &model.Tournament{ApplicationID: app.ID, EpochIndex: epoch.Index, Address: *epoch.TournamentAddress,
					Snapshot: model.TournamentSnapshot{AsOfBlock: 100, Standing: model.TournamentStandingMatchesActive}}
				if waiting == "future coverage" {
					root.Snapshot = model.TournamentSnapshot{AsOfBlock: 101, FinishedAtBlock: 90,
						Standing: model.TournamentStandingRootWinner, FinalStateHash: epoch.MachineHash}
				}
			}
			repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(false, nil).Once()
			repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(true, nil).Once()
			repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).Return(root, nil).Once()
			repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
				repository.Pagination{}, false).Return([]*model.Epoch{epoch, open}, uint64(2), nil).Once()
			repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, uint64(100)).Return(repository.ErrNoUpdate).Once()
			repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, open.Index, uint64(100)).Return(nil).Once()
			var logs bytes.Buffer
			s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))

			require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{epoch, open}, nil, 100))

			require.Equal(t, []uint64{open.Index}, loggedForeclosedEpochs(t, &logs))
			require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
			require.NotNil(t, epoch.Commitment)
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
			repo.AssertExpectations(t)
		})
	}
}

func TestPRTForeclosurePendingRootKeepsResultComparisonInOrder(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtForeclosedApp(1, 100)
	admitTestDeployment(s, repo, app)
	pending, later := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x200")
	pending.ApplicationID, later.ApplicationID = app.ID, app.ID
	// This row vector tests the frontier guard, not a reachable event history:
	// ingestion writes the earlier acceptance before it creates the next seal.
	// An unfinished earlier result still stops later result comparisons.
	require.Nil(t, pending.ClaimTransactionHash)
	repo.On("HasUndrainedEpochsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(false, nil).Once()
	repo.On("HasUnreconciledClaimsBeforeBlock", mock.Anything, app.ID, uint64(100)).Return(true, nil).Once()
	repo.On("GetTournament", mock.Anything, app.IApplicationAddress.Hex(), pending.TournamentAddress.Hex()).Return(
		&model.Tournament{ApplicationID: app.ID, EpochIndex: pending.Index, Address: *pending.TournamentAddress,
			Snapshot: model.TournamentSnapshot{AsOfBlock: 100, Standing: model.TournamentStandingMatchesActive}}, nil).Once()
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{pending, later}, uint64(2), nil).Once()
	for _, epoch := range []*model.Epoch{pending, later} {
		repo.On("ForeclosePRTEpoch", mock.Anything, app.ID, epoch.Index, uint64(100)).Return(repository.ErrNoUpdate).Once()
	}

	require.NoError(t, s.drainForeclosedClaims(t.Context(), app, []*model.Epoch{pending, later}, nil, 100))

	require.Equal(t, model.ApplicationStatus_OK, app.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, later.Status)
	require.NotNil(t, later.Commitment)
	repo.AssertNotCalled(t, "GetTournament", mock.Anything, app.IApplicationAddress.Hex(), later.TournamentAddress.Hex())
	repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestPRTForeclosureWaitNoticeSurvivesEmptyClockInventory(t *testing.T) {
	s, repo := newPRTServiceMock()
	app := prtForeclosedApp(1, 100)
	admitTestDeployment(s, repo, app)
	epoch := checkpointEpoch(0, "0x100")
	epoch.ClaimTransactionHash = new(common.HexToHash("0xabc"))
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	s.reportForeclosureRootWait(app, epoch, nil, 100)
	// Clock discovery excludes known claim transactions, but their missing
	// root still belongs to the reconciliation page. Moving heads must not
	// turn the same unresolved incident into a fresh notice every tick.
	s.reportProtectionStops(t.Context(), app, nil, txsender.BlockRef{Number: 101}, deploymentCompatible, 0, nil)
	s.reportForeclosureRootWait(app, epoch, nil, 101)
	require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("Foreclosed PRT epoch awaits")))

	// A successful full page with no pending epoch owns notice cleanup.
	repo.On("ListEpochs", mock.Anything, app.Name, repository.EpochFilter{Status: model.NonTerminalEpochStatuses()},
		repository.Pagination{}, false).Return([]*model.Epoch{}, uint64(0), nil).Once()
	require.NoError(t, s.foreclosePendingClaimEpochs(t.Context(), app, 101))
	for key := range s.protectionNotices[app.ID] {
		require.NotEqual(t, protectionForeclosureWait, key.kind)
	}
	repo.AssertExpectations(t)
}
