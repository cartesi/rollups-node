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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"math/big"
	"strings"
	"testing"
)

const (
	shutdownCancellationCase  = "shutdown cancellation"
	shutdownDeadlineCase      = "shutdown deadline"
	shutdownMixedDeadlineCase = "shutdown cancellation and deadline"
	runningCancellationCase   = "running cancellation"
)

const standingReadName = "standing"

func TestPRTObserverShutdownKeepsJoinedRealFailuresVisible(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, mixed := range []bool{false, true} {
		s, repo := newPRTServiceMock()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		causes := []error{repository.ErrTournamentIntervalStale, txsender.ErrInconclusive, context.Canceled}
		if mixed {
			causes = append(causes, failure)
		}
		repo.On("GetApplicationIDBoundary", mock.Anything, mock.Anything).
			Run(func(mock.Arguments) { cancel() }).Return(int64(0), errors.Join(causes...)).Once()
		reschedule, err := s.Tick(ctx)
		require.False(t, reschedule)
		if mixed {
			require.ErrorIs(t, err, failure)
		} else {
			require.NoError(t, err, "shutdown waits must not reach the tick template as operational errors")
		}
		repo.AssertExpectations(t)
	}
}

func TestTournamentObservationShutdownLogs(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, test := range []struct {
		name     string
		stopping bool
		cause    error
		wantLog  bool
	}{
		{shutdownCancellationCase, true, context.Canceled, false},
		{"joined shutdown cancellations", true, errors.Join(context.Canceled, fmt.Errorf("read: %w", context.Canceled)), false},
		{"shutdown database failure", true, dbErr, true},
		{"shutdown mixed database failure", true, fmt.Errorf("read: %w", errors.Join(context.Canceled, dbErr)), true},
		{shutdownDeadlineCase, true, context.DeadlineExceeded, true},
		{shutdownMixedDeadlineCase, true, errors.Join(context.Canceled, context.DeadlineExceeded), true},
		{runningCancellationCase, false, context.Canceled, true},
	} {
		for _, operation := range []string{
			"new descriptor", "stored descriptor", "new standing", "stored standing", "new result", "stored result",
			"projection load", "participant list", "epochs list", "acceptance receipt", "acceptance header", "acceptance update",
		} {
			t.Run(test.name+"/"+operation, func(t *testing.T) {
				s, repo := newPRTServiceMock()
				f := &observerCheckpointFixture{t: t, s: s, repo: repo, factory: &adapterFactoryMock{},
					consensus: &daveConsensusAdapterMock{}, client: &ethClientMock{}, app: prtRevertTestApp()}
				f.app.LastTournamentCheckBlock = 50
				f.s.adapterFactory, f.s.client = f.factory, f.client
				t.Cleanup(func() {
					f.repo.AssertExpectations(t)
					f.factory.AssertExpectations(t)
					f.consensus.AssertExpectations(t)
					f.client.AssertExpectations(t)
				})
				var output bytes.Buffer
				f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if test.stopping {
					cancel()
				}
				err := runObservationWithIOError(ctx, t, f, operation, test.cause)
				f.s.logErrorUnlessShutdown(ctx, "PRT observation", err, "application", f.app.Name)
				require.ErrorIs(t, err, test.cause)
				require.Equal(t, test.wantLog, strings.Contains(output.String(), "level=ERROR"), output.String())
				if test.wantLog {
					require.Contains(t, output.String(), "application="+f.app.Name)
				}
			})
		}
	}
}

