// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	scanInitializeCursor = "initialize cursor"
	scanStoredCount      = "stored count"
	scanChainCounter     = "chain counter"
	scanInputLogs        = "input logs"
	scanEpochLookup      = "epoch lookup"
	scanEpochWrite       = "epoch write"
	scanCursorWrite      = "cursor write"
)

// Exercise the scan caller, not just its error classifier. Cancellation occurs
// inside the failing operation, as it does when shutdown interrupts a DB query.
func TestInputScanErrorReporting(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, operation := range []string{
		scanInitializeCursor, scanStoredCount, scanChainCounter, scanInputLogs, scanEpochLookup, scanEpochWrite, scanCursorWrite,
	} {
		t.Run(operation, func(t *testing.T) {
			for _, test := range []struct {
				name   string
				err    error
				cancel bool
				quiet  bool
			}{
				{name: "shutdown", err: fmt.Errorf("query: %w", context.Canceled), cancel: true, quiet: true},
				{name: "joined shutdown", err: errors.Join(context.Canceled, fmt.Errorf("query: %w", context.Canceled)),
					cancel: true, quiet: true},
				{name: "active context cancellation", err: context.Canceled},
				{name: "database failure", err: dbErr},
				{name: "database failure during shutdown", err: dbErr, cancel: true},
				{name: "timeout", err: context.DeadlineExceeded},
				{name: "timeout during shutdown", err: context.DeadlineExceeded, cancel: true},
				{name: "mixed failure", err: errors.Join(context.Canceled, dbErr), cancel: true},
				{name: "mixed timeout", err: errors.Join(context.Canceled, context.DeadlineExceeded), cancel: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					reader, app, repo, output := inputScanFixture(t, operation, test.err, func(mock.Arguments) {
						if test.cancel {
							cancel()
						}
					})
					require.False(t, reader.scanIConsensusInputs(ctx, []appContracts{app}, 110))
					logs := output.String()
					require.Equal(t, 1, strings.Count(logs, `"msg":"Application input scan interrupted"`), logs)
					if test.quiet {
						require.NotContains(t, logs, `"level":"ERROR"`)
					} else {
						require.Equal(t, 1, strings.Count(logs, `"level":"ERROR"`), logs)
					}
					require.Contains(t, logs, `"application":"input-scan-app"`)
					require.Contains(t, logs, `"address":"`+strings.ToLower(app1Addr.Hex())+`"`)
					require.Contains(t, logs, `"most_recent_block":110`)
					if errors.Is(test.err, dbErr) {
						require.Contains(t, logs, dbErr.Error())
					}
					if errors.Is(test.err, context.DeadlineExceeded) {
						require.Contains(t, logs, context.DeadlineExceeded.Error())
					}
					if operation != scanInitializeCursor && operation != scanCursorWrite {
						repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
					}
					if operation != scanEpochWrite {
						repo.AssertNumberOfCalls(t, "CreateEpochsAndInputs", 0)
					}
				})
			}
		})
	}
}

