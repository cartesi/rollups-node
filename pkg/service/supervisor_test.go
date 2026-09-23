// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type testServiceImpl struct {
	BaseTemplate
	initialized chan struct{}
	started     chan struct{}
	done        chan struct{}
	ready       bool
	duration    time.Duration
	err         error

	serveDone chan struct{}
}

func (s *testServiceImpl) reset() {
	s.initialized = make(chan struct{}, 1)
	s.started = make(chan struct{}, 1)
	s.done = make(chan struct{}, 1)
	s.ready = false
}

func (s *testServiceImpl) Ready() bool {
	return s.ready
}

func (s *testServiceImpl) Serve(ctx context.Context) error {

	s.started <- struct{}{}

	err := s.err
	select {
	case <-time.After(s.duration):
	case <-ctx.Done():
		if err == nil {
			err = ctx.Err()
		}
	}

	if s.serveDone != nil {
		<-s.serveDone
	}

	s.done <- struct{}{}

	return err
}

func newTestService(name string) *testServiceImpl {
	svc := &testServiceImpl{duration: 10 * time.Minute}
	InitServiceTemplate(&svc.BaseTemplate, &BaseConfigs{Name: name})
	svc.reset()
	return svc
}

func newErrorService(name string) *testServiceImpl {
	svc := newTestService(name)
	svc.duration = 10 * time.Millisecond
	svc.err = fmt.Errorf("oops by %s", name)
	return svc
}

func asyncCall[R any](f func() R) <-chan R {
	ch := make(chan R)
	go func() { ch <- f() }()
	return ch
}

func waitCh(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

func emptyCh(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return false
	default:
		return true
	}
}

type SupervisorSuite struct {
	suite.Suite
}

func TestSupervisor(t *testing.T) {
	suite.Run(t, new(SupervisorSuite))
}

func (s *SupervisorSuite) newSupervisor(t *testing.T, logger *slog.Logger, services ...*testServiceImpl) (*supervisorImpl, error) {
	factories := make([]FactoryFunction, len(services))
	for i, svc := range services {
		factories[i] = func(context.Context, Supervisor) (SupervisedService, error) {
			svc.initialized <- struct{}{}
			return svc, nil
		}
	}
	supervisor, err := NewSupervisor(t.Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{
			Name:   s.T().Name(),
			Logger: logger,
		},
		Factories: factories,
	})
	if err != nil {
		return nil, err
	}
	return supervisor.(*supervisorImpl), nil
}

func (s *SupervisorSuite) TestServiceContextsAreCanceledOnStop() {
	child1 := newTestService("child-1")
	child2 := newTestService("child-2")

	supervisor, err := s.newSupervisor(s.T(), nil, child1, child2)
	require.NoError(s.T(), err)

	errCh := asyncCall(supervisor.Serve)

	require.True(s.T(), waitCh(child1.started), "child-1 did not start")
	require.True(s.T(), waitCh(child2.started), "child-2 did not start")

	require.True(s.T(), supervisor.Stop())

	require.True(s.T(), waitCh(child1.done), "child-1 did not observe ctx.Done()")
	require.True(s.T(), waitCh(child2.done), "child-2 did not observe ctx.Done()")

	select {
	case err := <-errCh:
		require.NoError(s.T(), err)
	case <-time.After(2 * time.Second):
		s.Fail("supervisor did not exit after Stop()")
	}
}

func (s *SupervisorSuite) TestItDoesNotRunAfterItIsStopped() {
	child := newTestService("child")

	supervisor, err := s.newSupervisor(s.T(), nil, child)
	require.NoError(s.T(), err)
	require.True(s.T(), waitCh(child.initialized), "child did not initialize")

	// enqueue a stop request
	require.True(s.T(), supervisor.Stop())

	// serve should return quickly
	require.NoError(s.T(), supervisor.Serve())

	// services should not have been started nor terminated
	require.True(s.T(), emptyCh(child.started), "child started")
	require.True(s.T(), emptyCh(child.done), "child concluded")
}

