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
	Run:     run,
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

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	createInfo := validator.CreateInfo{
		TickServiceConfigs: service.TickServiceConfigs{
			PollInterval:         cfg.ValidatorPollingInterval,
			ServiceConfigs: service.ServiceConfigs{
				Name:                 config.ServiceValidator,
				LogLevel:             config.ResolveServiceLogLevel(config.ServiceValidator, cfg.LogLevel),
				LogColor:             cfg.LogColor,
				EnableSignalHandling: true,
				TelemetryCreate:      true,
				TelemetryAddress:     cfg.ValidatorTelemetryAddress,
			},
		},
		Config: *cfg,
	}
	logger := service.NewServiceLogger(&createInfo.ServiceConfigs)
	createInfo.ServiceConfigs.Logger = logger

	var err error
	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer createInfo.Repository.Close()

	validatorService, err := validator.Create(ctx, &createInfo)
	cli.CheckErr(logger, err)

	cli.CheckErr(logger, validatorService.Serve())
}
