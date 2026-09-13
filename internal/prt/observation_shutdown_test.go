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
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	shutdownCancellationCase  = "shutdown cancellation"
	shutdownDeadlineCase      = "shutdown deadline"
	shutdownMixedDeadlineCase = "shutdown cancellation and deadline"
	runningCancellationCase   = "running cancellation"
)

func TestTournamentPublicationShutdown(t *testing.T) {
	dbErr := errors.New("database write failed")
	for _, test := range []struct {
		name    string
		cause   error
		wantErr bool
	}{
		{"canceled write", fmt.Errorf("write: %w", context.Canceled), false},
		{"database failure", dbErr, true},
		{"mixed failure", errors.Join(context.Canceled, dbErr), true},
		{"deadline", context.DeadlineExceeded, true},
	} {
		for _, throughServe := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/serve=%t", test.name, throughServe), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				f := newObserverCheckpointFixture(t)
				var logs bytes.Buffer
				require.NoError(t, service.InitTickServiceTemplate(&f.s.TickServiceTemplate, &service.TickServiceConfigs{
					BaseConfigs: service.BaseConfigs{Name: "prt", Logger: slog.New(slog.NewTextHandler(&logs, nil))},
				}, f.s))
				f.s.defaultBlock = model.DefaultBlock_Finalized
				f.repo.On("ListApplications", ctx, mock.Anything, repository.Pagination{}, false).
					Return([]*model.Application{f.app}, uint64(1), nil).Once()
				f.client.On("HeaderByNumber", ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64())).
					Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
				epoch := checkpointEpoch(0, "0x100")
				f.epochs(epoch)
				f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(uint64(2), nil).Once()
				root := f.tournament(epoch, *epoch.TournamentAddress, 0, 2, 0, 100, &TournamentEvents{}, nil)
				f.children(epoch, root)
				f.repo.On("StoreTournamentEvents", ctx, f.app.ID, mock.Anything, uint64(100)).
					Run(func(mock.Arguments) { cancel() }).Return(test.cause).Once()
				pending := pendingTournamentTransaction{Action: tournamentActionStage, Hash: common.HexToHash("0x999")}
				f.s.pendingTransactions[f.app.ID] = pending

				if throughServe {
					require.ErrorIs(t, f.s.Serve(ctx), context.Canceled)
					require.Equal(t, test.wantErr, strings.Contains(logs.String(), "level=ERROR msg=Tick"), logs.String())
				} else {
					reschedule, err := f.s.Tick(ctx)
					require.False(t, reschedule)
					if test.wantErr {
						require.ErrorIs(t, err, test.cause)
					} else {
						require.NoError(t, err)
					}
				}
				require.Equal(t, uint64(50), f.app.LastTournamentCheckBlock, "an interrupted write must not publish its cursor")
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
				require.Equal(t, model.EpochStatus_ClaimComputed, epoch.Status)
				require.Equal(t, pending, f.s.pendingTransactions[f.app.ID], "failed observation must not release a pending action")
				require.Equal(t, test.wantErr, len(f.s.observationFailures) == 1)
			})
		}
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
			"projection load", "children list", "epochs list", "acceptance receipt", "acceptance update",
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
		_, err := f.s.checkEpochs(ctx, f.app, 100)
		return err
	}
	if strings.HasPrefix(operation, "acceptance") {
		epoch.ClaimTransactionHash = new(common.HexToHash("0x900"))
		tournament := &model.Tournament{Address: *epoch.TournamentAddress, Snapshot: model.TournamentSnapshot{FinishedAtBlock: 50}}
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
		stored = &model.Tournament{Address: address, Level: 1, MaxLevel: 3, Kind: model.TournamentKindNonLeaf}
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
			if strings.HasSuffix(operation, "standing") {
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
		if operation == "children list" {
			expectTournamentAuxiliaryReads(adapter, mock.MatchedBy(resultCallOptsAtBlock(100)), 1, model.TournamentStandingAwaitingClosure)
			adapter.On("RetrieveAllEvents", mock.Anything).Return(&TournamentEvents{}, nil).Once()
			adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(50))).Return(zeroStructuralEventCounts(), nil).Once()
			adapter.On("BondRecovery", mock.MatchedBy(resultCallOptsAtBlock(50))).
				Return(canonicalBondRecovery(model.BondDispositionTournamentRunning, common.Address{}, 0), nil).Once()
			adapter.On("StructuralEventCounts", mock.MatchedBy(resultCallOptsAtBlock(100))).Return(zeroStructuralEventCounts(), nil).Once()
			f.emptyParticipants(epoch, address)
			f.repo.On("ListTournaments", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
				Return([]*model.Tournament{}, uint64(0), cause).Once()
		}
	}
	_, err := f.s.gatherTournamentData(ctx, f.app, epoch, 1, nil, nil, address, 3, 100)
	return err
}