func (s *SupervisorSuite) TestItLogsServiceErrors() {
	child1 := newErrorService("child-1")
	child2 := newErrorService("child-2")

	var buf bytes.Buffer
	logger := captureLogger(&buf)
	supervisor, err := s.newSupervisor(s.T(), logger, child1, child2)
	require.NoError(s.T(), err)

	errCh := asyncCall(supervisor.Serve)

	require.True(s.T(), waitCh(child1.started), "child-1 did not start")
	require.True(s.T(), waitCh(child2.started), "child-2 did not start")

	select {
	case err := <-errCh:
		require.ErrorContains(s.T(), err, "oops by child-1")
		require.ErrorContains(s.T(), err, "oops by child-2")
		logged := buf.String()
		require.Contains(s.T(), logged, "oops by child-1")
		require.Contains(s.T(), logged, "oops by child-2")
	case <-time.After(2 * time.Second):
		s.Fail("supervisor did not exit after child errors")
	}
}

func (s *SupervisorSuite) TestItStopsWhenOnServiceError() {
	healthyChild := newTestService("healthy-child")
	errorChild := newErrorService("error-child")

	var buf bytes.Buffer
	logger := captureLogger(&buf)
	supervisor, err := s.newSupervisor(s.T(), logger, healthyChild, errorChild)
	require.NoError(s.T(), err)

	errCh := asyncCall(supervisor.Serve)

	require.True(s.T(), waitCh(healthyChild.started), "healthyChild did not start")
	require.True(s.T(), waitCh(errorChild.started), "errorChild did not start")

	require.True(s.T(), waitCh(healthyChild.done), "healthyChild did not terminate")
	require.True(s.T(), waitCh(errorChild.done), "errorChild did not terminate")

	select {
	case err := <-errCh:
		require.ErrorContains(s.T(), err, "oops by error-child")
		logged := buf.String()
		require.Contains(s.T(), logged, "oops by error-child")
	case <-time.After(2 * time.Second):
		s.Fail("supervisor did not exit after child error")
	}
}

func (s *SupervisorSuite) TestLifecycleIndications() {
	child1 := newTestService("child-1")
	child2 := newTestService("child-2")

	supervisor, err := s.newSupervisor(s.T(), nil, child1, child2)
	require.NoError(s.T(), err)

	// before serving, supervisor is not alive and not ready
	require.False(s.T(), supervisor.Alive())
	require.NotEmpty(s.T(), supervisor.NotReady())

	errCh := asyncCall(supervisor.Serve)

	// when services have started but are not ready, supervisor is alive and not ready
	require.True(s.T(), waitCh(child1.started), "child-1 did not start")
	require.True(s.T(), waitCh(child2.started), "child-2 did not start")
	require.True(s.T(), supervisor.Alive())
	require.NotEmpty(s.T(), supervisor.NotReady())

	// when only some services are ready, supervisor is alive and not ready.
	child1.ready = true
	require.True(s.T(), supervisor.Alive())
	require.NotEmpty(s.T(), supervisor.NotReady())

	// when all services are ready, supervisor is alive and ready.
	child2.ready = true
	require.True(s.T(), supervisor.Alive())
	require.Empty(s.T(), supervisor.NotReady())

	// when one service becomes not ready, supervisor becomes not ready also.
	child2.ready = false
	require.True(s.T(), supervisor.Alive())
	require.NotEmpty(s.T(), supervisor.NotReady())

	// when all services become ready again, supervisor becomes ready again.
	child2.ready = true
	require.True(s.T(), supervisor.Alive())
	require.Empty(s.T(), supervisor.NotReady())

	// when supervisor is stopped, it becomes not alive and not ready immediately.
	require.True(s.T(), supervisor.Stop())
	require.False(s.T(), supervisor.Alive())
	require.NotEmpty(s.T(), supervisor.NotReady())

	select {
	case err := <-errCh:
		require.NoError(s.T(), err)
	case <-time.After(2 * time.Second):
		s.Fail("supervisor did not exit after Stop()")
	}

	child1.reset()
	child2.reset()
}