func runObservationWithIOError(ctx context.Context, t *testing.T, f *observerCheckpointFixture, operation string, cause error) error {
	t.Helper()
	epoch := checkpointEpoch(0, "0x100")
	if operation == "epochs list" {
		f.repo.On("ListEpochs", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
			Return([]*model.Epoch{}, uint64(0), cause).Once()
		_, err := observeEpochsForTest(ctx, f.s, f.app, 100)
		return err
	}
	if strings.HasPrefix(operation, "acceptance") {
		if operation == "acceptance header" {
			header := &types.Header{Number: big.NewInt(50)}
			f.client.On("HeaderByNumber", mock.Anything, header.Number).Return((*types.Header)(nil), cause).Once()
			return f.s.validateAcceptanceReceipt(ctx, &types.Receipt{BlockHash: header.Hash(), BlockNumber: header.Number})
		}
		epoch.ClaimTransactionHash = new(common.HexToHash("0x900"))
		tournament := &model.Tournament{Address: *epoch.TournamentAddress,
			Snapshot: model.TournamentSnapshot{FinishedAtBlock: 50, AsOfBlock: 50}}
		if operation == "acceptance update" {
			f.app.LastTournamentCheckBlock = 100
			f.acceptance(epoch, tournament, cause)
		} else {
			f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), epoch.TournamentAddress.Hex()).
				Return(tournament, nil).Once()
			f.client.On("TransactionReceipt", mock.Anything, *epoch.ClaimTransactionHash).
				Return((*types.Receipt)(nil), cause).Once()
		}
		_, err := f.s.reconcileAcceptedEpochs(ctx, f.app, []*model.Epoch{epoch}, f.consensus, 100)
		return err
	}

	address := *epoch.TournamentAddress
	adapter := &tournamentAdapterMock{}
	t.Cleanup(func() { adapter.AssertExpectations(t) })
	var stored *model.Tournament
	if !strings.HasPrefix(operation, "new ") {
		stored = &model.Tournament{Address: address, Level: 1, MaxLevel: 3, Kind: model.TournamentKindNonLeaf,
			Snapshot: model.TournamentSnapshot{AsOfBlock: 50}}
	}
	var loadError error
	if operation == "projection load" {
		loadError = cause
	}
	f.repo.On("GetTournament", mock.Anything, f.app.IApplicationAddress.Hex(), address.Hex()).Return(stored, loadError).Once()
	if loadError == nil {
		f.factory.On("CreateTournamentAdapter", address).Return(adapter, nil).Once()
		descriptor := TournamentDescriptor{BaseCycle: big.NewInt(0), Level: 1, Kind: model.TournamentKindNonLeaf}
		var descriptorError error
		if strings.HasSuffix(operation, "descriptor") {
			descriptorError = cause
		}
		adapter.On("Descriptor", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(descriptor, descriptorError).Once()
		if descriptorError == nil {
			var standingError error
			if strings.HasSuffix(operation, standingReadName) {
				standingError = cause
			}
			standing := TournamentStanding{State: model.TournamentStandingAwaitingClosure}
			if strings.HasSuffix(operation, "result") {
				standing = TournamentStanding{State: model.TournamentStandingInnerEliminableWinnerExpired,
					HasCandidate: true, Candidate: *epoch.Commitment, FinishedAt: 90}
				adapter.On("InnerResult", mock.MatchedBy(resultCallOptsAtBlock(100))).
					Return(InnerResult{}, cause).Once()
			}
			adapter.On("Standing", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(standing, standingError).Once()
		}
		if operation == "participant list" {
			expectTournamentAuxiliaryReads(adapter, mock.MatchedBy(resultCallOptsAtBlock(100)), 1, model.TournamentStandingAwaitingClosure)
			adapter.On("RetrieveAllEvents", mock.Anything).Return(&TournamentEvents{}, nil).Once()
			adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(zeroStructuralEventCounts(), nil).Once()
			addressString := address.Hex()
			f.repo.On("ListCommitments", mock.Anything, f.app.Name,
				repository.CommitmentFilter{EpochIndex: &epoch.Index, TournamentAddress: &addressString}, repository.Pagination{}, false).
				Return([]*model.Commitment{}, uint64(0), nil).Once()
			f.repo.On("ListMatches", mock.Anything, f.app.Name,
				repository.MatchFilter{EpochIndex: &epoch.Index, TournamentAddress: &addressString}, repository.Pagination{}, false).
				Return([]*model.Match{}, uint64(0), cause).Once()
		}
	}
	_, err := f.s.gatherTournamentData(ctx, f.app, epoch, 1, nil, nil, address, 3, 100)
	return err
}

func TestTournamentJoinReadsPreserveDependencyErrors(t *testing.T) {
	for _, operation := range []string{"commitment standing", "descriptor", standingReadName, "bond value"} {
		for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("database unavailable")} {
			t.Run(operation+"/"+cause.Error(), func(t *testing.T) {
				f := newDeliveryFixture(t)
				view := resultTestSnapshot(f.epoch, false)
				view.stage.IsFinished = false
				f.snapshot(view)
				adapter := &tournamentAdapterMock{}
				f.factory.On("CreateTournamentAdapter", *f.epoch.TournamentAddress).Return(adapter, nil)
				var readErr error
				if operation == "commitment standing" {
					readErr = cause
				}
				adapter.On("CommitmentStanding", mock.Anything, [32]byte(*f.epoch.Commitment)).Return(CommitmentStanding{}, readErr)
				if readErr == nil {
					if operation == "descriptor" {
						readErr = cause
					}
					adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{Height: model.Log2EpochComputationHashLeafCount,
						StartInstant: 100, Allowance: 30}, readErr)
				}
				if readErr == nil {
					if operation == standingReadName {
						readErr = cause
					}
					adapter.On("Standing", mock.Anything).Return(TournamentStanding{AcceptsJoins: true}, readErr)
				}
				if readErr == nil {
					adapter.On("BondValue", mock.Anything).Return((*big.Int)(nil), cause)
				}
				call, err := f.d.planAction(t.Context(), f.app, joinTournament, f.block, observerFacts(100))
				require.ErrorIs(t, err, cause)
				require.Nil(t, call)
				require.Empty(t, f.backend.published)
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	}
}