// Set up exactly one failing operation. Unreached operations have no expectation:
// an attempt to write after a failed read fails the test immediately.
func inputScanFixture(t *testing.T, operation string, failure error, during func(mock.Arguments)) (
	*Service, appContracts, *MockRepository, *bytes.Buffer,
) {
	t.Helper()
	app := appContracts{application: &model.Application{
		ID: 1, Name: "input-scan-app", IApplicationAddress: app1Addr, IInputBoxAddress: inputBoxAddr,
		IInputBoxBlock: 90, EpochLength: 10, LastInputCheckBlock: 100, ConsensusType: model.Consensus_Authority,
		Enabled: true, Status: model.ApplicationStatus_OK,
	}}
	repo := newMockRepository()
	input := newMockInputBox()
	app.inputSource = input
	output := new(bytes.Buffer)
	reader := &Service{repository: repo, defaultBlock: model.DefaultBlock_Latest}
	reader.Logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() {
		repo.AssertExpectations(t)
		input.AssertExpectations(t)
	})
	if operation == scanInitializeCursor {
		app.application.LastInputCheckBlock = 0
		repo.On("UpdateEventLastCheckBlock", mock.Anything, []int64{1}, model.MonitoredEvent_InputAdded, uint64(89)).
			Run(during).Return(failure).Once()
		return reader, app, repo, output
	}
	count := repo.On("GetNumberOfInputs", mock.Anything, app1Addr.Hex()).Return(uint64(0), nil).Once()
	if operation == scanStoredCount {
		count.Run(during).Return(uint64(0), failure)
		return reader, app, repo, output
	}
	chainCount := input.On("GetNumberOfInputs", mock.Anything, app1Addr).Return(big.NewInt(0), nil)
	if operation == scanChainCounter {
		chainCount.Run(during).Return((*big.Int)(nil), failure).Once()
		return reader, app, repo, output
	}
	if operation == scanInputLogs {
		chainCount.Return(big.NewInt(1), nil)
		input.On("RetrieveInputs", mock.Anything, mock.Anything, mock.Anything).
			Run(during).Return([]iinputbox.IInputBoxInputAdded(nil), failure).Once()
		return reader, app, repo, output
	}
	epoch := repo.On("GetEpoch", mock.Anything, app1Addr.Hex(), uint64(10)).Return((*model.Epoch)(nil), nil).Once()
	switch operation {
	case scanEpochLookup:
		epoch.Run(during).Return((*model.Epoch)(nil), failure)
	case scanEpochWrite:
		// Closing an epoch with no new inputs still requires an atomic epoch+cursor write.
		epoch.Return(&model.Epoch{Index: 10, FirstBlock: 100, LastBlock: 109, Status: model.EpochStatus_Open}, nil)
		repo.On("CreateEpochsAndInputs", mock.Anything, app1Addr.Hex(), mock.Anything, uint64(110)).
			Run(during).Return(failure).Once()
	case scanCursorWrite:
		repo.On("UpdateEventLastCheckBlock", mock.Anything, []int64{1}, model.MonitoredEvent_InputAdded, uint64(110)).
			Run(during).Return(failure).Once()
	default:
		t.Fatalf("unknown input-scan operation %q", operation)
	}
	return reader, app, repo, output
}

func TestInputScanHelperPreservesError(t *testing.T) {
	for _, operation := range []string{scanStoredCount, scanChainCounter, scanInputLogs, scanEpochLookup, scanEpochWrite, scanCursorWrite} {
		t.Run(operation, func(t *testing.T) {
			cause := errors.New("operation failed")
			reader, app, _, logs := inputScanFixture(t, operation, cause, func(mock.Arguments) {})
			err := reader.readAndStoreApplicationInputs(t.Context(), 100, 110, app)
			require.ErrorIs(t, err, cause)
			require.NotEqual(t, cause.Error(), err.Error(), "the operation must add context")
			require.NotContains(t, logs.String(), `"level":"ERROR"`, "the caller owns operational error logging")
			require.NotContains(t, logs.String(), "Application input scan interrupted")
		})
	}
}

func TestInputScanClosesEmptyEpochAtomically(t *testing.T) {
	reader, app, repo, _ := inputScanFixture(t, scanEpochWrite, nil, func(args mock.Arguments) {
		epochs := args.Get(2).(map[*model.Epoch][]*model.Input)
		require.Len(t, epochs, 1)
		for epoch, inputs := range epochs {
			require.Equal(t, model.EpochStatus_Closed, epoch.Status)
			require.EqualValues(t, 10, epoch.Index)
			require.Empty(t, inputs)
		}
	})
	require.True(t, reader.scanIConsensusInputs(t.Context(), []appContracts{app}, 110))
	repo.AssertNumberOfCalls(t, "CreateEpochsAndInputs", 1)
	repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
}

func TestInputScanStatusFailureReportedOnce(t *testing.T) {
	repo := newMockRepository()
	repo.On("UpdateApplicationStatus", mock.Anything, int64(1), model.ApplicationStatus_Corrupted, mock.Anything).
		Return(nil).Once()
	app := appContracts{application: &model.Application{
		ID: 1, Name: "invalid-epoch-length", IApplicationAddress: app1Addr,
		Status: model.ApplicationStatus_OK, LastInputCheckBlock: 100,
	}}
	var logs bytes.Buffer
	reader := &Service{repository: repo}
	reader.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	require.True(t, reader.scanIConsensusInputs(t.Context(), []appContracts{app}, 110),
		"the recorded integrity fault is local to this application")
	require.Contains(t, logs.String(), "Application has epoch length of zero")
	require.Equal(t, 1, strings.Count(logs.String(), `"level":"ERROR"`))
	require.NotContains(t, logs.String(), "Application input scan interrupted")
	repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
	repo.AssertExpectations(t)
}

