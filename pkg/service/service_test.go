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
	tickCount atomic.Int32
	onTick    func(n int32) // called on each Tick with the tick count (1-based)
}

func (m *mockImpl) Alive() bool       { return true }
func (m *mockImpl) Ready() bool       { return true }
func (m *mockImpl) Reload() []error   { return nil }
func (m *mockImpl) Stop(bool) []error { return nil }
func (m *mockImpl) Tick() []error {
	n := m.tickCount.Add(1)
	if m.onTick != nil {
		m.onTick(n)
	}
	return nil
}

// createTestService creates a Service for testing with the given mock and
// optional reschedule support. It uses a long poll interval so timer ticks
// do not interfere with test assertions.
func createTestService(
	t *testing.T,
	impl *mockImpl,
	enableReschedule bool,
) (*Service, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{}
	err := Create(ctx, &CreateInfo{
		Name:             "test",
		LogLevel:         slog.LevelError,
		Impl:             impl,
		PollInterval:     10 * time.Minute, // long: tests control wakeup explicitly
		Context:          ctx,
		Cancel:           cancel,
		EnableReschedule: enableReschedule,
	}, s)
	require.NoError(t, err)
	return s, cancel
}

type ServeSuite struct {
	suite.Suite
}

func TestServe(t *testing.T) {
	suite.Run(t, new(ServeSuite))
}

func (s *ServeSuite) TestDisabledReschedulePreservesExistingBehavior() {
	// With rescheduling disabled and a short poll interval,
	// Serve() should tick only on timer fires.
	impl := &mockImpl{}
	ctx, cancel := context.WithCancel(context.Background())
	svc := &Service{}
	err := Create(ctx, &CreateInfo{
		Name:         "test-no-resched",
		LogLevel:     slog.LevelError,
		Impl:         impl,
		PollInterval: 20 * time.Millisecond,
		Context:      ctx,
		Cancel:       cancel,
	}, svc)
	s.Require().NoError(err)

	done := make(chan struct{})
	go func() {
		_ = svc.Serve()
		close(done)
	}()

	// Let a few timer ticks fire.
	time.Sleep(90 * time.Millisecond)
	cancel()
	<-done

	// The initial tick + ~3-4 timer ticks at 20ms intervals over 90ms.
	// We just verify it ticked more than once (timer is working) and
	// not an unreasonable number (no busy-loop).
	ticks := impl.tickCount.Load()
	s.GreaterOrEqual(ticks, int32(2), "should have at least 2 ticks (initial + timer)")
	s.LessOrEqual(ticks, int32(10), "should not have an unreasonable number of ticks")
}

func (s *ServeSuite) TestRescheduleTriggersImmediateRetick() {
	// When SignalReschedule() is called from Tick(), Serve() should call
	// Tick() again immediately without waiting for the timer.
	var svc *Service
	impl := &mockImpl{
		onTick: func(n int32) {
			// Signal reschedule on ticks 1 and 2 (the initial tick
			// and the first rescheduled tick). Stop on tick 3.
			if n <= 2 {
				svc.SignalReschedule()
			}
		},
	}

	svc, cancel := createTestService(s.T(), impl, true)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = svc.Serve()
		close(done)
	}()

	// Wait briefly. With a 10-minute poll interval, the only way to get
	// 3 ticks quickly is via SignalReschedule.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	ticks := impl.tickCount.Load()
	s.GreaterOrEqual(ticks, int32(3),
		"should have at least 3 ticks: initial + 2 rescheduled")
}

func (s *ServeSuite) TestRescheduleCoalesces() {
	// Multiple signals while Tick() is running should result in at most
	// one extra tick, not one per signal.
	var svc *Service
	tickStarted := make(chan struct{})
	tickProceed := make(chan struct{})

	impl := &mockImpl{
		onTick: func(n int32) {
			if n == 1 {
				// Signal that the first tick is running.
				close(tickStarted)
				// Block the first tick until the test is ready.
				<-tickProceed
			}
		},
	}

	svc, cancel := createTestService(s.T(), impl, true)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = svc.Serve()
		close(done)
	}()

	// Wait for the first tick to start.
	<-tickStarted

	// Send multiple signals while tick is blocked. Only one fits in the buffer.
	for range 10 {
		svc.SignalReschedule()
	}

	// Let the first tick complete.
	close(tickProceed)

	// Wait for the rescheduled tick to fire, then shut down.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Should be exactly 2 ticks: the initial one + one rescheduled (coalesced).
	ticks := impl.tickCount.Load()
	s.Equal(int32(2), ticks,
		"should have exactly 2 ticks: initial + 1 coalesced reschedule")
}

func (s *ServeSuite) TestContextCancellationExitsPromptly() {
	// When context is cancelled with a reschedule signal pending,
	// Serve() should exit promptly.
	var svc *Service
	impl := &mockImpl{
		onTick: func(_ int32) {
			svc.SignalReschedule()
		},
	}

	svc, cancel := createTestService(s.T(), impl, true)

	done := make(chan struct{})
	go func() {
		_ = svc.Serve()
		close(done)
	}()

	// Let the initial tick fire and signal reschedule.
	time.Sleep(20 * time.Millisecond)
	cancel()

	// Serve() should exit promptly.
	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		s.Fail("Serve() did not exit within 2 seconds after context cancellation")
	}
}

func (s *ServeSuite) TestServeExitsOnContextCancelledBeforeFirstTick() {
	impl := &mockImpl{}

	// Create the service with a live context, then cancel before Serve().
	svc, cancel := createTestService(s.T(), impl, false)
	cancel()

	err := svc.Serve()
	s.NoError(err)
	// No ticks should have fired since context was already cancelled.
	s.Equal(int32(0), impl.tickCount.Load())
}

func (s *ServeSuite) TestRescheduleEnabledCreatesChannel() {
	impl := &mockImpl{}
	svc, cancel := createTestService(s.T(), impl, true)
	defer cancel()

	s.NotNil(svc.reschedule, "reschedule channel should be created when enabled")
}

func (s *ServeSuite) TestRescheduleDisabledLeavesNilChannel() {
	impl := &mockImpl{}
	svc, cancel := createTestService(s.T(), impl, false)
	defer cancel()

	s.Nil(svc.reschedule, "reschedule channel should be nil when disabled")
}

func (s *ServeSuite) TestSignalRescheduleNoopWhenDisabled() {
	impl := &mockImpl{}
	svc, cancel := createTestService(s.T(), impl, false)
	defer cancel()

	// Should not panic on nil channel.
	s.NotPanics(func() { svc.SignalReschedule() })
}

func (s *ServeSuite) TestDrainReschedule() {
	impl := &mockImpl{}
	svc, cancel := createTestService(s.T(), impl, true)
	defer cancel()

	s.False(svc.DrainReschedule(), "should be empty initially")

	svc.SignalReschedule()
	s.True(svc.DrainReschedule(), "should drain pending signal")
	s.False(svc.DrainReschedule(), "should be empty after drain")
}
