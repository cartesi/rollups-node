// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
)

var (
	logLevel           string
	logColor           bool
	databaseConnection string
	pollInterval       string
	maxStartupTime     string
	telemetryAddress   string
	cfg                *config.ValidatorConfig
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceValidator,
	Short:   "Runs cartesi-rollups-" + config.ServiceValidator,
	Long:    "Runs cartesi-rollups-" + config.ServiceValidator + " in standalone mode",
	RunE:    run,
	Version: version.BuildVersion,
}

func init() {
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.VALIDATOR_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"Tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cli.AddFlagStrVar(flags, &pollInterval, "poll-interval", config.VALIDATOR_POLLING_INTERVAL,
		"Poll interval")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadValidatorConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) (runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	name := config.ServiceValidator
	logger := service.NewLogger(name, cfg.LogLevel, cfg.LogColor)
	// Return errors to Cobra only after all resource cleanup has completed.
	defer func() { cli.LogErr(logger, runErr) }()
	cmd.SilenceUsage = true

	repo, err := factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	if err != nil {
		return err
	}
	defer repo.Close()

	supCfg := &service.SupervisorConfigs{
		BaseConfigs:          service.BaseConfigs{Name: name, Logger: logger},
		EnableSignalHandling: true,
		TelemetryAddress:     cfg.ValidatorTelemetryAddress,
		Factories: []service.FactoryFunction{
			func(ctx context.Context, sup service.Supervisor) (service.SupervisedService, error) {
				return validator.Create(ctx, &validator.CreateInfo{
					Config:     *cfg,
					Logger:     sup.Logger(),
					Repository: repo,
				})
			},
		},
	}
	sup, err := service.NewSupervisor(ctx, supCfg)
	if err != nil {
		return err
	}
	return sup.Serve()
}
