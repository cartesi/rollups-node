// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func failTournamentObservation(ctx context.Context, f *observerCheckpointFixture, epoch *model.Epoch, head uint64, cause error) {
	f.t.Helper()
	f.epochs(epoch)
	windowEnd := min(head, f.app.LastEpochCheckBlock)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(windowEnd))).
		Return(uint64(0), cause).Once()
	deferActions, err := f.s.checkEpochs(ctx, f.app, head)
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
		require.Equal(t, test.count < tournamentObservationFailureThreshold, f.s.Ready())
	}
	require.Equal(t, uint64(51), f.s.observationFailures[f.app.ID].fromBlock)
	require.Equal(t, 1, strings.Count(output.String(), "readiness is degraded"))
	require.Contains(t, output.String(), "application="+f.app.Name)
	require.Contains(t, output.String(), "operation=tournament_event_window")
	require.Contains(t, output.String(), "from_block=51")
	require.Contains(t, output.String(), "configured_head=104")
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestTournamentObservationHealthClearsOnlyAfterPublication(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	for head := uint64(100); head < 105; head++ {
		failTournamentObservation(t.Context(), f, epoch, head, errors.New("observation unavailable"))
	}
	require.False(t, f.s.Ready())

	// A head with no new observation window does not prove recovery.
	f.epochs()
	_, err := f.s.checkEpochs(t.Context(), f.app, 105)
	require.NoError(t, err)
	require.False(t, f.s.Ready())
	require.Equal(t, uint8(5), f.s.observationFailures[f.app.ID].failedHeads)

	// An unrelated query failure and an empty result cannot clear it either.
	f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Epoch{}, uint64(0), errors.New("epoch query unavailable")).Once()
	_, err = f.s.checkEpochs(t.Context(), f.app, 106)
	require.ErrorContains(t, err, "epoch query unavailable")
	f.epochs()
	_, err = f.s.checkEpochs(t.Context(), f.app, 107)
	require.NoError(t, err)
	require.False(t, f.s.Ready())

	f.epochs(epoch)
	f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(1), nil).Once()
	f.tournament(epoch, *epoch.TournamentAddress, 0, 1, 90, 100, &TournamentEvents{}, nil)
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
	f.unaccepted(epoch, 90)
	_, err = f.s.checkEpochs(t.Context(), f.app, 108)
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
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(cause).Once()
	_, err := f.s.checkEpochs(t.Context(), f.app, 100)
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
	f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, uint64(100)).Return(nil).Once()
	cause := errors.New("acceptance write unavailable")
	f.acceptance(epoch, tournament, cause)
	_, err := f.s.checkEpochs(t.Context(), f.app, 100)
	require.ErrorIs(t, err, cause)
	require.Equal(t, uint64(100), f.app.LastTournamentCheckBlock)
	require.Empty(t, f.s.observationFailures)
	require.True(t, f.s.Ready())
}

func TestTournamentObservationHealthKeepsHealthyApplicationsRunning(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	f.s.defaultBlock = model.DefaultBlock_Finalized
	f.s.submissionEnabled = true
	epoch := checkpointEpoch(0, "0x100")
	healthy := prtRevertTestApp()
	healthy.ID++
	healthy.Name = "healthy-app"
	healthy.IApplicationAddress = common.HexToAddress("0x999")
	healthy.IConsensusAddress = common.HexToAddress("0x998")
	healthy.LastEpochCheckBlock = 200
	unprepared := &model.Epoch{Status: model.EpochStatus_Closed, LastBlock: 201}
	f.factory.On("CreateDaveConsensusAdapter", healthy.IConsensusAddress).Return(&daveConsensusAdapterMock{}, nil)
	f.s.pendingTransactions[healthy.ID] = pendingTournamentTransaction{Hash: common.HexToHash("0x777")}
	f.client.On("TransactionByHash", mock.Anything, common.HexToHash("0x777")).Return((*types.Transaction)(nil), true, nil)
	for head := uint64(100); head < 106; head++ {
		f.repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
			Return([]*model.Application{f.app, healthy}, uint64(2), nil).Once()
		f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
			Return(&types.Header{Number: new(big.Int).SetUint64(head)}, nil).Once()
		f.epochs(epoch)
		f.consensus.On("TournamentLevelCount", mock.Anything).Return(uint64(0), errors.New("invalid descriptor")).Once()
		f.repo.On("ListEpochs", mock.Anything, healthy.Name, mock.Anything, repository.Pagination{}, false).
			Return([]*model.Epoch{unprepared}, uint64(1), nil).Once()
		f.repo.On("StoreTournamentEvents", mock.Anything, healthy.ID, mock.Anything, head).Return(nil).Once()
		f.client.On("BlockNumber", mock.Anything).Return(head, nil).Once()
		reschedule, err := f.s.Tick(t.Context())
		require.False(t, reschedule)
		require.ErrorContains(t, err, f.app.IApplicationAddress.Hex())
		require.Equal(t, head, healthy.LastTournamentCheckBlock)
	}
	require.False(t, f.s.Ready())
	require.Len(t, f.s.observationFailures, 1)
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Equal(t, model.ApplicationStatus_OK, healthy.Status)
}