func (s *SupervisorSuite) TestWaitingForStop() {
	child1 := newTestService("child-1")
	child2 := newTestService("child-2")
	child2.serveDone = make(chan struct{})

	supervisor, err := s.newSupervisor(s.T(), nil, child1, child2)
	require.NoError(s.T(), err)

	// supervisor is now starting
	serveCh := asyncCall(supervisor.Serve)

	require.True(s.T(), waitCh(child1.started), "child-1 did not start")

	// cannot restart while stating
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)

	// supervisor is now fully started
	require.True(s.T(), waitCh(child2.started), "child-2 did not start")

	// cannot restart before it stops
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)

	// let services execute for a short while before stopping
	time.Sleep(200 * time.Millisecond)

	// supervisor is now stopping
	supervisor.Stop()

	require.True(s.T(), waitCh(child1.done), "child-1 should have stopped")
	require.True(s.T(), emptyCh(child2.done), "child-2 should still be serving")

	// cannot stop nor restart before all services stopped
	require.False(s.T(), supervisor.Stop())
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)

	// let last service to stop
	child2.serveDone <- struct{}{}
	require.True(s.T(), waitCh(child2.done), "child-2 should have stopped")

	// cannot stop nor restart before all services are finalized
	require.False(s.T(), supervisor.Stop())
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)

	// supervisor shall finish stopping now

	select {
	case err := <-serveCh:
		require.NoError(s.T(), err)
	case <-time.After(2 * time.Second):
		s.Fail("supervisor did not exit after Stop()")
	}

	child1.reset()
	child2.reset()

	// cannot stop nor restart even after all services are finalized
	require.False(s.T(), supervisor.Stop())
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)
}

func (s *SupervisorSuite) TestFailedInitialization() {
	healthyChild := newTestService("healthy-child")
	errorChild := newErrorService("error-child")

	supervisor, err := NewSupervisor(s.T().Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{Name: s.T().Name()},
		Factories: []FactoryFunction{
			func(context.Context, Supervisor) (SupervisedService, error) {
				return healthyChild, nil
			},
			func(context.Context, Supervisor) (SupervisedService, error) {
				return nil, fmt.Errorf("failed initialization")
			},
			func(context.Context, Supervisor) (SupervisedService, error) {
				return errorChild, nil
			},
		},
	})
	require.Nil(s.T(), supervisor)
	require.ErrorIs(s.T(), err, ErrServiceBadInit)

	require.True(s.T(), emptyCh(healthyChild.started), "healthyChild did not start")
	require.True(s.T(), emptyCh(errorChild.started), "errorChild did not start")
}

func TestInitializationFailureJoinsFactoriesAndCleansUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	cause := errors.New("database unavailable")
	early := newTestService("early")
	late := newTestService("late")
	earlyBuilt := make(chan struct{})
	var captured *supervisorImpl
	var canceledSibling bool
	sup, err := NewSupervisor(ctx, &SupervisorConfigs{
		BaseConfigs: BaseConfigs{Name: "test", Logger: discardLogger()},
		Factories: []FactoryFunction{
			func(context.Context, Supervisor) (SupervisedService, error) {
				close(earlyBuilt)
				return early, nil
			},
			func(context.Context, Supervisor) (SupervisedService, error) {
				<-earlyBuilt
				// An errored factory may return a typed nil; it must not be collected.
				var failed *testServiceImpl
				return failed, cause
			},
			func(ctx context.Context, s Supervisor) (SupervisedService, error) {
				captured = s.(*supervisorImpl)
				<-ctx.Done()
				// Construction can finish successfully despite sibling cancellation.
				return late, nil
			},
			func(ctx context.Context, _ Supervisor) (SupervisedService, error) {
				<-ctx.Done()
				canceledSibling = true
				return nil, ctx.Err()
			},
		},
	})
	require.Nil(t, sup)
	require.ErrorIs(t, err, ErrServiceBadInit)
	require.ErrorIs(t, err, cause)
	require.ErrorContains(t, err, "factory 1")
	require.True(t, canceledSibling, "constructor must join canceled factories")
	require.ErrorIs(t, captured.context.Err(), context.Canceled)
	for _, child := range []*testServiceImpl{early, late} {
		require.Empty(t, child.started)
	}
}

