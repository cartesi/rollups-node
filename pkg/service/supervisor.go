// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/cartesi/rollups-node/internal/version"
)

func inParallel[T any](f func(T), list []T) {
	count := len(list)
	done := make(chan struct{}, count)
	for _, v := range list {
		go func(v T) {
			defer func() { done <- struct{}{} }()
			f(v)
		}(v)
	}
	for range count {
		<-done
	}
}

type FactoryFunction func(context.Context, *Supervisor) (SupervisedService, error)

type SupervisorConfigs struct {
	BaseConfigs
	Factories            []FactoryFunction
	EnableSignalHandling bool
	TelemetryCreate      bool
	TelemetryAddress     string
}

// Supervisor of multiple services under a single management.
type Supervisor struct {
	Name        string
	Logger      *slog.Logger
	services    []SupervisedService
	context     context.Context
	cancel      context.CancelFunc
	sigShutdown chan os.Signal // SIGINT/SIGTERM to exit gracefully

	serving   atomic.Bool
	stopping  atomic.Bool
	stoppedCh chan struct{}
}

func NewSupervisor(ctx context.Context, c *SupervisorConfigs) (*Supervisor, error) {
	s := &Supervisor{}

	s.context, s.cancel = context.WithCancel(context.Background())
	s.stoppedCh = make(chan struct{})

	// log
	s.Name = c.Name
	s.Logger = c.Logger
	if s.Logger == nil {
		s.Logger = NewLogger(s.Name, c.LogLevel, c.LogColor)
	}

	// signal handling
	if c.EnableSignalHandling {
		s.sigShutdown = make(chan os.Signal, 1)
		signal.Notify(s.sigShutdown, syscall.SIGINT, syscall.SIGTERM)
		go func(ch <-chan os.Signal) {
			for range ch {
				s.Stop(false)
			}
		}(s.sigShutdown)
	}

	factories := make([]FactoryFunction, len(c.Factories))
	copy(factories, c.Factories)

	// telemetry
	if c.TelemetryCreate {
		if c.TelemetryAddress == "" {
			c.TelemetryAddress = ":8080"
		}
		factories = append(factories,
			func(context.Context, *Supervisor) (SupervisedService, error) {
				return CreateDefaultTelemetry(s, c.TelemetryAddress), nil
			},
		)
	}

	s.Logger.Info("Create", "version", version.BuildVersion, "log_level", c.LogLevel, "pid", os.Getpid())

	// initialize all services
	ctxInit, cancelInit := context.WithCancel(ctx)
	defer cancelInit()
	var initFail atomic.Bool
	initSvcCh := make(chan SupervisedService, len(factories))
	for _, factory := range factories {
		go func(factory FactoryFunction) {
			svc, initErr := factory(ctxInit, s)
			switch {
			case initErr != nil:
				switch {
				case initFail.CompareAndSwap(false, true):
					cancelInit()
					s.Logger.Error("Subservice initialization failure, shutting down",
						"subservice", svc,
						"error", initErr,
					)
				case errors.Is(initErr, context.Canceled):
					s.Logger.Info("Subservice initialization canceled",
						"subservice", svc,
					)
				default:
					s.Logger.Warn("Subservice initialization failure",
						"subservice", svc,
						"error", initErr,
					)
				}
				svc = nil // service is not initialized due to a initialization error
			case initFail.Load():
				svc.Teardown()
				svc = nil // service is not initialized because it was already finalized
			}
			initSvcCh <- svc
		}(factory)
	}

	// wait for the conclusion of the initialization of all services
	for range len(factories) {
		svc := <-initSvcCh
		if svc != nil {
			s.Logger.Info("Subservice initialized", "subservice", svc)
			s.services = append(s.services, svc)
		} else {
			inParallel(func(svc SupervisedService) { svc.Teardown() }, s.services)
			for range len(factories) - len(s.services) - 1 {
				<-initSvcCh
			}
			return nil, ErrServiceBadInit
		}
	}

	return s, nil
}

func (s *Supervisor) String() string {
	return s.Name
}

func (s *Supervisor) Alive() bool {
	return s.serving.Load() && !s.stopping.Load()
}

func (s *Supervisor) Ready() bool {
	if !s.Alive() {
		return false
	}
	for _, svc := range s.services {
		if !svc.Ready() {
			s.Logger.Info("Service still not ready", "service", svc.String())
			return false
		}
	}
	return true
}

func (s *Supervisor) Serve() error {
	// CAS achieves once-semantics: the second caller returns immediately
	// (fire-and-forget) rather than blocking like sync.Once. This is safe
	// because the orchestrator calls Cancel() after Stop() and waits for
	// the Serve goroutine to exit.
	if !s.serving.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	defer func() {
		s.Stop(false) // make sure context is canceled
		close(s.stoppedCh)
	}()

	// check if we were stopped already.
	if s.stopping.Load() {
		inParallel(func(svc SupervisedService) { svc.Teardown() }, s.services)
		s.services = []SupervisedService{}
		return nil
	}

	s.Logger.Info("Supervised services started")

	var err error

	stopSvcCh := make(chan struct{}, len(s.services))
	for _, svc := range s.services {
		go func(svc SupervisedService) {
			s.Logger.Info("Starting subservice", "subservice", svc)
			svcErr := svc.Serve(s.context)
			switch {
			case !s.stopping.Load() && s.Stop(false):
				s.Logger.Error("Subservice stopped unexpectedly, shutting down",
					"service", svc,
					"err", svcErr,
				)
				err = ErrServiceStopped
			case svcErr == nil || errors.Is(svcErr, context.Canceled):
				s.Logger.Info("Subservice stopped",
					"subservice", svc,
				)
			default:
				s.Logger.Warn("Subservice failed during shutting down",
					"subservice", svc,
					"err", svcErr,
				)
			}
			svc.Teardown()
			stopSvcCh <- struct{}{}
		}(svc)
	}

	// wait for all services to terminate
	for range s.services {
		<-stopSvcCh
	}
	s.services = nil

	s.Logger.Info("Supervisor terminated")

	return err
}

func (s *Supervisor) Stop(wait bool) bool {
	stopped := s.stopping.CompareAndSwap(false, true)
	if stopped {
		s.Logger.Info("Stopping supervisor")
		s.cancel()
	}

	if wait {
		<-s.stoppedCh
	}

	return stopped
}

func (s *Supervisor) Close() {
	s.Stop(false)
	if s.serving.CompareAndSwap(false, true) {
		inParallel(func(svc SupervisedService) { svc.Teardown() }, s.services)
		s.services = []SupervisedService{}
	}
	if s.sigShutdown != nil {
		ch := s.sigShutdown
		signal.Stop(ch)
		s.sigShutdown = nil
		close(ch)
	}
}
