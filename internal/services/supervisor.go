// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package services

import (
	"context"
	"errors"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"golang.org/x/sync/errgroup"
)

const DefaultServiceTimeout = 5 * time.Second

var (
	ServiceTimeoutError    = errors.New("timed out waiting for service to be ready")
	SupervisorTimeoutError = errors.New("timed out waiting for services to stop")
)

// SupervisorService is a simple implementation of a supervisor.
// It runs its services until the first returns a non-nil error.
type SupervisorService struct {
	// Name of the service
	Name string

	// Services to be managed
	Services []Service

	// The amount of time to wait for a service to be ready.
	// Default is 5 seconds
	ReadyTimeout time.Duration

	// The amount of time to wait for a service to exit after
	// its context is canceled. Default is 5 seconds
	StopTimeout time.Duration
}

func (s SupervisorService) String() string {
	return s.Name
}

func (s SupervisorService) Start(ctx context.Context, ready chan<- struct{}) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// flag indicating if a service timed out during start
	var serviceTimedOut bool
	readyTimeout := s.ReadyTimeout
	if readyTimeout <= 0 {
		readyTimeout = DefaultServiceTimeout
	}
	stopTimeout := s.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = DefaultServiceTimeout
	}

Loop:
	// start services one by one
	for _, service := range s.Services {
		service := service
		serviceReady := make(chan struct{}, 1)

		group.Go(func() error {
			err := service.Start(ctx, serviceReady)
			if err != nil && !errors.Is(err, context.Canceled) {
				config.ErrorLogger.Printf("%v: %v exited with error. %v", s, service, err)
			} else {
				config.InfoLogger.Printf("%v: %v exited successfully\n", s, service)
			}
			return err
		})

		select {
		// service is ready, move along
		case <-serviceReady:
			config.InfoLogger.Printf("%v: %v is ready\n", s, service)
		// a service exited with error
		case <-ctx.Done():
			break Loop
		// service took too long to become ready
		case <-time.After(readyTimeout):
			config.ErrorLogger.Printf("%v: %v timed out\n", s, service)
			cancel()
			serviceTimedOut = true
			break Loop
		}
	}

	// if nothing went wrong while starting services, SupervisorService is ready
	if ctx.Err() == nil {
		ready <- struct{}{}
		config.InfoLogger.Printf("%v: all services are ready\n", s)
	}

	// wait until a service exits with error or the external context is canceled
	<-ctx.Done()

	// wait for all services to stop
	wait := make(chan error)
	go func() {
		wait <- group.Wait()
	}()

	select {
	case err := <-wait:
		config.InfoLogger.Printf("%v: all services exited", s)
		if serviceTimedOut {
			return ServiceTimeoutError
		}
		return err
	case <-time.After(stopTimeout):
		config.ErrorLogger.Printf("%v: %v", s, SupervisorTimeoutError)
		return SupervisorTimeoutError
	}
}