func TestInitializationKeepsFactoryOrder(t *testing.T) {
	first := newTestService("first")
	second := newTestService("second")
	secondBuilt := make(chan struct{})
	sup, err := NewSupervisor(t.Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{Logger: discardLogger()},
		Factories: []FactoryFunction{
			func(context.Context, Supervisor) (SupervisedService, error) {
				<-secondBuilt
				return first, nil
			},
			func(context.Context, Supervisor) (SupervisedService, error) {
				close(secondBuilt)
				return second, nil
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []SupervisedService{first, second}, sup.(*supervisorImpl).services)
}

func TestFatalShutdownPreservesCauseAndWaitsForServices(t *testing.T) {
	child := newTestService("child")
	child.serveDone = make(chan struct{})
	sup, err := NewSupervisor(t.Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{Logger: discardLogger()},
		Factories:   []FactoryFunction{func(context.Context, Supervisor) (SupervisedService, error) { return child, nil }},
	})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- sup.Serve() }()
	require.True(t, waitCh(child.started))
	cause := errors.New("unconfirmed advance result")
	sup.Fatal(cause)
	later := errors.New("later failure")
	sup.Fatal(later)
	require.False(t, sup.Alive())
	select {
	case <-done:
		t.Error("Serve returned before the service finished shutdown")
	default:
	}
	close(child.serveDone)
	select {
	case err := <-done:
		require.ErrorIs(t, err, cause)
		require.ErrorIs(t, err, later)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not finish shutdown")
	}
}

func TestFatalBeforeServe(t *testing.T) {
	for _, cause := range []error{errors.New("fatal initialization result"), nil} {
		child := newTestService("child")
		sup, err := NewSupervisor(t.Context(), &SupervisorConfigs{
			BaseConfigs: BaseConfigs{Logger: discardLogger()},
			Factories:   []FactoryFunction{func(context.Context, Supervisor) (SupervisedService, error) { return child, nil }},
		})
		require.NoError(t, err)
		sup.Fatal(cause)
		if cause == nil {
			cause = ErrServiceStopped
		}
		require.ErrorIs(t, sup.Serve(), cause)
		require.Empty(t, child.started)
	}
}

func TestSupervisorRejectsEmptyFactoryList(t *testing.T) {
	for _, telemetry := range []string{"", "localhost:0"} {
		sup, err := NewSupervisor(t.Context(), &SupervisorConfigs{TelemetryAddress: telemetry})
		require.Nil(t, sup)
		require.ErrorIs(t, err, ErrServiceBadInit)
		require.ErrorContains(t, err, "at least one service factory")
	}
}

func TestConcurrentFatalCallsPreserveAllCauses(t *testing.T) {
	child := newTestService("child")
	child.serveDone = make(chan struct{})
	sup, err := NewSupervisor(t.Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{Logger: discardLogger()},
		Factories:   []FactoryFunction{func(context.Context, Supervisor) (SupervisedService, error) { return child, nil }},
	})
	require.NoError(t, err)
	t.Cleanup(func() { sup.Stop(); close(child.serveDone) })
	done := make(chan error, 1)
	go func() { done <- sup.Serve() }()
	require.True(t, waitCh(child.started))

	causes := make([]error, 64)
	start := make(chan struct{})
	var calls sync.WaitGroup
	for i := range causes {
		if i != 0 {
			causes[i] = fmt.Errorf("fatal failure %d", i)
		}
		calls.Add(1)
		go func(cause error) {
			defer calls.Done()
			<-start
			sup.Fatal(cause)
		}(causes[i])
	}
	close(start)
	calls.Wait()
	require.False(t, sup.Alive())
	child.serveDone <- struct{}{}
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrServiceStopped, "nil Fatal calls retain the default cause")
		for _, cause := range causes[1:] {
			require.ErrorIs(t, err, cause)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not finish shutdown")
	}
}

func (s *SupervisorSuite) TestUnexpectedExitPreservesServiceAndCause() {
	for _, cause := range []error{nil, errors.New("bind failed"), context.Canceled} {
		s.Run(fmt.Sprintf("cause=%v", cause), func() {
			child := newTestService("unexpected-child")
			child.duration = 0
			child.err = cause
			sup, err := s.newSupervisor(s.T(), discardLogger(), child)
			s.Require().NoError(err)
			err = sup.Serve()
			if cause == nil {
				s.Require().ErrorIs(err, ErrServiceStopped)
			} else {
				s.Require().ErrorIs(err, cause)
			}
		})
	}
}

