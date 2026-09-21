// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDoNothing(t *testing.T) {
	m, r, _ := newServiceMock(t)
	defer r.AssertExpectations(t)

	prevEpochs := makeEpochMap()
	currEpochs := makeEpochMap()

	transitions, err := m.submitClaimsAndUpdateDatabase(context.Background(), prevEpochs, currEpochs, makeApplicationMap(), big.NewInt(0))
	assert.NoError(t, err)
	assert.Equal(t, 0, transitions, "no transitions when no epochs to process")
}

func TestTickInterleavesStagesWithPinnedBlockAndReschedulesOnProgress(t *testing.T) {
	m, r, b := newServiceMock(t)
	defer r.AssertExpectations(t)
	defer b.AssertExpectations(t)

	err := service.InitTickServiceTemplate(&m.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs:  service.BaseConfigs{Name: "claimer-test"},
		PollInterval: time.Hour,
	}, m)
	require.NoError(t, err)

	tickBlock := big.NewInt(100)
	app := makeApplication()
	currEpoch := makeComputedEpoch(app, 3)
	currEvent := makeSubmittedEvent(app, currEpoch)

	b.On("getDefaultBlockNumber", mock.Anything).
		Return(tickBlock, nil).Once()
	r.On("SelectClaimsToSubmitPerApp", mock.Anything).
		Return(makeEpochMap(), makeEpochMap(currEpoch), makeApplicationMap(app), nil).Once()
	b.On("getConsensusAddress", mock.Anything, app, tickBlock).
		Return(app.IConsensusAddress, nil).Once()
	expectNoForeignClaimAccepted(b, app, currEpoch, currEpoch.LastBlock+1, tickBlock.Uint64())
	b.On("findClaimSubmittedEventAndSucc", mock.Anything, app, currEpoch, currEpoch.LastBlock+1, tickBlock.Uint64()).
		Return(&iconsensus.IConsensus{}, []*iconsensus.IConsensusClaimSubmitted{currEvent}, nil).Once()
	r.On("UpdateEpochWithSubmittedClaim", mock.Anything, app.ID, currEpoch.Index, currEvent.Raw.TxHash).
		Return(nil).Once()

	r.On("SelectClaimsToStagePerApp", mock.Anything).
		Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), nil).Once()
	r.On("SelectClaimsToAcceptPerApp", mock.Anything).
		Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), nil).Once()
	r.On("ListApplications", mock.Anything, mock.MatchedBy(func(f repository.ApplicationFilter) bool {
		return f.Enabled != nil &&
			*f.Enabled &&
			f.ForeclosureRecorded != nil &&
			*f.ForeclosureRecorded &&
			assert.ElementsMatch(t,
				[]model.Consensus{model.Consensus_Authority, model.Consensus_Quorum},
				f.ConsensusTypes,
			)
	}), repository.Pagination{}, false).
		Return([]*model.Application{}, 0, nil).Once()

	reschedule, err := m.Tick(context.Background())

	require.NoError(t, err)
	assert.True(t, reschedule, "a successful stage transition should request an immediate follow-up tick")
}

func TestTickCancellationRequiresCanceledServiceContext(t *testing.T) {
	stages := []string{"getDefaultBlockNumber", "SelectClaimsToSubmitPerApp", "SelectClaimsToStagePerApp", "SelectClaimsToAcceptPerApp"}
	for stage, name := range stages {
		for _, canceled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/canceled=%v", name, canceled), func(t *testing.T) {
				m, r, b := newServiceMock(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if canceled {
					cancel()
				}
				failure := fmt.Errorf("dependency: %w", context.Canceled)
				var rpcErr error
				if stage == 0 {
					rpcErr = failure
				}
				b.On(stages[0], mock.Anything).Return(big.NewInt(100), rpcErr).Once()
				for i := 1; i <= stage; i++ {
					var queryErr error
					if i == stage {
						queryErr = failure
					}
					r.On(stages[i], mock.Anything).Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), queryErr).Once()
				}
				reschedule, err := m.Tick(ctx)
				require.False(t, reschedule)
				if canceled {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, context.Canceled)
				}
				r.AssertExpectations(t)
				b.AssertExpectations(t)
			})
		}
	}
}

func TestTickShutdownErrorClassification(t *testing.T) {
	const canceledState = "canceled"
	dbErr := errors.New("database unavailable")
	for _, state := range []string{"active", canceledState, "deadline exceeded"} {
		for _, test := range []struct {
			name             string
			err              error
			cancellationOnly bool
		}{
			{"cancellation", context.Canceled, true},
			{"wrapped cancellation", fmt.Errorf("query: %w", context.Canceled), true},
			{"joined cancellations", errors.Join(context.Canceled, fmt.Errorf("query: %w", context.Canceled)), true},
			{"database failure", dbErr, false},
			{"deadline", context.DeadlineExceeded, false},
			{"cancellation and deadline", errors.Join(context.Canceled, context.DeadlineExceeded), false},
			{"cancellation and database failure", errors.Join(context.Canceled, dbErr), false},
			{"nested database failure", fmt.Errorf("read: %w", errors.Join(context.Canceled, dbErr)), false},
		} {
			t.Run(state+"/"+test.name, func(t *testing.T) {
				m, _, blockchain := newServiceMock(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				switch state {
				case canceledState:
					cancel()
				case "deadline exceeded":
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
					defer deadlineCancel()
				}
				blockchain.On("getDefaultBlockNumber", ctx).Return(big.NewInt(100), test.err).Once()

				reschedule, err := m.Tick(ctx)

				require.False(t, reschedule)
				if state == canceledState && test.cancellationOnly {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, test.err, "keep the complete operational error")
				}
				blockchain.AssertExpectations(t)
			})
		}
	}
}

