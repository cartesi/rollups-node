// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/spf13/cobra"
)

// SingleServiceOptions supplies the configuration and constructor for a standalone service.
type SingleServiceOptions struct {
	Command            *cobra.Command
	Name               string
	LogLevel           slog.Level
	LogColor           bool
	MaxStartupTime     time.Duration
	DatabaseConnection config.SafeURL
	TelemetryAddress   string
	Create             func(context.Context, *slog.Logger, repository.Repository) (service.SupervisedService, error)
}

// RunSingleService owns the repository and supervisor lifecycle. Resources are
// cleaned up before an error is logged and returned to Cobra.
func RunSingleService(opts SingleServiceOptions) (runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), opts.MaxStartupTime)
	defer cancel()
	logger := service.NewLogger(opts.Name, opts.LogLevel, opts.LogColor)
	defer func() { LogErr(logger, runErr) }()
	opts.Command.SilenceUsage = true

	repo, err := factory.NewRepositoryFromConnectionString(ctx, opts.DatabaseConnection.Raw())
	if err != nil {
		return err
	}
	defer repo.Close()

	sup, err := service.NewSupervisor(ctx, &service.SupervisorConfigs{
		BaseConfigs:          service.BaseConfigs{Name: opts.Name, Logger: logger},
		EnableSignalHandling: true,
		TelemetryAddress:     opts.TelemetryAddress,
		Factories: []service.FactoryFunction{
			func(ctx context.Context, sup service.Supervisor) (service.SupervisedService, error) {
				return opts.Create(ctx, sup.Logger(), repo)
			},
		},
	})
	if err != nil {
		return err
	}
	return sup.Serve()
}
