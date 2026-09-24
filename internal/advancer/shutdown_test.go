// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestAdvancerTickCancellation(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, test := range []struct {
		name     string
		shutdown bool
		cause    error
		quiet    bool
	}{
		{name: "active cancellation", cause: context.Canceled},
		{name: "active mixed failure", cause: errors.Join(context.Canceled, dbErr)},
		{name: "shutdown cancellation", shutdown: true, cause: context.Canceled, quiet: true},
		{name: "wrapped shutdown", shutdown: true, cause: fmt.Errorf("query: %w", context.Canceled), quiet: true},
		{name: "joined shutdown", shutdown: true, cause: errors.Join(context.Canceled, context.Canceled), quiet: true},
		{name: "shutdown database failure", shutdown: true, cause: dbErr},
		{name: "shutdown mixed failure", shutdown: true, cause: errors.Join(context.Canceled, dbErr)},
		{name: "shutdown nested failure", shutdown: true, cause: fmt.Errorf("query: %w", errors.Join(dbErr, context.Canceled))},
		{name: "active deadline", cause: context.DeadlineExceeded},
		{name: "shutdown deadline", shutdown: true, cause: context.DeadlineExceeded},
		{name: "shutdown mixed deadline", shutdown: true, cause: errors.Join(context.Canceled, context.DeadlineExceeded)},
		{name: "active missing machine", cause: ErrNoApp},
		{name: "shutdown missing machine", shutdown: true, cause: ErrNoApp},
		{name: "shutdown missing machine and failure", shutdown: true, cause: errors.Join(ErrNoApp, dbErr)},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo := &MockRepository{}
			mm := newMockMachineManager()
			svc, err := newMockAdvancerService(mm, repo)
			require.NoError(t, err)
			svc.machineManager = &shutdownMachineManager{MockMachineManager: mm, update: func(context.Context) error {
				if test.shutdown {
					cancel()
				}
				return test.cause
			}}
			logs := &advancerLogCapture{}
			svc.Logger = slog.New(logs)

			reschedule, err := svc.Tick(ctx)
			require.False(t, reschedule)
			if test.quiet {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.cause, "preserve the complete dependency error")
			}
			require.Equal(t, test.quiet, logs.contains(slog.LevelDebug, "Tick canceled during shutdown"))
			require.Zero(t, repo.ApplicationStatusUpdates)
			require.False(t, mm.PendingApplicationFailures)
		})
	}
}

func TestAdvancerStepPreservesEarlierFailures(t *testing.T) {
	firstErr := errors.New("first application database failure")
	for _, lastErr := range []error{nil, context.Canceled, errors.Join(context.Canceled, errors.New("second database failure"))} {
		for _, method := range []string{"Step", "Tick"} {
			t.Run(fmt.Sprintf("%s/%v", method, lastErr), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				mm := newMockMachineManager()
				for id := int64(1); id <= 3; id++ {
					mm.Map[id] = newMockInstance(newMockMachine(id))
				}
				baseRepo := &MockRepository{}
				svc, err := newMockAdvancerService(mm, baseRepo)
				require.NoError(t, err)
				var calls []string
				svc.repository = &shutdownEpochRepository{MockRepository: baseRepo, list: func(address string) error {
					calls = append(calls, address)
					if len(calls) == 1 {
						return firstErr
					}
					cancel()
					return lastErr
				}}
				run := svc.Step
				if method == "Tick" {
					run = svc.Tick
				}

				reschedule, err := run(ctx)
				require.False(t, reschedule)
				require.ErrorIs(t, err, firstErr)
				if lastErr != nil {
					require.ErrorIs(t, err, lastErr)
				} else {
					require.ErrorIs(t, err, context.Canceled, "stop before the next application even after a successful read")
				}
				require.Equal(t, []string{
					mm.Map[1].application.IApplicationAddress.Hex(),
					mm.Map[2].application.IApplicationAddress.Hex(),
				}, calls, "do not dispatch another application after cancellation")
				require.Zero(t, baseRepo.ApplicationStatusUpdates)
				require.False(t, mm.PendingApplicationFailures)
			})
		}
	}
}

func TestAdvancerServeReportsShutdownFailures(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		quiet bool
	}{
		{"cancellation", fmt.Errorf("query: %w", context.Canceled), true},
		{"mixed database failure", errors.Join(context.Canceled, errors.New("database unavailable")), false},
		{"deadline", context.DeadlineExceeded, false},
		{"missing machine and failure", errors.Join(ErrNoApp, errors.New("database unavailable")), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			mm := newMockMachineManager()
			svc, err := newMockAdvancerService(mm, &MockRepository{})
			require.NoError(t, err)
			svc.machineManager = &shutdownMachineManager{MockMachineManager: mm, update: func(context.Context) error {
				cancel()
				return test.cause
			}}
			logs := &advancerLogCapture{}
			svc.Logger = slog.New(logs)

			require.ErrorIs(t, svc.Serve(ctx), context.Canceled)
			require.Equal(t, !test.quiet, logs.contains(slog.LevelError, "Tick"))
		})
	}
}

type shutdownMachineManager struct {
	*MockMachineManager
	update func(context.Context) error
}

func (m *shutdownMachineManager) UpdateMachines(ctx context.Context) error {
	return m.update(ctx)
}

type shutdownEpochRepository struct {
	*MockRepository
	list func(string) error
}

func (r *shutdownEpochRepository) ListEpochs(
	_ context.Context, address string, _ repository.EpochFilter, _ repository.Pagination, _ bool,
) ([]*model.Epoch, uint64, error) {
	return nil, 0, r.list(address)
}
