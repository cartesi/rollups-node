// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"
)

func TestValidatorTickShutdownErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, operation := range []string{"applications", "epochs", "outputs", "claim write"} {
		for _, stopping := range []bool{false, true} {
			for _, test := range []struct {
				name             string
				cause            error
				cancellationOnly bool
			}{
				{"cancellation", context.Canceled, true},
				{"wrapped cancellation", fmt.Errorf("query: %w", context.Canceled), true},
				{"joined cancellations", errors.Join(context.Canceled, fmt.Errorf("query: %w", context.Canceled)), true},
				{"database query failure", dbErr, false},
				{"deadline", context.DeadlineExceeded, false},
				{"mixed deadline", errors.Join(context.Canceled, context.DeadlineExceeded), false},
				{"mixed database failure", errors.Join(context.Canceled, dbErr), false},
				{"nested database failure", fmt.Errorf("query: %w", errors.Join(context.Canceled, dbErr)), false},
			} {
				t.Run(fmt.Sprintf("%s/stopping=%t/%s", operation, stopping, test.name), func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					s, logs := validatorShutdownFixture(ctx, t, operation, test.cause, func(mock.Arguments) {
						if stopping {
							cancel()
						}
					})

					reschedule, err := s.Tick(ctx)
					require.False(t, reschedule)
					if stopping && test.cancellationOnly {
						require.NoError(t, err)
						require.NotContains(t, logs.String(), "level=ERROR")
						require.NotContains(t, logs.String(), "level=WARN")
					} else {
						require.ErrorIs(t, err, test.cause, "preserve all causes, not just one joined error")
					}
				})
			}
		}
	}
}

func TestValidatorClaimWriteShutdownLogs(t *testing.T) {
	dbErr := errors.New("database write failed")
	for _, test := range []struct {
		name    string
		cause   error
		wantLog bool
	}{
		{"canceled write", fmt.Errorf("write: %w", context.Canceled), false},
		{"completed write", nil, false},
		{"database failure", dbErr, true},
		{"mixed failure", errors.Join(context.Canceled, dbErr), true},
		{"deadline", context.DeadlineExceeded, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s, logs := validatorShutdownFixture(ctx, t, "claim write", test.cause, func(mock.Arguments) { cancel() })
			require.ErrorIs(t, s.Serve(ctx), context.Canceled)
			if test.wantLog {
				require.Contains(t, logs.String(), "level=ERROR msg=Tick")
				if errors.Is(test.cause, dbErr) {
					require.Contains(t, logs.String(), dbErr.Error())
				} else {
					require.Contains(t, logs.String(), test.cause.Error())
				}
			} else {
				require.NotContains(t, logs.String(), "level=ERROR")
			}
		})
	}
}

// Cancel from the failing operation, after Tick has started. No status-write
// expectation is installed: an I/O failure must not change application health.
func validatorShutdownFixture(
	ctx context.Context, t *testing.T, operation string, cause error, hook func(mock.Arguments),
) (*Service, *bytes.Buffer) {
	t.Helper()
	repo := newMockrepo()
	postContext := merkle.CreatePostContext()
	s := &Service{repository: repo, pristinePostContext: postContext, pristineRootHash: postContext[merkle.TREE_DEPTH]}
	logs := new(bytes.Buffer)
	require.NoError(t, service.InitTickServiceTemplate(&s.TickServiceTemplate, &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{Name: "validator", Logger: slog.New(slog.NewTextHandler(logs, nil))},
	}, s))
	app := &model.Application{ID: 1, Name: "validator-app", Status: model.ApplicationStatus_OK}
	t.Cleanup(func() {
		repo.AssertExpectations(t)
		require.Equal(t, model.ApplicationStatus_OK, app.Status)
	})
	apps := repo.On("ListApplications", ctx, mock.Anything, mock.Anything, false).Once()
	if operation == "applications" {
		apps.Run(hook).Return([]*model.Application(nil), uint64(0), cause)
		return s, logs
	}
	apps.Return([]*model.Application{app}, uint64(1), nil)
	epochs := repo.On("ListEpochs", ctx, app.IApplicationAddress.Hex(), mock.Anything, mock.Anything, false).Once()
	if operation == "epochs" {
		epochs.Run(hook).Return([]*model.Epoch(nil), uint64(0), cause)
		return s, logs
	}
	epoch := &model.Epoch{Status: model.EpochStatus_InputsProcessed,
		MachineHash: new(common.HexToHash("0x123")), TxBufferDataBlock: &s.pristineRootHash}
	epochs.Return([]*model.Epoch{epoch}, uint64(1), nil)
	outputs := repo.On("ListOutputs", ctx, app.IApplicationAddress.Hex(), mock.Anything, mock.Anything, false).Once()
	if operation == "outputs" {
		outputs.Run(hook).Return([]*model.Output(nil), uint64(0), cause)
		return s, logs
	}
	outputs.Return([]*model.Output(nil), uint64(0), nil)
	repo.On("GetLastInput", ctx, app.IApplicationAddress.Hex(), epoch.Index).
		Return(&model.Input{MachineHash: epoch.MachineHash, TxBufferDataBlock: epoch.TxBufferDataBlock}, nil).Once()
	repo.On("StoreClaimAndProofs", ctx, epoch, mock.Anything).Run(hook).Return(cause).Once()
	return s, logs
}

func TestValidatorTickShutdownRetainsEarlierError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	repo := newMockrepo()
	s := &Service{repository: repo}
	s.Logger = slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
	apps := []*model.Application{
		{IApplicationAddress: common.HexToAddress("0x1")},
		{IApplicationAddress: common.HexToAddress("0x2")},
		{IApplicationAddress: common.HexToAddress("0x3")},
	}
	repo.On("ListApplications", ctx, mock.Anything, mock.Anything, false).Return(apps, uint64(len(apps)), nil).Once()
	firstErr := errors.New("first application database read failed")
	repo.On("ListEpochs", ctx, apps[0].IApplicationAddress.Hex(), mock.Anything, mock.Anything, false).
		Return([]*model.Epoch(nil), uint64(0), firstErr).Once()
	repo.On("ListEpochs", ctx, apps[1].IApplicationAddress.Hex(), mock.Anything, mock.Anything, false).
		Run(func(mock.Arguments) { cancel() }).Return([]*model.Epoch(nil), uint64(0), context.Canceled).Once()

	reschedule, err := s.Tick(ctx)
	require.False(t, reschedule)
	require.ErrorIs(t, err, firstErr)
	repo.AssertNotCalled(t, "ListEpochs", ctx, apps[2].IApplicationAddress.Hex(), mock.Anything, mock.Anything, false)
	repo.AssertExpectations(t)
}

func TestValidatorTickDoneContext(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%t", deadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(t.Context(), time.Time{})
			}
			defer cancel()
			cancel()
			repo := newMockrepo()
			s := &Service{repository: repo}
			s.Logger = slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
			reschedule, err := s.Tick(ctx)
			require.False(t, reschedule)
			if deadline {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, repo.Calls, "a stopped tick must not query the database")
		})
	}
}