func (s *SupervisorSuite) TestConcurrentDrainErrorsPreserveAllCauses() {
	for _, fatal := range []bool{false, true} {
		s.Run(fmt.Sprintf("fatal=%v", fatal), func() {
			children := make([]*testServiceImpl, 16)
			release := make(chan struct{})
			for i := range children {
				child := newTestService(fmt.Sprintf("draining-child-%d", i))
				child.err = fmt.Errorf("drain failure %d", i)
				child.serveDone = release
				children[i] = child
			}
			children[0].err = context.DeadlineExceeded
			children[1].err = fmt.Errorf("normal shutdown: %w", context.Canceled)
			sup, err := s.newSupervisor(s.T(), discardLogger(), children...)
			s.Require().NoError(err)
			s.T().Cleanup(func() { sup.Stop() })
			done := make(chan error, 1)
			go func() { done <- sup.Serve() }()
			for _, child := range children {
				s.Require().True(waitCh(child.started))
			}
			fatalCause := errors.New("fatal trigger")
			if fatal {
				sup.Fatal(fatalCause)
			} else {
				sup.Stop()
			}
			close(release)
			select {
			case err := <-done:
				s.Require().NotErrorIs(err, context.Canceled)
				if fatal {
					s.Require().ErrorIs(err, fatalCause)
				}
				for i, child := range children {
					if i == 1 {
						continue
					}
					s.Require().ErrorIs(err, child.err)
				}
			case <-time.After(2 * time.Second):
				s.T().Fatal("supervisor did not finish draining")
			}
		})
	}
}

func (s *SupervisorSuite) TestShutdownSuppressesOnlyCancellationErrors() {
	storageErr := errors.New("storage flush failed")
	otherErr := errors.New("connection close failed")
	wrappedCancellation := fmt.Errorf("shutdown: %w", context.Canceled)
	for _, tc := range []struct {
		name string
		err  error
		want []error
	}{
		{name: "nil"},
		{name: "cancellation", err: context.Canceled},
		{name: "wrapped cancellation", err: wrappedCancellation},
		{name: "joined cancellations", err: errors.Join(context.Canceled, wrappedCancellation)},
		{name: "wrapped joined cancellations", err: fmt.Errorf("drain: %w", errors.Join(context.Canceled, wrappedCancellation))},
		{name: "storage failure", err: storageErr, want: []error{storageErr}},
		{name: "deadline", err: context.DeadlineExceeded, want: []error{context.DeadlineExceeded}},
		{name: "joined storage failure", err: errors.Join(context.Canceled, storageErr), want: []error{storageErr}},
		{name: "joined deadline", err: errors.Join(context.Canceled, context.DeadlineExceeded), want: []error{context.DeadlineExceeded}},
		{name: "nested failures", err: fmt.Errorf("drain: %w", errors.Join(wrappedCancellation, errors.Join(storageErr, otherErr))), want: []error{storageErr, otherErr}},
		{name: "multiple wrapped causes", err: fmt.Errorf("drain: %w; storage: %w", context.Canceled, storageErr), want: []error{storageErr}},
	} {
		s.Run(tc.name, func() {
			child := newTestService("draining-child")
			child.err = tc.err
			sup, err := s.newSupervisor(s.T(), discardLogger(), child)
			s.Require().NoError(err)
			s.T().Cleanup(func() { sup.Stop() })
			done := make(chan error, 1)
			go func() { done <- sup.Serve() }()
			s.Require().True(waitCh(child.started))
			sup.Stop()
			select {
			case err := <-done:
				if len(tc.want) == 0 {
					s.Require().NoError(err)
				} else {
					for _, cause := range tc.want {
						s.Require().ErrorIs(err, cause)
					}
					// Keep the original error and its diagnostic wrapping intact.
					s.Require().ErrorIs(err, tc.err)
				}
			case <-time.After(2 * time.Second):
				s.T().Fatal("supervisor did not finish shutdown")
			}
		})
	}
}