// Exercise the receipt-to-database path that shutdown interrupted in the
// snapshot integration test. Also run it through Serve to check the ERROR log.
func TestTickStagingWriteShutdown(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, test := range []struct {
		name       string
		writeErr   error
		queryFails bool
		wantErr    error
	}{
		{"shutdown at next query", context.Canceled, true, nil},
		{"shutdown at tick end", fmt.Errorf("write: %w", context.Canceled), false, nil},
		{"shutdown after completed write", nil, false, nil},
		{"database failure before shutdown", dbErr, true, dbErr},
		{"mixed failure at tick end", errors.Join(context.Canceled, dbErr), false, dbErr},
		{"timeout before shutdown", context.DeadlineExceeded, true, context.DeadlineExceeded},
	} {
		for _, throughServe := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/serve=%t", test.name, throughServe), func(t *testing.T) {
				m, repo, blockchain := newServiceMock(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var logs bytes.Buffer
				m.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
				app := makeApplication()
				epoch := makeComputedEpoch(app, 3)
				computed := makeEpochMap(epoch)
				txHash := common.HexToHash("0x10")
				pending := inFlightTx{txHash: txHash}
				m.claimsInFlight[app.ID] = pending
				tickBlock := big.NewInt(100)
				stagedLog := makeClaimStagedLog(app, epoch)
				stagedLog.BlockNumber = epoch.LastBlock + 1
				blockchain.On("getDefaultBlockNumber", ctx).Return(tickBlock, nil).Once()
				repo.On("SelectClaimsToSubmitPerApp", ctx).
					Return(makeEpochMap(), computed, makeApplicationMap(app), nil).Once()
				blockchain.On("pollTransaction", ctx, txHash, tickBlock).
					Return(true, &types.Receipt{
						TxHash: txHash, Status: types.ReceiptStatusSuccessful,
						BlockNumber: new(big.Int).SetUint64(stagedLog.BlockNumber),
						Logs:        []*types.Log{&stagedLog},
					}, nil).Once()
				repo.On("UpdateEpochThroughStaging", ctx, app.ID, epoch.Index, txHash, stagedLog.BlockNumber).
					Run(func(mock.Arguments) { cancel() }).Return(test.writeErr).Once()
				var queryErr error
				if test.queryFails {
					queryErr = context.Canceled
				}
				repo.On("SelectClaimsToStagePerApp", ctx).
					Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), queryErr).Once()
				if !test.queryFails {
					repo.On("SelectClaimsToAcceptPerApp", ctx).
						Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), nil).Once()
					repo.On("ListApplications", ctx, mock.Anything, repository.Pagination{}, false).
						Return([]*model.Application{}, 0, nil).Once()
				}

				if throughServe {
					require.ErrorIs(t, m.Serve(ctx), context.Canceled)
					if test.wantErr == nil {
						require.NotContains(t, logs.String(), "level=ERROR")
					} else {
						require.Contains(t, logs.String(), "level=ERROR msg=Tick")
						require.Contains(t, logs.String(), test.wantErr.Error())
					}
				} else {
					reschedule, err := m.Tick(ctx)
					require.False(t, reschedule)
					if test.wantErr == nil {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, test.wantErr)
					}
				}
				if test.writeErr == nil {
					require.NotContains(t, m.claimsInFlight, app.ID)
					require.NotContains(t, computed, app.ID)
				} else {
					require.Equal(t, pending, m.claimsInFlight[app.ID], "keep tracking until the database write succeeds")
					require.Contains(t, computed, app.ID)
				}
				repo.AssertExpectations(t)
				blockchain.AssertExpectations(t)
			})
		}
	}
}

func TestTickPreservesEarlierErrorsOnShutdown(t *testing.T) {
	for _, test := range []struct {
		name         string
		failStage    bool
		cancelAccept bool
	}{
		{name: "submit error before stage cancellation"},
		{name: "submit error before accept cancellation", cancelAccept: true},
		{name: "stage error before accept cancellation", failStage: true, cancelAccept: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, repo, blockchain := newServiceMock(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			workErr := errors.New("claim work RPC failed")
			app := makeApplication()
			computed, submitted := makeEpochMap(), makeEpochMap()
			if test.failStage {
				submitted = makeEpochMap(makeSubmittedEpoch(app, 3))
			} else {
				computed = makeEpochMap(makeComputedEpoch(app, 3))
			}
			blockchain.On("getDefaultBlockNumber", ctx).Return(big.NewInt(100), nil).Once()
			blockchain.On("getConsensusAddress", ctx, app, big.NewInt(100)).
				Return(app.IConsensusAddress, workErr).Once()
			repo.On("SelectClaimsToSubmitPerApp", ctx).
				Return(makeEpochMap(), computed, makeApplicationMap(app), nil).Once()
			if test.cancelAccept {
				repo.On("SelectClaimsToStagePerApp", ctx).
					Return(makeEpochMap(), submitted, makeApplicationMap(app), nil).Once()
				repo.On("SelectClaimsToAcceptPerApp", ctx).
					Run(func(mock.Arguments) { cancel() }).
					Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), context.Canceled).Once()
			} else {
				repo.On("SelectClaimsToStagePerApp", ctx).
					Run(func(mock.Arguments) { cancel() }).
					Return(makeEpochMap(), makeEpochMap(), makeApplicationMap(), context.Canceled).Once()
			}

			reschedule, err := m.Tick(ctx)

			require.False(t, reschedule)
			require.ErrorIs(t, err, workErr)
			require.ErrorIs(t, err, context.Canceled, "keep both causes when shutdown accompanies a real error")
			repo.AssertExpectations(t)
			blockchain.AssertExpectations(t)
		})
	}
}
