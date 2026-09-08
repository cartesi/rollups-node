// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// mockImpl is a minimal ServiceImpl for testing the Serve() loop.
type mockImpl struct {
	TickServiceTemplate
	tickCount atomic.Int32
	onTick    func(n int32) bool // called on each Tick with the tick count (1-based)
}

func (m *mockImpl) Tick(context.Context) (bool, error) {
	n := m.tickCount.Add(1)
	reschedule := false
	if m.onTick != nil {
		reschedule = m.onTick(n)
	}
	return reschedule, nil
}

// createTestService creates a Service for testing with the given mock and
// optional reschedule support. It uses a long poll interval so timer ticks
// do not interfere with test assertions.
func createTestService(
	t *testing.T,
	interval time.Duration,
) *mockImpl {
	t.Helper()

	impl := &mockImpl{}

	err := InitTickServiceTemplate(
		&impl.TickServiceTemplate,
		&TickServiceConfigs{
			BaseConfigs: BaseConfigs{
				Name:     "test",
				LogLevel: slog.LevelError,
			},
			PollInterval: interval,
		},
		impl,
	)
	require.NoError(t, err)

	return impl
}

type ServeSuite struct {
	suite.Suite
}

func TestServe(t *testing.T) {
	suite.Run(t, new(ServeSuite))
}

func (s *ServeSuite) TestItStopsWhenContextIsCanceled() {
	impl := createTestService(s.T(), 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- impl.Serve(ctx)
	}()

	cancel()
	s.Require().ErrorIs(<-errCh, context.Canceled)
}

func (s *ServeSuite) TestDisabledReschedulePreservesExistingBehavior() {
	// With rescheduling disabled and a short poll interval,
	// Serve() should tick only on timer fires.
	impl := createTestService(s.T(), 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- impl.Serve(ctx)
	}()

	// Let a few timer ticks fire.
	time.Sleep(90 * time.Millisecond)

	cancel()
	s.Require().ErrorIs(<-errCh, context.Canceled)

	// The initial tick + ~3-4 timer ticks at 20ms intervals over 90ms.
	// We just verify it ticked more than once (timer is working) and
	// not an unreasonable number (no busy-loop).
	ticks := impl.tickCount.Load()
	s.GreaterOrEqual(ticks, int32(2), "should have at least 2 ticks (initial + timer)")
	s.LessOrEqual(ticks, int32(10), "should not have an unreasonable number of ticks")
}

func (s *ServeSuite) TestRescheduleTriggersImmediateRetick() {
	impl := createTestService(s.T(), 10*time.Minute)

	// When Tick() returns reschedule=true, Serve() should call
	// Tick() again immediately without waiting for the timer.
	impl.onTick = func(n int32) bool {
		// Signal reschedule on ticks 1 and 2 (the initial tick
		// and the first rescheduled tick). Stop on tick 3.
		return n <= 2
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- impl.Serve(ctx)
	}()

	// Wait briefly. With a 10-minute poll interval, the only way to get
	// 3 ticks quickly is via SignalReschedule.
	time.Sleep(100 * time.Millisecond)

	cancel()
	s.Require().ErrorIs(<-errCh, context.Canceled)

	ticks := impl.tickCount.Load()
	s.GreaterOrEqual(ticks, int32(3),
		"should have at least 3 ticks: initial + 2 rescheduled")
}

func (s *ServeSuite) TestContextCancellationExitsPromptly() {
	impl := createTestService(s.T(), 10*time.Minute)

	// When context is cancelled with a reschedule signal pending,
	// Serve() should exit promptly.
	impl.onTick = func(_ int32) bool {
		return true
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- impl.Serve(ctx)
	}()

	// Let the initial tick fire and signal reschedule.
	time.Sleep(20 * time.Millisecond)

	cancel()

	// Serve() should exit promptly.
	select {
	case err := <-errCh:
		s.Require().ErrorIs(err, context.Canceled)
	case <-time.After(2 * time.Second):
		s.Fail("Serve() did not exit within 2 seconds after context cancellation")
	}
}

func (s *ServeSuite) TestServeExitsOnContexCancellationBeforeFirstTick() {
	// Create the service with a live context, then cancel before Serve().
	impl := createTestService(s.T(), 10*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := impl.Serve(ctx)
	s.Require().ErrorIs(err, context.Canceled)

	// No ticks should have fired since context was already cancelled.
	s.Equal(int32(0), impl.tickCount.Load())
}
