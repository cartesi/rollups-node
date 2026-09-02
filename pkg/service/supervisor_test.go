// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
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
	finalized   chan struct{}
	ready       bool
	duration    time.Duration
	err         error

	serveDone    chan struct{}
	teardownDone chan struct{}
}

func (s *testServiceImpl) reset() {
	s.initialized = make(chan struct{}, 1)
	s.started = make(chan struct{}, 1)
	s.done = make(chan struct{}, 1)
	s.finalized = make(chan struct{}, 1)
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

func (s *testServiceImpl) Teardown() {
	if s.teardownDone != nil {
		<-s.teardownDone
	}
	s.finalized <- struct{}{}
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

func (s *SupervisorSuite) newSupervisor(t *testing.T, logger *slog.Logger, services ...*testServiceImpl) (*Supervisor, error) {
	factories := make([]FactoryFunction, len(services))
	for i, svc := range services {
		factories[i] = func(context.Context, *Supervisor) (SupervisedService, error) {
			svc.initialized <- struct{}{}
			return svc, nil
		}
	}
	return NewSupervisor(t.Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{
			Name:   s.T().Name(),
			Logger: logger,
		},
		Factories: factories,
	})
}

func (s *SupervisorSuite) TestServiceContextsAreCanceledOnStop() {
	child1 := newTestService("child-1")
	child2 := newTestService("child-2")

	supervisor, err := s.newSupervisor(s.T(), nil, child1, child2)
	require.NoError(s.T(), err)

	errCh := asyncCall(supervisor.Serve)

	require.True(s.T(), waitCh(child1.started), "child-1 did not start")
	require.True(s.T(), waitCh(child2.started), "child-2 did not start")

	require.True(s.T(), supervisor.Stop(false))

	require.True(s.T(), waitCh(child1.done), "child-1 did not observe ctx.Done()")
	require.True(s.T(), waitCh(child2.done), "child-2 did not observe ctx.Done()")

	require.True(s.T(), waitCh(child1.finalized), "child1 was not finalized")
	require.True(s.T(), waitCh(child2.finalized), "child2 was not finalized")

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
	require.True(s.T(), supervisor.Stop(false))

	// serve should return quickly
	require.NoError(s.T(), supervisor.Serve())

	// services should not have been started nor terminated
	require.True(s.T(), emptyCh(child.started), "child started")
	require.True(s.T(), emptyCh(child.done), "child concluded")

	// services should however have been finalized
	require.True(s.T(), waitCh(child.finalized), "child did not finalize")
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
		require.ErrorIs(s.T(), err, ErrServiceStopped)
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

	require.True(s.T(), waitCh(healthyChild.finalized), "healthyChild was not finalized")
	require.True(s.T(), waitCh(errorChild.finalized), "errorChild was not finalized")

	select {
	case err := <-errCh:
		require.ErrorIs(s.T(), err, ErrServiceStopped)
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
	require.False(s.T(), supervisor.Ready())

	errCh := asyncCall(supervisor.Serve)

	// when services have started but are not ready, supervisor is alive and not ready
	require.True(s.T(), waitCh(child1.started), "child-1 did not start")
	require.True(s.T(), waitCh(child2.started), "child-2 did not start")
	require.True(s.T(), supervisor.Alive())
	require.False(s.T(), supervisor.Ready())

	// when only some services are ready, supervisor is alive and not ready.
	child1.ready = true
	require.True(s.T(), supervisor.Alive())
	require.False(s.T(), supervisor.Ready())

	// when all services are ready, supervisor is alive and ready.
	child2.ready = true
	require.True(s.T(), supervisor.Alive())
	require.True(s.T(), supervisor.Ready())

	// when one service becomes not ready, supervisor becomes not ready also.
	child2.ready = false
	require.True(s.T(), supervisor.Alive())
	require.False(s.T(), supervisor.Ready())

	// when all services become ready again, supervisor becomes ready again.
	child2.ready = true
	require.True(s.T(), supervisor.Alive())
	require.True(s.T(), supervisor.Ready())

	// when supervisor is stopped, it becomes not alive and not ready immediately.
	require.True(s.T(), supervisor.Stop(false))
	require.False(s.T(), supervisor.Alive())
	require.False(s.T(), supervisor.Ready())

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
	child2.teardownDone = make(chan struct{})

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
	stoppedCh := asyncCall(func() bool { return supervisor.Stop(true) })

	require.True(s.T(), waitCh(child1.done), "child-1 should have stopped")
	require.True(s.T(), emptyCh(child2.done), "child-2 should still be serving")

	// cannot stop nor restart before all services stopped
	require.False(s.T(), supervisor.Stop(false))
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)

	// let last service to stop
	child2.serveDone <- struct{}{}
	require.True(s.T(), waitCh(child2.done), "child-2 should have stopped")

	// cannot stop nor restart before all services are finalized
	require.False(s.T(), supervisor.Stop(false))
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)

	// let last service to be finalized
	child2.teardownDone <- struct{}{}
	require.True(s.T(), waitCh(child2.finalized), "child-2 was not finalized")

	// supervisor shall finish stopping now

	select {
	case err := <-serveCh:
		require.NoError(s.T(), err)
	case <-time.After(2 * time.Second):
		s.Fail("supervisor did not exit after Stop()")
	}

	select {
	case stopped := <-stoppedCh:
		require.True(s.T(), stopped, "supervisor blocking stop didn't report success")
	case <-time.After(2 * time.Second):
		s.Fail("supervisor blocking stop not return after stopping terminated")
	}

	child1.reset()
	child2.reset()

	// cannot stop nor restart even after all services are finalized
	require.False(s.T(), supervisor.Stop(false))
	require.ErrorIs(s.T(), supervisor.Serve(), ErrAlreadyStarted)
}

func (s *SupervisorSuite) TestFailedInitialization() {
	healthyChild := newTestService("healthy-child")
	errorChild := newErrorService("error-child")

	supervisor, err := NewSupervisor(s.T().Context(), &SupervisorConfigs{
		BaseConfigs: BaseConfigs{Name: s.T().Name()},
		Factories: []FactoryFunction{
			func(context.Context, *Supervisor) (SupervisedService, error) {
				return healthyChild, nil
			},
			func(context.Context, *Supervisor) (SupervisedService, error) {
				return nil, fmt.Errorf("failed initialization")
			},
			func(context.Context, *Supervisor) (SupervisedService, error) {
				return errorChild, nil
			},
		},
	})
	require.Nil(s.T(), supervisor)
	require.ErrorIs(s.T(), err, ErrServiceBadInit)

	waitCh(healthyChild.finalized)
	waitCh(errorChild.finalized)

	emptyCh(healthyChild.started)
	emptyCh(errorChild.started)
}
