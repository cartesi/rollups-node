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
	"sync/atomic"
	"syscall"

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
	Ready() bool
	Serve() error
	Stop() bool
}

// supervisorImpl is the default Supervisor implementation.
type supervisorImpl struct {
	Name        string
	logger      *slog.Logger
	services    []SupervisedService
	context     context.Context
	cancel      context.CancelFunc
	sigShutdown chan os.Signal // SIGINT/SIGTERM to exit gracefully

	serving  atomic.Bool
	stopping atomic.Bool
}

func NewSupervisor(ctx context.Context, c *SupervisorConfigs) (Supervisor, error) {
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

func (s *supervisorImpl) Ready() bool {
	if !s.Alive() {
		return false
	}
	for _, svc := range s.services {
		if !svc.Ready() {
			s.logger.Info("Service still not ready", "service", svc.String())
			return false
		}
	}
	return true
}

func (s *supervisorImpl) Serve() error {
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
	}()

	// check if we were stopped already.
	if s.stopping.Load() {
		return nil
	}

	s.logger.Info("Supervised services started")

	var err error

	stopSvcCh := make(chan struct{}, len(s.services))
	for _, svc := range s.services {
		go func() {
			s.logger.Info("Starting subservice", "subservice", svc)
			svcErr := svc.Serve(s.context)
			unexpected := s.Stop()
			switch {
			case unexpected:
				s.logger.Error("Subservice stopped unexpectedly, shutting down",
					"service", svc,
					"err", svcErr,
				)
				// Only the Stop winner writes err; stopSvcCh joins that write.
				err = ErrServiceStopped
			case svcErr == nil || errors.Is(svcErr, context.Canceled):
				s.logger.Info("Subservice stopped",
					"subservice", svc,
				)
			default:
				s.logger.Warn("Subservice failed during shutting down",
					"subservice", svc,
					"err", svcErr,
				)
			}
			stopSvcCh <- struct{}{}
		}()
	}

	// wait for all services to terminate
	for range s.services {
		<-stopSvcCh
	}

	s.logger.Info("Supervisor terminated")

	return err
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
