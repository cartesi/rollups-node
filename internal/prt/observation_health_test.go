// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func failTournamentObservation(ctx context.Context, f *observerCheckpointFixture, epoch *model.Epoch, head uint64, cause error) {
	f.t.Helper()
	f.epochs(epoch)
	windowEnd := head
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(windowEnd))).
		Return(uint64(0), cause).Once()
	deferActions, err := observeEpochsForTest(ctx, f.s, f.app, head)
	require.ErrorIs(f.t, err, cause)
	require.True(f.t, deferActions)
	require.Equal(f.t, uint64(50), f.app.LastTournamentCheckBlock)
	require.Equal(f.t, model.ApplicationStatus_OK, f.app.Status)
}

func TestTournamentObservationHealthCountsIncreasingHeads(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	for i, test := range []struct {
		head  uint64
		count uint8
	}{
		{100, 1}, {100, 1}, {99, 1}, {101, 2}, {102, 3}, {103, 4}, {104, 5}, {105, 5},
	} {
		// Neither a new error message nor a changing head resets the failure.
		failTournamentObservation(t.Context(), f, epoch, test.head, fmt.Errorf("provider response %d at head %d", i, test.head))
		require.Equal(t, test.count, f.s.observationFailures[f.app.ID].failedHeads)
		require.True(t, f.s.Ready())
	}
	require.Equal(t, uint64(10), f.s.observationFailures[f.app.ID].fromBlock)
	require.Equal(t, 1, strings.Count(output.String(), "PRT tournament observation is stalled"))
	require.Contains(t, output.String(), "application="+f.app.Name)
	require.Contains(t, output.String(), "operation=tournament_observation")
	require.Contains(t, output.String(), "from_block=10")
	require.Contains(t, output.String(), "configured_head=104")
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentObservationHealthClearsAfterAnotherObserverRetiresTarget(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.s.observationFailures[f.app.ID] = tournamentObservationFailure{fromBlock: 51, lastHead: 105, failedHeads: 5}
	f.repo.storedTournaments = []*model.Tournament{{Address: common.HexToAddress("0x100"),
		Snapshot: model.TournamentSnapshot{AsOfBlock: 105, FinishedAtBlock: 90, Standing: model.TournamentStandingRootWinner,
			BondRecovery: model.TournamentBondRecovery{Disposition: model.BondDispositionRecovered}}}}
	require.NoError(t, f.s.observeTournamentState(t.Context(), f.app, nil, f.consensus, observerFacts(106)))
	require.Empty(t, f.s.observationFailures)
	f.s.recordTournamentObservationFailure(t.Context(), f.app, 107, 80, errors.New("new target unavailable"))
	require.Equal(t, uint8(1), f.s.observationFailures[f.app.ID].failedHeads)
	f.s.recordTournamentObservationFailure(t.Context(), f.app, 108, 95, errors.New("another target unavailable"))
	require.Equal(t, uint64(95), f.s.observationFailures[f.app.ID].fromBlock)
}

func TestTournamentObservationHealthClearsOnlyAfterPublication(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	for head := uint64(100); head < 105; head++ {
		failTournamentObservation(t.Context(), f, epoch, head, errors.New("observation unavailable"))
	}
	require.True(t, f.s.Ready())

	// A head with no new observation window does not prove recovery.
	f.epochs()
	_, err := observeEpochsForTest(t.Context(), f.s, f.app, 50)
	require.NoError(t, err)
	require.True(t, f.s.Ready())
	require.Equal(t, uint8(5), f.s.observationFailures[f.app.ID].failedHeads)

	// An unrelated query failure and an empty result cannot clear it either.
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{}, uint64(0), errors.New("epoch query unavailable")).Once()
	_, err = observeEpochsForTest(t.Context(), f.s, f.app, 106)
	require.ErrorContains(t, err, "epoch query unavailable")
	f.epochs()
	_, err = observeEpochsForTest(t.Context(), f.s, f.app, 50)
	require.NoError(t, err)
	require.True(t, f.s.Ready())

	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
	f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
	f.unaccepted(epoch, 90)
	_, err = observeEpochsForTest(t.Context(), f.s, f.app, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	require.True(t, f.s.Ready())
	require.Empty(t, f.s.observationFailures)
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentObservationHealthIncludesFailedPublication(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Once()
	f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	cause := errors.New("no match found for update")
	f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(cause).Once()
	_, err := observeEpochsForTest(t.Context(), f.s, f.app, 100)
	require.ErrorIs(t, err, cause)
	require.Equal(t, uint8(1), f.s.observationFailures[f.app.ID].failedHeads)
	require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock)
}

func TestTournamentObservationHealthIgnoresAcceptanceFailure(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	epoch.ClaimTransactionHash = new(common.HexToHash("0x900"))
	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(1), nil).Once()
	tournament := f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
	cause := errors.New("acceptance write unavailable")
	f.acceptance(epoch, tournament, cause)
	_, err := observeEpochsForTest(t.Context(), f.s, f.app, 100)
	require.ErrorIs(t, err, cause)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	require.Empty(t, f.s.observationFailures)
	require.True(t, f.s.Ready())
}

