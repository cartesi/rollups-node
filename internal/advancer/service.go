// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
const httpShutdownTimeout = 10 * time.Second

// Service is the main advancer service that processes inputs through Cartesi machines
type Service struct {
	service.TickService
	inputBatchSize uint64
	snapshotsDir   string
	repository     AdvancerRepository
	machineManager manager.MachineProvider
	inspector      *inspect.Inspector

	// cleanedUp ensures HTTP server shutdown and machine manager close run
	// exactly once, even when Stop() is called multiple times (by the child's
	// Serve() loop and by the parent orchestrator).
	cleanedUp atomic.Bool
}

// CreateInfo contains the configuration for creating an advancer service
type CreateInfo struct {
	service.CreateInfo
	Config     config.AdvancerConfig
	Repository repository.Repository
}

// Create initializes a new advancer service
func Create(ctx context.Context, c *CreateInfo) (service.IService, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	s.TickImpl = s
	c.Impl = s
	c.EnableReschedule = true

	err = service.NewTickService(&c.CreateInfo, &s.TickService)
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
		var admission *service.SemaphoreAdmission
		if c.Config.InspectMaxInflight > 0 {
			admission = service.NewSemaphoreAdmission(c.Config.InspectMaxInflight)
		}
		inspector, err := inspect.NewInspector(inspect.CreateInfo{
			Repository:         c.Repository,
			Machines:           manager,
			Address:            c.Config.InspectAddress,
			LogLevel:           c.LogLevel,
			LogPretty:          c.LogColor,
			Admission:          admission,
			CORSAllowedOrigins: c.Config.InspectCorsAllowedOrigins,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create inspect service: %w", err)
		}
		s.inspector = inspector
	}

	s.snapshotsDir = c.Config.SnapshotsDir

	s.LogConfig(c.Config)

	return s, nil
}

// Service interface implementation
func (s *Service) Tick(ctx context.Context) []error {
	hadWork, err := s.Step(ctx)

	// Signal reschedule whenever work was done, even if some apps errored.
	// Failed apps are marked Failed and removed by the machine manager,
	// so they won't cause amplified retries on the next tick.
	// Without this, one failing app delays all healthy apps by a full poll interval.
	if hadWork {
		s.SignalReschedule()
	}

	if err == nil {
		return nil
	}
	// During shutdown, the machine manager is closed and GetMachine() may
	// return ErrNoApp. Suppress this to avoid spurious ERR log entries.
	if errors.Is(err, ErrNoApp) && s.IsStopping() {
		s.Logger.Warn("Tick interrupted by shutdown", "error", err)
		return nil
	}
	return []error{err}
}

func (s *Service) OnStop(b bool) []error {
	var errs []error
	if s.inspector != nil {
		s.Logger.Info("Shutting down inspect HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()
		if err := s.inspector.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown inspect HTTP server: %w", err))
		}
	}
	if s.machineManager != nil {
		s.Logger.Info("Closing machine manager")
		if err := s.machineManager.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close machine manager: %w", err))
		}
	}
	return append(errs, s.TickService.OnStop(b)...)
}
func (s *Service) OnServe(ctx context.Context) error {
	if s.inspector != nil {
		go func() {
			if err := s.inspector.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.Logger.Error("Inspect HTTP server failed — shutting down", "error", err)
				s.Cancel()
			}
		}()
	}
	return s.TickService.OnServe(ctx)
}
