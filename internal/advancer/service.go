// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package advancer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/machine"
	"github.com/cartesi/rollups-node/pkg/service"
)

// Service is the main advancer service that processes inputs through Cartesi machines
type Service struct {
	service.TickServiceTemplate
	supervisor     *service.Supervisor
	inputBatchSize uint64
	snapshotsDir   string
	repository     AdvancerRepository
	machineManager manager.MachineProvider

	// cleanedUp ensures HTTP server shutdown and machine manager close run
	// exactly once, even when Stop() is called multiple times (by the child's
	// Serve() loop and by the parent orchestrator).
	cleanedUp atomic.Bool
}

// CreateInfo contains the configuration for creating an advancer service
type CreateInfo struct {
	Config     config.AdvancerConfig
	Logger     *slog.Logger
	Repository repository.Repository
	Machines   manager.MachineProvider
	Supervisor *service.Supervisor
}

// Create initializes a new advancer service
func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}
	// This is a process-wide compatibility invariant, not an application
	// failure. Refuse advancer startup instead of retrying every application on
	// every tick while reporting a misleading healthy service.
	if err = machine.ValidateEmulatorComputationHashLimits(); err != nil {
		return nil, fmt.Errorf("invalid machine execution constants: %w", err)
	}

	if c.Supervisor == nil {
		return nil, fmt.Errorf("supervisor on advancer service Create is nil")
	}

	s := &Service{
		supervisor:   c.Supervisor,
		snapshotsDir: c.Config.SnapshotsDir,
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on advancer service Create is nil")
	}

	s.machineManager = c.Machines
	if s.machineManager == nil {
		return nil, fmt.Errorf("machine manager on advancer service Create is nil")
	}

	s.inputBatchSize = c.Config.AdvancerInputBatchSize
	if s.inputBatchSize == 0 {
		return nil, fmt.Errorf("advancer input batch size must be greater than 0")
	}

	tickCfg := &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     config.ServiceAdvancer,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		PollInterval: c.Config.AdvancerPollingInterval,
	}
	err = service.InitTickServiceTemplate(&s.TickServiceTemplate, tickCfg, s)
	if err != nil {
		return nil, err
	}

	s.Logger.Info("Created", "config", c.Config)

	return s, nil
}

// Service interface implementation
func (s *Service) Ready() bool {
	// This is a local fail-closed signal while application-failure durability
	// is unresolved. It cannot atomically revoke work already selected by a
	// separate process before that durable status becomes visible.
	return s.machineManager != nil && !s.machineManager.HasPendingApplicationFailures()
}

func (s *Service) Tick(ctx context.Context) (bool, error) {
	// Signal reschedule whenever work was done, even if some apps errored.
	// Failed apps are marked Failed and removed by the machine manager,
	// so they won't cause amplified retries on the next tick.
	// Without this, one failing app delays all healthy apps by a full poll interval.
	hadWork, err := s.Step(ctx)

	if err == nil {
		return hadWork, nil
	}
	// During shutdown, the machine manager is closed and GetMachine() may
	// return ErrNoApp. Suppress this to avoid spurious ERR log entries.
	if errors.Is(err, ErrNoApp) && ctx.Err() != nil {
		s.Logger.Warn("Tick interrupted by shutdown", "error", err)
		return hadWork, nil
	}
	// Canceled is graceful per the project convention: code paths that
	// wrap cancellation (e.g. handleSnapshot → createSnapshot →
	// "failed to update input snapshot URI: %w") would otherwise surface
	// at ERR via the framework's Tick wrapper. DeadlineExceeded remains a
	// real failure and is propagated.
	if errors.Is(err, context.Canceled) {
		s.Logger.Debug("Tick cancelled (shutdown)", "error", err)
		return hadWork, nil
	}
	return hadWork, err
}
