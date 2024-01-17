// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SupervisorServiceSuite struct {
	suite.Suite
}

func TestSupervisorService(t *testing.T) {
	suite.Run(t, new(SupervisorServiceSuite))
}

func (s *SupervisorServiceSuite) TestItIsReadyAfterStartingAllServices() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var services = []Service{
		NewMockService("Mock1", 0),
		NewMockService("Mock2", 0),
		NewMockService("Mock3", 0),
	}

	ctxClosed := make(chan time.Time)
	go func() {
		<-ctx.Done()
		close(ctxClosed)
	}()

	for _, service := range services {
		mockService := service.(*MockService)
		mockService.
			On("Start", mock.Anything, mock.Anything).
			Return(nil).
			WaitUntil(ctxClosed)
	}

	supervisor := SupervisorService{
		Name:     "supervisor",
		Services: services,
	}

	ready := make(chan struct{})
	go func() {
		_ = supervisor.Start(ctx, ready)
	}()

	select {
	case <-ready:
		for _, service := range services {
			mockService := service.(*MockService)
			mockService.AssertCalled(s.T(), "Start", mock.Anything, mock.Anything)
		}
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to be ready")
	}
}

func (s *SupervisorServiceSuite) TestItStopsAllServicesWhenContextIsCanceled() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []Service{
		NewMockService("Mock1", 0),
		NewMockService("Mock2", 0),
		NewMockService("Mock3", 0),
	}

	ctxClosed := make(chan time.Time)
	go func() {
		<-ctx.Done()
		close(ctxClosed)
	}()

	for _, service := range services {
		mockService := service.(*MockService)
		mockService.
			On("Start", mock.Anything, mock.Anything).
			Return(context.Canceled).
			WaitUntil(ctxClosed)
	}
	supervisor := SupervisorService{
		Name:     "supervisor",
		Services: services,
	}

	result := make(chan error)
	ready := make(chan struct{})
	go func() {
		result <- supervisor.Start(ctx, ready)
	}()

	select {
	case <-ready:
		cancel()
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to be ready")
	}

	select {
	case err := <-result:
		s.ErrorIs(err, context.Canceled)
		for _, service := range services {
			mockService := service.(*MockService)
			mockService.AssertExpectations(s.T())
		}
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to be ready")
	}
}

func (s *SupervisorServiceSuite) TestItStopsAllServicesIfAServiceStops() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockErr := errors.New("err")
	services := []Service{
		NewMockService("Mock1", 0),
		NewMockService("Mock2", 0),
		NewMockService("Mock3", 0),
	}

	for idx, service := range services {
		mockService := service.(*MockService)
		if idx == len(services)-1 {
			mockService.
				On("Start", mock.Anything, mock.Anything).
				Return(mockErr).
				After(100 * time.Millisecond)
		} else {
			mockService.
				On("Start", mock.Anything, mock.Anything).
				Return(context.Canceled).
				After(500 * time.Millisecond)
		}
	}

	supervisor := SupervisorService{
		Name:     "supervisor",
		Services: services,
	}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- supervisor.Start(ctx, ready)
	}()

	select {
	case err := <-result:
		s.ErrorIs(err, mockErr)
		for _, service := range services {
			mockService := service.(*MockService)
			mockService.AssertExpectations(s.T())
		}
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to return")
	}
}

func (s *SupervisorServiceSuite) TestItStopsCreatingServicesIfAServiceFailsToStart() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockErr := errors.New("err")
	services := []Service{
		NewMockService("Mock1", 0),
		NewMockService("Mock2", -1),
		NewMockService("Mock3", 0),
	}

	for idx, service := range services {
		mockService := service.(*MockService)
		if idx == 1 {
			mockService.On("Start", mock.Anything, mock.Anything).Return(mockErr)
		} else {
			mockService.
				On("Start", mock.Anything, mock.Anything).
				Return(context.Canceled).
				After(300 * time.Millisecond)
		}
	}

	supervisor := SupervisorService{
		Name:     "supervisor",
		Services: services,
	}

	result := make(chan error, 1)
	ready := make(chan struct{}, 1)
	go func() {
		result <- supervisor.Start(ctx, ready)
	}()

	select {
	case err := <-result:
		s.ErrorIs(err, mockErr)
		last := services[len(services)-1].(*MockService)
		last.AssertNotCalled(s.T(), "Start", mock.Anything, mock.Anything)
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to return")
	}
}