func TestTournamentSubmissionShutdownLogs(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, action := range []struct {
		name string
		run  func(*Service, context.Context, *model.Application, *model.Epoch, error) error
	}{
		{"join", func(s *Service, ctx context.Context, app *model.Application, epoch *model.Epoch, err error) error {
			return s.handleJoinTournamentRevert(ctx, app, epoch, nil, err)
		}},
		{"stage", (*Service).handleStageTournamentResultRevert},
		{"accept", (*Service).handleAcceptTournamentResultRevert},
	} {
		for _, test := range []struct {
			name     string
			stopping bool
			cause    error
			wantLog  bool
		}{
			{shutdownCancellationCase, true, context.Canceled, false},
			{"shutdown mixed failure", true, errors.Join(context.Canceled, dbErr), true},
			{shutdownDeadlineCase, true, context.DeadlineExceeded, true},
			{shutdownMixedDeadlineCase, true, errors.Join(context.Canceled, context.DeadlineExceeded), true},
			{runningCancellationCase, false, context.Canceled, true},
		} {
			t.Run(action.name+"/"+test.name, func(t *testing.T) {
				s, _ := newPRTServiceMock()
				var output bytes.Buffer
				s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if test.stopping {
					cancel()
				}
				err := action.run(s, ctx, prtRevertTestApp(), prtRevertTestEpoch(), test.cause)
				require.ErrorIs(t, err, test.cause)
				require.Equal(t, test.wantLog, strings.Contains(output.String(), "level=ERROR"), output.String())
			})
		}
	}
}

func TestTournamentJoinReadsShutdownLogs(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, operation := range []string{"commitment record", "commitment standing", "bond value"} {
		for _, test := range []struct {
			name     string
			stopping bool
			cause    error
			wantLog  bool
		}{
			{shutdownCancellationCase, true, context.Canceled, false},
			{"shutdown mixed failure", true, errors.Join(context.Canceled, dbErr), true},
			{shutdownDeadlineCase, true, context.DeadlineExceeded, true},
			{shutdownMixedDeadlineCase, true, errors.Join(context.Canceled, context.DeadlineExceeded), true},
			{runningCancellationCase, false, context.Canceled, true},
		} {
			t.Run(operation+"/"+test.name, func(t *testing.T) {
				s, repo := newPRTServiceMock()
				app, epoch := prtRevertTestApp(), resultTestEpoch(model.EpochStatus_ClaimComputed)
				epoch.CommitmentProof = make([]common.Hash, model.Log2EpochComputationHashLeafCount)
				var output bytes.Buffer
				s.Logger = slog.New(slog.NewTextHandler(&output, nil))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if test.stopping {
					cancel()
				}
				var recordErr error
				if operation == "commitment record" {
					recordErr = test.cause
				}
				repo.On("GetCommitment", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index,
					epoch.TournamentAddress.Hex(), epoch.Commitment.Hex()).Return((*model.Commitment)(nil), recordErr).Once()
				if recordErr == nil {
					factory := &adapterFactoryMock{}
					adapter := &tournamentAdapterMock{}
					s.adapterFactory = factory
					factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
					var standingErr error
					if operation == "commitment standing" {
						standingErr = test.cause
					}
					opts := mock.MatchedBy(resultCallOptsAtBlock(100))
					adapter.On("CommitmentStanding", opts, [32]byte(*epoch.Commitment)).Return(CommitmentStanding{}, standingErr).Once()
					if standingErr == nil {
						adapter.On("Descriptor", opts).
							Return(TournamentDescriptor{Height: model.Log2EpochComputationHashLeafCount}, nil).Once()
						adapter.On("BondValue", opts).Return((*big.Int)(nil), test.cause).Once()
					}
					t.Cleanup(func() { factory.AssertExpectations(t); adapter.AssertExpectations(t) })
				}
				_, err := s.reactToTournament(ctx, app, epoch, 100)
				require.ErrorIs(t, err, test.cause)
				require.Equal(t, test.wantLog, strings.Contains(output.String(), "level=ERROR"), output.String())
				repo.AssertExpectations(t)
			})
		}
	}
}
