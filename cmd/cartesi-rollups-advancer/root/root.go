// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/inspect"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
)

var (
	logLevel               string
	logColor               bool
	databaseConnection     string
	pollInterval           string
	maxStartupTime         string
	telemetryAddress       string
	inspectAddress         string
	machinelogLevel        string
	enableMachineHashCheck bool
	enableInspect          bool
	cfg                    *config.AdvancerConfig
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceAdvancer,
	Short:   "Runs cartesi-rollups-" + config.ServiceAdvancer,
	Long:    "Runs cartesi-rollups-" + config.ServiceAdvancer + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVar(flags, &inspectAddress, "inspect-address", config.INSPECT_ADDRESS,
		"Inspect service address and port")
	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.ADVANCER_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"Tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format")
	cli.AddFlagStrVar(flags, &pollInterval, "poll-interval", config.ADVANCER_POLLING_INTERVAL,
		"Poll interval")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")
	cli.AddFlagBoolVar(flags, &enableInspect, "inspect-enabled", config.FEATURE_INSPECT_ENABLED,
		"Enable or disable the inspect service")
	cli.AddFlagBoolVar(flags, &enableMachineHashCheck, "machine-hash-check", config.FEATURE_MACHINE_HASH_CHECK_ENABLED,
		"Enable or disable machine hash check (DO NOT USE IN PRODUCTION)")
	cli.AddFlagStrVar(flags, &machinelogLevel, "machine-log-level", config.JSONRPC_MACHINE_LOG_LEVEL,
		"Remote Machine log level: trace, debug, info, warning, error, fatal")

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadAdvancerConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	// Create shared components

	name := config.ServiceAdvancer
	logger := service.NewLogger(name, cfg.LogLevel, cfg.LogColor)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer repo.Close()

	machineManager := manager.NewMachineManager(
		repo,
		logger,
		cfg.FeatureMachineHashCheckEnabled,
		cfg.AdvancerInputBatchSize,
	)
	defer machineManager.Close()

	// Create factories of services

	factories := []service.FactoryFunction{
		func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
			return advancer.Create(ctx, &advancer.CreateInfo{
				Config:     *cfg,
				Repository: repo,
				Machines:   machineManager,
				Supervisor: sup,
				Logger:     sup.Logger,
			})
		},
	}

	if cfg.FeatureInspectEnabled {
		factories = append(factories,
			func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
				return inspect.Create(ctx, &inspect.CreateInfo{
					Config:     *cfg,
					Repository: repo,
					Machines:   machineManager,
				})
			},
		)
	}

	supCfg := &service.SupervisorConfigs{
		BaseConfigs:          service.BaseConfigs{Name: name, Logger: logger},
		EnableSignalHandling: true,
		TelemetryCreate:      true,
		TelemetryAddress:     cfg.AdvancerTelemetryAddress,
		Factories:            factories,
	}
	sup, err := service.NewSupervisor(ctx, supCfg)
	cli.CheckErr(logger, err)
	defer sup.Close()
	cli.CheckErr(logger, sup.Serve())
}