func TestTournamentObservationHealthIgnoresPendingAndMissingTransactions(t *testing.T) {
	for _, pending := range []bool{true, false} {
		t.Run(fmt.Sprintf("pending=%t", pending), func(t *testing.T) {
			f := newObserverCheckpointFixture(t)
			f.s.defaultBlock = model.DefaultBlock_Finalized
			f.s.submissionEnabled = true
			f.app.LastEpochCheckBlock = 200
			epoch := &model.Epoch{Status: model.EpochStatus_Closed, LastBlock: 201}
			hash := common.HexToHash("0x777")
			f.s.pendingTransactions[f.app.ID] = pendingTournamentTransaction{Action: tournamentActionJoin, Hash: hash}
			var lookupError error
			if !pending {
				lookupError = ethereum.NotFound
			}
			for head := uint64(100); head < 106; head++ {
				f.repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
					Return([]*model.Application{f.app}, uint64(1), nil).Once()
				f.client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
					Return(&types.Header{Number: new(big.Int).SetUint64(head)}, nil).Once()
				f.epochs(epoch)
				f.repo.On("StoreTournamentEvents", mock.Anything, f.app.ID, mock.Anything, head).Return(nil).Once()
				f.client.On("BlockNumber", mock.Anything).Return(head, nil).Once()
				f.client.On("TransactionByHash", mock.Anything, hash).
					Return((*types.Transaction)(nil), pending, lookupError).Once()
				reschedule, err := f.s.Tick(t.Context())
				require.False(t, reschedule)
				if pending {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, ethereum.NotFound)
				}
				require.True(t, f.s.Ready())
				require.Empty(t, f.s.observationFailures)
			}
			require.Contains(t, f.s.pendingTransactions, f.app.ID)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		})
	}
}

func TestTournamentObservationHealthPrunesIneligibleApplications(t *testing.T) {
	f := newObserverCheckpointFixture(t)
	epoch := checkpointEpoch(0, "0x100")
	for head := uint64(100); head < 105; head++ {
		failTournamentObservation(t.Context(), f, epoch, head, errors.New("observation unavailable"))
	}
	f.repo.On("ListApplications", mock.Anything, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Application{}, uint64(0), errors.New("application list unavailable")).Once()
	reschedule, err := f.s.Tick(t.Context())
	require.False(t, reschedule)
	require.ErrorContains(t, err, "application list unavailable")
	require.False(t, f.s.Ready(), "a failed eligibility query cannot clear a failure")

	// Only disabled or removed PRT applications lose observer eligibility.
	f.repo.On("ListApplications", mock.Anything, mock.MatchedBy(func(filter repository.ApplicationFilter) bool {
		return filter.Enabled != nil && *filter.Enabled &&
			len(filter.Statuses) == 0 &&
			filter.ConsensusType != nil && *filter.ConsensusType == model.Consensus_PRT
	}), repository.Pagination{}, false).Return([]*model.Application{}, uint64(0), nil).Once()
	reschedule, err = f.s.Tick(t.Context())
	require.False(t, reschedule)
	require.NoError(t, err)
	require.True(t, f.s.Ready())
	require.Empty(t, f.s.observationFailures)
}

func TestTournamentObservationHealthIgnoresShutdownCancellation(t *testing.T) {
	for _, test := range []struct {
		name     string
		stopping bool
		cause    error
		counted  bool
	}{
		{shutdownCancellationCase, true, context.Canceled, false},
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
			s.pruneTournamentObservationFailures([]*model.Application{app})
			require.False(t, s.Ready())
			require.Equal(t, status, app.Status)
			require.Empty(t, repo.Calls)
		})
	}
}
