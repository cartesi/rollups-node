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

func (m *mockImpl) OnReload() []error   { return nil }
func (m *mockImpl) OnStop(bool) []error { return nil }
func (m *mockImpl) Tick(ctx context.Context) (bool, []error) {
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
	impl *mockImpl,
) (IService, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	err := InitTickServiceTemplate(&TickServiceConfigs{
		ServiceConfigs: ServiceConfigs{
			Name:             "test",
			LogLevel:         slog.LevelError,
			Context:          ctx,
			Cancel:           cancel,
		},
		PollInterval:     10 * time.Minute, // long: tests control wakeup explicitly
	}, &impl.TickServiceTemplate, impl, impl)
	require.NoError(t, err)
	return impl, cancel
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
	err := InitTickServiceTemplate(&TickServiceConfigs{
		ServiceConfigs: ServiceConfigs{
			Name:     "test-no-resched",
			LogLevel: slog.LevelError,
			Context:  ctx,
			Cancel:   cancel,
		},
		PollInterval: 20 * time.Millisecond,
	}, &impl.TickServiceTemplate, impl, impl)
	s.Require().NoError(err)

	done := make(chan struct{})
	go func() {
		_ = impl.Serve()
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
	var impl *mockImpl
	impl = &mockImpl{
		onTick: func(n int32) bool {
			// Signal reschedule on ticks 1 and 2 (the initial tick
			// and the first rescheduled tick). Stop on tick 3.
			return n <= 2
		},
	}

	svc, cancel := createTestService(s.T(), impl)
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

func (s *ServeSuite) TestContextCancellationExitsPromptly() {
	// When context is cancelled with a reschedule signal pending,
	// Serve() should exit promptly.
	var impl *mockImpl
	impl = &mockImpl{
		onTick: func(_ int32) bool {
			return true
		},
	}

	svc, cancel := createTestService(s.T(), impl)

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
	svc, cancel := createTestService(s.T(), impl)
	cancel()

	err := svc.Serve()
	s.NoError(err)
	// No ticks should have fired since context was already cancelled.
	s.Equal(int32(0), impl.tickCount.Load())
}

type delayedCloseImpl struct {
	ServiceTemplate
	onServeInitChan chan struct{}
	onStopInitChan chan struct{}
}

func (s *delayedCloseImpl) OnStop(bool) []error {
	<-s.onStopInitChan  // wait signal to initiate stop
	return nil
}

func (s *delayedCloseImpl) OnServe(ctx context.Context) error {
	close(s.onServeInitChan)  // signal service was initiated
	<-ctx.Done()
	return nil
}

func (s *ServeSuite) TestServeExitsAfterStopIsComplete() {
	svc := &delayedCloseImpl{
		onServeInitChan: make(chan struct{}),
		onStopInitChan: make(chan struct{}),
	}

	// Create the service with a live context, then cancel before Serve().
	err := InitServiceTemplate(&ServiceConfigs{
		Name:     "stopOnChanClose",
		LogLevel: slog.LevelError,
	}, &svc.ServiceTemplate, svc)

	onServeEndChan := make(chan error)
	go func() {
		err = svc.Serve()
		onServeEndChan <- err  // signal service ended and provide error
		close(onServeEndChan)
	}()

	onStopEndChan := make(chan []error)
	select {
	case <-svc.onServeInitChan:  // wait service to initiate, so can be stopped.
		// initiate service shutdown through context cancelation
		go func() {
			errs := svc.Stop(true)
			onStopEndChan <- errs  // signal stop ended and provide the errors
			close(onStopEndChan)
		}()
	case <-time.After(2 * time.Second):
		s.Fail("Serve() did not start within 2 seconds")
	}

	// Serve() nor Stop() should not exit just yet.
	select {
	case <-onServeEndChan:
		s.Fail("Serve() exited before 'OnStop' completion")
	case <-onStopEndChan:
		s.Fail("Stop() exited before 'OnStop' completion")
	case <-time.After(100 * time.Millisecond):
		// OK
	}

	close(svc.onStopInitChan)  // signal that stop shall initiate and eventually complete

	// Serve() should exit without errors.
	select {
	case err := <-onServeEndChan:
		s.NoError(err)
	case <-time.After(2 * time.Second):
		s.Fail("Serve() did not exit within 2 seconds after 'OnStop' concluded")
	}

	// Stop() should exit without errors.
	select {
	case errs := <-onStopEndChan:
		s.Empty(errs)
	case <-time.After(2 * time.Second):
		s.Fail("Stop() did not exit within 2 seconds after 'OnStop' concluded")
	}
}

func (s *ServeSuite) TestServeExitsAfterStopIsCompleteOnContextCancelation() {
	svc := &delayedCloseImpl{
		onServeInitChan: make(chan struct{}),
		onStopInitChan: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create the service with a live context, then cancel before Serve().
	err := InitServiceTemplate(&ServiceConfigs{
		Name:     "stopOnChanClose",
		LogLevel: slog.LevelError,
		Context:  ctx,
		Cancel:   cancel,
	}, &svc.ServiceTemplate, svc)

	onServeEndChan := make(chan error)
	go func() {
		err = svc.Serve()
		onServeEndChan <- err  // signal service ended and provide error
		close(onServeEndChan)
	}()

	select {
	case <-svc.onServeInitChan:  // wait service to initiate, so can be stopped.
		cancel()  // initiate service shutdown through context cancelation
	case <-time.After(2 * time.Second):
		s.Fail("Serve() did not start within 2 seconds")
	}

	// Serve() should not exit just yet.
	select {
	case <-onServeEndChan:
		s.Fail("Serve() exited before 'OnStop' completion")
	case <-time.After(100 * time.Millisecond):
		// OK
	}

	close(svc.onStopInitChan)  // signal that stop shall initiate and eventually complete

	// Serve() should exit without errors.
	select {
	case err := <-onServeEndChan:
		s.NoError(err)
	case <-time.After(2 * time.Second):
		s.Fail("Serve() did not exit within 2 seconds after 'OnStop' concluded")
	}
}