func TestTournamentObservationHealthKeepsHealthyApplicationsRunning(t *testing.T) {
	f, client := tickFixture(t)
	healthy := *f.app
	healthy.ID++
	healthy.Name = "healthy-app"
	healthy.IApplicationAddress = common.HexToAddress("0x999")
	healthy.IConsensusAddress = common.HexToAddress("0x998")
	f.repo.apps = append(f.repo.apps, &healthy)
	ready := resultTestSnapshot(f.epoch, false)
	consensus := &daveConsensusAdapterMock{}
	consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(ready.sealed, nil)
	consensus.On("CanStageTournamentResult", mock.Anything).Return(ready.stage, nil)
	consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(ready.accept, nil)
	f.factory.On("CreateDaveConsensusAdapter", healthy.IConsensusAddress).Return(consensus, nil)
	f.repo.On("GetEpoch", mock.Anything, healthy.IApplicationAddress.Hex(), f.epoch.Index).Return((*model.Epoch)(nil), nil)
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{f.epoch}, uint64(1), nil)
	f.repo.On("ListEpochs", mock.Anything, healthy.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{}, uint64(0), nil)
	f.consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(ready.sealed, nil)
	f.consensus.On("CanStageTournamentResult", mock.Anything).Return(ready.stage, nil)
	f.consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(ready.accept, nil)
	f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(0), errors.New("invalid descriptor"))
	for head := uint64(101); head < 107; head++ {
		tickHead(client, head)
		_, err := f.d.Tick(t.Context())
		require.ErrorContains(t, err, "invalid descriptor")
		require.Equal(t, uint64(100), healthy.LastTournamentCheckBlock, "a healthy app with no tournament has no new coverage to record")
	}
	require.True(t, f.d.Ready(), "per-app observation cannot change hard prerequisite readiness")
	require.Len(t, f.d.observationFailures, 1)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Equal(t, model.ApplicationStatus_OK, healthy.Status)
}

func TestTournamentObservationHealthClearsDisabledApplications(t *testing.T) {
	f := newDeliveryFixture(t)
	f.app.Enabled = false
	f.app.Status = model.ApplicationStatus_Failed
	f.d.observationFailures[f.app.ID] = tournamentObservationFailure{failedHeads: tournamentObservationFailureThreshold}
	facts := txsender.BlockRef{Number: f.app.LastTournamentCheckBlock}
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, facts))
	require.Empty(t, f.d.observationFailures)
	f.repo.AssertNotCalled(t, "ListEpochs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	f.factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
}

func TestTournamentObservationHealthIgnoresShutdownCancellation(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, test := range []struct {
		name     string
		stopping bool
		cause    error
		counted  bool
	}{
		{shutdownCancellationCase, true, context.Canceled, false},
		{"joined shutdown cancellations", true, errors.Join(context.Canceled, fmt.Errorf("read: %w", context.Canceled)), false},
		{"shutdown database failure", true, dbErr, true},
		{"shutdown mixed database failure", true, fmt.Errorf("read: %w", errors.Join(context.Canceled, dbErr)), true},
		{shutdownDeadlineCase, true, context.DeadlineExceeded, true},
		{"shutdown joined deadline", true, errors.Join(context.Canceled, context.DeadlineExceeded), true},
		{runningCancellationCase, false, context.Canceled, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.stopping {
				cancel()
			}
			failTournamentObservation(ctx, f, checkpointEpoch(0, "0x100"), 100, test.cause)
			require.Equal(t, test.counted, len(f.s.observationFailures) == 1)
		})
	}
}

func TestTournamentObservationReadinessConcurrentWithUpdates(t *testing.T) {
	s, _ := newPRTServiceMock()
	app := prtRevertTestApp()
	cause := errors.New("observation unavailable")
	var readers sync.WaitGroup
	readers.Go(func() {
		for range 1000 {
			s.Ready()
		}
	})
	for head := uint64(1); head <= 1000; head++ {
		s.recordTournamentObservationFailure(t.Context(), app, head, head, cause)
		s.clearTournamentObservationFailure(app.ID)
	}
	readers.Wait()
	require.True(t, s.Ready())
}

func TestTournamentObservationHealthIncludesLocalFailureStates(t *testing.T) {
	for _, status := range []model.ApplicationStatus{
		model.ApplicationStatus_OK, model.ApplicationStatus_Failed,
		model.ApplicationStatus_Corrupted, model.ApplicationStatus_Diverged,
	} {
		t.Run(status.String(), func(t *testing.T) {
			s, repo := newPRTServiceMock()
			app := prtRevertTestApp()
			app.Status = status
			for head := uint64(1); head <= tournamentObservationFailureThreshold; head++ {
				s.recordTournamentObservationFailure(t.Context(), app, head, head, errors.New("window unavailable"))
			}
			require.True(t, s.Ready())
			require.Equal(t, status, app.Status)
			require.Empty(t, repo.Calls)
		})
	}
}
