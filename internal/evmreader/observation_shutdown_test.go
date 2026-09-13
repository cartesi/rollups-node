// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestObservationDatabaseErrorReporting(t *testing.T) {
	const (
		applicationList = "application list"
		pendingOutputs  = "pending outputs"
	)
	dbErr := errors.New("database unavailable")
	for _, operation := range []string{applicationList, pendingOutputs} {
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
					var logs bytes.Buffer
					repo := newMockRepository()
					reader := &Service{repository: repo}
					reader.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
					fail := func(mock.Arguments) {
						if test.cancel {
							cancel()
						}
					}
					switch operation {
					case applicationList:
						repo.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
							Run(fail).Return([]*model.Application(nil), uint64(0), test.err).Once()
						require.False(t, reader.processBlockHead(ctx, 110, nil))
					case pendingOutputs:
						app := appContracts{application: &model.Application{
							ID: 1, Name: "output-scan-app", IApplicationAddress: app1Addr, LastOutputCheckBlock: 100,
						}}
						repo.Unset("GetNumberOfPendingExecutableOutputs")
						repo.On("GetNumberOfPendingExecutableOutputs", mock.Anything, app1Addr.Hex()).
							Run(fail).Return(uint64(0), test.err).Once()
						require.False(t, reader.checkForOutputExecution(ctx, []appContracts{app}, 110))
						require.Contains(t, logs.String(), `"application":"output-scan-app"`)
					}
					require.Equal(t, 1, strings.Count(logs.String(), `"error":`), logs.String())
					if test.quiet {
						require.NotContains(t, logs.String(), `"level":"ERROR"`)
						require.Contains(t, logs.String(), `"level":"DEBUG"`)
					} else {
						require.Equal(t, 1, strings.Count(logs.String(), `"level":"ERROR"`), logs.String())
					}
					if errors.Is(test.err, dbErr) {
						require.Contains(t, logs.String(), dbErr.Error())
					}
					if errors.Is(test.err, context.DeadlineExceeded) {
						require.Contains(t, logs.String(), context.DeadlineExceeded.Error())
					}
					repo.AssertNumberOfCalls(t, "UpdateEventLastCheckBlock", 0)
					repo.AssertNumberOfCalls(t, "UpdateOutputsExecution", 0)
					repo.AssertExpectations(t)
				})
			}
		})
	}
}

func TestOutputScanStopsAfterShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	repo := newMockRepository()
	repo.Unset("GetNumberOfPendingExecutableOutputs")
	repo.On("GetNumberOfPendingExecutableOutputs", mock.Anything, app1Addr.Hex()).
		Run(func(mock.Arguments) { cancel() }).Return(uint64(0), context.Canceled).Once()
	reader := &Service{repository: repo}
	reader.Logger = testLogger(t)
	apps := []appContracts{
		{application: &model.Application{ID: 1, IApplicationAddress: app1Addr, LastOutputCheckBlock: 100}},
		{application: &model.Application{ID: 2, IApplicationAddress: app2Addr, LastOutputCheckBlock: 100}},
	}
	require.False(t, reader.checkForOutputExecution(ctx, apps, 110))
	repo.AssertExpectations(t)
}
