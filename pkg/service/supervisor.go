// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/internal/version"
	"golang.org/x/sync/errgroup"
)

type FactoryFunction func(context.Context, Supervisor) (SupervisedService, error)

type SupervisorConfigs struct {
	BaseConfigs
	Factories            []FactoryFunction
	EnableSignalHandling bool
	TelemetryAddress     string
}

// Supervisor manages multiple services under a single lifecycle.
type Supervisor interface {
	String() string
	Logger() *slog.Logger
	Alive() bool
	// NotReady returns failing service names, or the supervisor name when not alive.
	// An empty result means all services are ready.
	NotReady() []string
	Serve() error
	Stop() bool
	// Fatal joins all fatal causes and initiates shutdown.
	Fatal(error)
}

// supervisorImpl is the default Supervisor implementation.
type supervisorImpl struct {
	Name        string
	logger      *slog.Logger
	services    []SupervisedService
	context     context.Context
	cancel      context.CancelFunc
	sigShutdown chan os.Signal // SIGINT/SIGTERM to exit gracefully
	fatalErr    error
	fatalMux    sync.RWMutex

	serving  atomic.Bool
	stopping atomic.Bool
}

func NewSupervisor(ctx context.Context, c *SupervisorConfigs) (Supervisor, error) {
	if len(c.Factories) == 0 {
		return nil, fmt.Errorf("%w: at least one service factory is required", ErrServiceBadInit)
	}
	s := &supervisorImpl{}

	s.context, s.cancel = context.WithCancel(context.Background())

	// log
	s.Name = c.Name
	s.logger = c.Logger
	if s.logger == nil {
		s.logger = NewLogger(s.Name, c.LogLevel, c.LogColor)
	}

	// signal handling
	if c.EnableSignalHandling {
		s.sigShutdown = make(chan os.Signal, 1)
		signal.Ignore(syscall.SIGHUP)
		signal.Notify(s.sigShutdown, syscall.SIGINT, syscall.SIGTERM)
	}

	factories := make([]FactoryFunction, len(c.Factories))
	copy(factories, c.Factories)

	// telemetry
	if c.TelemetryAddress != "" {
		factories = append(factories,
			func(context.Context, Supervisor) (SupervisedService, error) {
				return createDefaultTelemetry(s, c.TelemetryAddress), nil
			},
		)
	}

	s.logger.Info("Create", "version", version.BuildVersion, "log_level", c.LogLevel, "pid", os.Getpid())

	// Each factory owns one slot; Wait joins all writes before collection or cleanup.
	g, ctxInit := errgroup.WithContext(ctx)
	svcs := make([]SupervisedService, len(factories))
	for i, factory := range factories {
		g.Go(func() error {
			svc, err := factory(ctxInit, s)
			if err != nil {
				return fmt.Errorf("factory %d: %w", i, err)
			}
			svcs[i] = svc
			return nil
		})
	}
	initErr := g.Wait()
	for _, svc := range svcs {
		if svc != nil {
			s.logger.Info("Subservice initialized", "subservice", svc)
			s.services = append(s.services, svc)
		}
	}
	if initErr != nil {
		s.logger.Error("Subservice initialization failure, shutting down", "error", initErr)
		s.Stop()
		return nil, errors.Join(ErrServiceBadInit, initErr)
	}

	return s, nil
}

func (s *supervisorImpl) String() string {
	return s.Name
}

func (s *supervisorImpl) Logger() *slog.Logger { return s.logger }

func (s *supervisorImpl) Alive() bool {
	return s.serving.Load() && !s.stopping.Load()
}

func (s *supervisorImpl) NotReady() []string {
	if !s.Alive() {
		name := s.Name
		if name == "" {
			name = "supervisor"
		}
		return []string{name}
	}
	var names []string
	for _, svc := range s.services {
		if !svc.Ready() {
			names = append(names, svc.String())
		}
	}
	// Initialization completes concurrently; keep probe responses deterministic.
	slices.Sort(names)
	return names
}

func (s *supervisorImpl) Serve() (err error) {
	// CAS achieves once-semantics: the second caller returns immediately
	// (fire-and-forget) rather than blocking like sync.Once. This is safe
	// because the orchestrator calls Cancel() after Stop() and waits for
	// the Serve goroutine to exit.
	if !s.serving.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	if s.sigShutdown != nil {
		go func(ch <-chan os.Signal) {
			<-ch
			s.Stop()
		}(s.sigShutdown)
	}

	defer func() {
		s.Stop() // make sure context is canceled

		s.fatalMux.RLock()
		defer s.fatalMux.RUnlock()
		if s.fatalErr != nil {
			err = errors.Join(err, s.fatalErr)
		}
	}()

	// check if we were stopped already.
	if s.stopping.Load() {
		return nil
	}

	s.logger.Info("Supervised services started")

	svcErrCh := make(chan error, len(s.services))
	for _, svc := range s.services {
		go func() {
			s.logger.Info("Starting subservice", "subservice", svc)
			svcErr := svc.Serve(s.context)
			unexpected := s.Stop()
			switch {
			case unexpected:
				s.logger.Error("Subservice stopped unexpectedly, shutting down",
					"subservice", svc,
					"err", svcErr,
				)
				if svcErr == nil {
					svcErr = ErrServiceStopped
				}
			case svcErr == nil || errutil.IsOnlyCancellation(svcErr):
				s.logger.Info("Subservice stopped",
					"subservice", svc,
				)
				svcErr = nil
			default:
				// Non-cancellation drain failures, including deadlines, fail shutdown.
				s.logger.Warn("Subservice failed during shutting down",
					"subservice", svc,
					"err", svcErr,
				)
			}
			svcErrCh <- svcErr
		}()
	}

	// Join every service and aggregate errors only in the supervisor goroutine.
	errs := make([]error, 0, len(s.services))
	for range s.services {
		if err := <-svcErrCh; err != nil {
			errs = append(errs, err)
		}
	}

	s.logger.Info("Supervisor terminated")

	return errors.Join(errs...)
}

// Fatal publishes the failure before cancellation so Serve observes it even if
// all services return context.Canceled as a result of shutdown.
func (s *supervisorImpl) Fatal(err error) {
	if err == nil {
		err = ErrServiceStopped
	}
	s.fatalMux.Lock()
	defer s.fatalMux.Unlock()

	s.fatalErr = errors.Join(s.fatalErr, err)

	s.Stop()
}

func (s *supervisorImpl) Stop() bool {
	stopped := s.stopping.CompareAndSwap(false, true)
	if stopped {
		s.logger.Info("Stopping supervisor")
		s.cancel()
		if s.sigShutdown != nil {
			ch := s.sigShutdown
			signal.Stop(ch)
			close(ch)
		}
	}
	return stopped
}