func (s *SupervisorServiceSuite) TestItStopsCreatingServicesIfContextIsCanceled() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []Service{
		NewMockService("Mock1", 0),
		NewMockService("Mock2", time.Second),
		NewMockService("Mock3", 0),
		NewMockService("Mock4", 0),
	}

	ctxClosed := make(chan time.Time)
	go func() {
		<-ctx.Done()
		close(ctxClosed)
	}()

	for _, service := range services {
		mockService := service.(*MockService)
		mockService.
			On("Start", mock.Anything, mock.Anything).
			Return(context.Canceled).
			WaitUntil(ctxClosed)
	}
	supervisor := SupervisorService{
		Name:     "supervisor",
		Services: services,
	}

	result := make(chan error)
	ready := make(chan struct{}, 1)
	go func() {
		result <- supervisor.Start(ctx, ready)
	}()

	<-time.After(300 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		s.ErrorIs(err, context.Canceled)
		for idx, service := range services {
			mockService := service.(*MockService)
			if idx > 1 {
				mockService.AssertNotCalled(s.T(), "Start", mock.Anything, mock.Anything)
			} else {
				mockService.AssertExpectations(s.T())
			}
		}
	case <-ready:
		s.FailNow("supervisor shouldn't be ready")
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to return")
	}
}

func (s *SupervisorServiceSuite) TestItTimesOutIfServiceTakesTooLongToBeReady() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock1 := NewMockService("Mock1", 500*time.Millisecond)
	mock1.
		On("Start", mock.Anything, mock.Anything).
		Return(context.Canceled).
		After(time.Second)

	supervisor := SupervisorService{
		Name:         "supervisor",
		Services:     []Service{mock1},
		ReadyTimeout: 200 * time.Millisecond,
	}

	result := make(chan error)
	ready := make(chan struct{}, 1)
	go func() {
		result <- supervisor.Start(ctx, ready)
	}()

	select {
	case err := <-result:
		s.ErrorIs(err, ServiceTimeoutError)
		mock1.AssertCalled(s.T(), "Start", mock.Anything, mock.Anything)
	case <-ready:
		s.FailNow("supervisor shouldn't be ready")
	case <-time.After(DefaultServiceTimeout):
		s.FailNow("timed out waiting for supervisor to return")
	}
}

func (s *SupervisorServiceSuite) TestItTimesOutIfServicesTakeTooLongToStop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock1 := NewMockService("Mock1", 0)
	mock1.
		On("Start", mock.Anything, mock.Anything).
		Return(context.Canceled).
		After(time.Second)

	timeout := 500 * time.Millisecond
	supervisor := SupervisorService{
		Name:        "supervisor",
		Services:    []Service{mock1},
		StopTimeout: timeout,
	}

	result := make(chan error)
	ready := make(chan struct{}, 1)
	go func() {
		result <- supervisor.Start(ctx, ready)
	}()

	<-ready
	cancel()

	err := <-result
	s.ErrorIs(err, SupervisorTimeoutError)
}

type MockService struct {
	mock.Mock
	Name string
	// The time to wait before notifying it is ready. Provide a negative value
	// to prevent such notification from being sent
	ReadyDelay time.Duration
}

func (m *MockService) Start(ctx context.Context, ready chan<- struct{}) error {
	if m.ReadyDelay >= 0 {
		go func() {
			<-time.After(m.ReadyDelay)
			ready <- struct{}{}
		}()
	}

	returnArgs := m.Called(ctx, ready)
	return returnArgs.Error(0)
}

func (m *MockService) String() string {
	return m.Name
}

func NewMockService(name string, readyDelay time.Duration) *MockService {
	return &MockService{
		Name:       name,
		ReadyDelay: readyDelay,
	}
}
