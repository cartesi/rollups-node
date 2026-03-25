// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/inspect"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
)

// httpShutdownTimeout is how long to wait for in-flight inspect HTTP requests
// to drain before forcibly closing the server during shutdown.
const httpShutdownTimeout = 10 * time.Second //nolint: mnd

// Service is the main advancer service that processes inputs through Cartesi machines
type Service struct {
	service.Service
	inputBatchSize uint64
	snapshotsDir   string
	repository     AdvancerRepository
	machineManager manager.MachineProvider
	inspector      *inspect.Inspector
	HTTPServer     *http.Server
	HTTPServerFunc func() error
	stopping       atomic.Bool
	stopOnce       sync.Once
}

// CreateInfo contains the configuration for creating an advancer service
type CreateInfo struct {
	service.CreateInfo
	Config     config.AdvancerConfig
	Repository repository.Repository
}

// Create initializes a new advancer service
func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on advancer service Create is nil")
	}

	s.inputBatchSize = c.Config.AdvancerInputBatchSize
	if s.inputBatchSize == 0 {
		return nil, fmt.Errorf("advancer input batch size must be greater than 0")
	}

	// Create the machine manager
	manager := manager.NewMachineManager(
		c.Repository,
		s.Logger,
		c.Config.FeatureMachineHashCheckEnabled,
		s.inputBatchSize,
	)
	s.machineManager = manager

	// Initialize the inspect service if enabled
	if c.Config.FeatureInspectEnabled {
		s.inspector, s.HTTPServer, s.HTTPServerFunc = inspect.NewInspector(
			c.Repository,
			manager,
			c.Config.InspectAddress,
			c.LogLevel,
			c.LogColor,
		)
	}

	s.snapshotsDir = c.Config.SnapshotsDir

	return s, nil
}

// Service interface implementation
func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }
func (s *Service) Tick() []error {
	err := s.Step(s.Context)
	if err == nil {
		return nil
	}
	// During shutdown, Stop() sets s.stopping before closing the machine
	// manager. An in-flight Tick may see ErrNoApp from GetMachine() when
	// the manager is closed mid-operation. We suppress this to avoid
	// spurious ERR log entries during graceful shutdown.
	if errors.Is(err, ErrNoApp) && s.stopping.Load() {
		s.Logger.Warn("Tick interrupted by shutdown", "error", err)
		return nil
	}
	return []error{err}
}
func (s *Service) Stop(b bool) []error {
	var errs []error
	s.stopping.Store(true)
	s.stopOnce.Do(func() {
		if s.HTTPServer != nil {
			s.Logger.Info("Shutting down inspect HTTP server")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
			defer cancel()
			if err := s.HTTPServer.Shutdown(shutdownCtx); err != nil {
				errs = append(errs, fmt.Errorf("failed to shutdown inspect HTTP server: %w", err))
			}
		}
		if s.machineManager != nil {
			s.Logger.Info("Closing machine manager")
			if err := s.machineManager.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close machine manager: %w", err))
			}
		}
	})
	return errs
}
func (s *Service) Serve() error {
	if s.inspector != nil && s.HTTPServerFunc != nil {
		go func() {
			if err := s.HTTPServerFunc(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.Logger.Error("Inspect HTTP server failed — shutting down", "error", err)
				s.Cancel()
			}
		}()
	}
	return s.Service.Serve()
}
func (s *Service) String() string {
	return s.Name
}
