// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const serviceName = "prt"

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
	Use:     "cartesi-rollups-" + serviceName,
	Short:   "Runs cartesi-rollups-" + serviceName,
	Long:    "Runs cartesi-rollups-" + serviceName + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10003", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "Tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&pollInterval, "poll-interval", "7", "Poll interval")
	cobra.CheckErr(viper.BindPFlag(config.VALIDATOR_POLLING_INTERVAL, Cmd.Flags().Lookup("poll-interval")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

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

	createInfo := prt.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 serviceName,
			LogLevel:             cfg.LogLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
			PollInterval:         cfg.ValidatorPollingInterval,
		},
		Config: *cfg,
	}
	var err error
	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.String())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	validatorService, err := prt.Create(ctx, &createInfo)
	cobra.CheckErr(err)

	cobra.CheckErr(validatorService.Serve())
}
