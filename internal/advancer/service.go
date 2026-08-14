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
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/cartesi/rollups-node/pkg/service"
)

// httpShutdownTimeout is how long to wait for in-flight inspect HTTP requests
// to drain before forcibly closing the server during shutdown.
const httpShutdownTimeout = 10 * time.Second

// Service is the main advancer service that processes inputs through Cartesi machines
type Service struct {
	service.Service
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
func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}
	// This is a process-wide compatibility invariant, not an application
	// failure. Refuse advancer startup instead of retrying every application on
	// every tick while reporting a misleading healthy service.
	if err = machine.ValidateEmulatorComputationHashLimits(); err != nil {
		return nil, fmt.Errorf("invalid machine execution constants: %w", err)
	}

	s := &Service{}
	c.Impl = s
	c.EnableReschedule = true

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

	return s, nil
}

// Service interface implementation
func (s *Service) Alive() bool { return true }
func (s *Service) Ready() bool {
	// This is a local fail-closed signal while application-failure durability
	// is unresolved. It cannot atomically revoke work already selected by a
	// separate process before that durable status becomes visible.
	return s.machineManager != nil && !s.machineManager.HasPendingApplicationFailures()
}
func (s *Service) Reload() []error { return nil }
func (s *Service) Tick() []error {
	hadWork, err := s.Step(s.Context)

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
	// Canceled is graceful per the project convention: code paths that
	// wrap cancellation (e.g. handleSnapshot → createSnapshot →
	// "failed to update input snapshot URI: %w") would otherwise surface
	// at ERR via the framework's Tick wrapper. DeadlineExceeded remains a
	// real failure and is propagated.
	if errors.Is(err, context.Canceled) {
		s.Logger.Debug("Tick cancelled (shutdown)", "error", err)
		return nil
	}
	return []error{err}
}

func (s *Service) Stop(b bool) []error {
	// CAS achieves once-semantics: the second caller returns immediately
	// (fire-and-forget) rather than blocking like sync.Once. This is safe
	// because the orchestrator calls Cancel() after Stop() and waits for
	// the Serve goroutine to exit.
	if !s.cleanedUp.CompareAndSwap(false, true) {
		return nil // already stopped
	}
	// This method shadows service.Service.Stop(), so set the stopping flag
	// explicitly. Without this, a concurrent Tick that observes closed
	// resources would not see IsStopping() == true.
	s.SetStopping()
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
	return errs
}
func (s *Service) Serve() error {
	if s.inspector != nil {
		go func() {
			if err := s.inspector.Serve(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