func TestInputScanContinuesAfterApplicationFailure(t *testing.T) {
	reader, failed, repo, logs := inputScanFixture(t, scanEpochLookup, errors.New("epoch lookup failed"), func(mock.Arguments) {})
	healthy := appContracts{application: &model.Application{
		ID: 2, Name: "healthy", IApplicationAddress: app2Addr, EpochLength: 10, LastInputCheckBlock: 100,
	}}
	input := newMockInputBox()
	input.On("GetNumberOfInputs", mock.Anything, app2Addr).Return(big.NewInt(0), nil)
	healthy.inputSource = input
	repo.On("GetNumberOfInputs", mock.Anything, app2Addr.Hex()).Return(uint64(0), nil).Once()
	repo.On("GetEpoch", mock.Anything, app2Addr.Hex(), uint64(10)).Return((*model.Epoch)(nil), nil).Once()
	repo.On("UpdateEventLastCheckBlock", mock.Anything, []int64{2}, model.MonitoredEvent_InputAdded, uint64(110)).
		Return(nil).Once()
	require.False(t, reader.scanIConsensusInputUnit(t.Context(), iConsensusInputScanUnit{
		lastInputCheckBlock: 100, endBlock: 110, apps: []appContracts{failed, healthy},
	}))
	require.Equal(t, 1, strings.Count(logs.String(), `"level":"ERROR"`))
	repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 1)
	input.AssertExpectations(t)
}

func TestInputScanShutdownStopsLaterWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader, app, repo, logs := inputScanFixture(t, scanEpochLookup, context.Canceled, func(mock.Arguments) { cancel() })
	app.application.LastForecloseCheckBlock = 110
	app.application.LastOutputCheckBlock = 100

	// Output observation would query the repository if dispatch continued after shutdown.
	require.False(t, reader.runBlockScanners(ctx, []appContracts{app}, 110))
	repo.AssertNumberOfCalls(t, "GetNumberOfPendingExecutableOutputs", 0)
	repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
	require.NotContains(t, logs.String(), `"level":"ERROR"`)
}

func TestInputScanFailureThenShutdownKeepsEarlierError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader, failed, repo, logs := inputScanFixture(t, scanStoredCount, errors.New("database unavailable"), func(mock.Arguments) {})
	canceled := appContracts{application: &model.Application{
		ID: 2, Name: "canceled", IApplicationAddress: app2Addr, EpochLength: 10,
	}}
	repo.On("GetNumberOfInputs", mock.Anything, app2Addr.Hex()).
		Run(func(mock.Arguments) { cancel() }).Return(uint64(0), context.Canceled).Once()
	// The third scan has no expectations. It must not start after cancellation.
	require.False(t, reader.scanIConsensusInputUnit(ctx, iConsensusInputScanUnit{
		lastInputCheckBlock: 100, endBlock: 110, apps: []appContracts{failed, canceled, failed},
	}))
	require.Equal(t, 1, strings.Count(logs.String(), `"level":"ERROR"`))
	require.Contains(t, logs.String(), "database unavailable")
}

func TestInputScanCancellationDoesNotRefreshReadiness(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader, app, repo, logs := inputScanFixture(t, scanEpochLookup, context.Canceled, func(mock.Arguments) { cancel() })
	app.application.LastForecloseCheckBlock = 110
	app.application.LastOutputCheckBlock = 100
	client := newMockEthClient()
	client.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(110)}, nil).Once()
	reader.client = client
	repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*model.Application{app.application}, uint64(1), nil).Once()
	reader.resolver = newApplicationAdapterResolver(reader.Logger,
		newMockAdapterFactory().SetupDefaultBehaviorSingleApp(newMockApplicationContract(), app.inputSource.(*MockInputBox)))
	previous := time.Now().Add(-time.Minute)
	reader.lastSuccessfulPoll.Store(&previous)
	reader.consecutiveScanFailures.Store(2)
	_, err := reader.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, previous, *reader.lastSuccessfulPoll.Load())
	require.EqualValues(t, 2, reader.consecutiveScanFailures.Load())
	require.NotContains(t, logs.String(), `"level":"ERROR"`)
}
