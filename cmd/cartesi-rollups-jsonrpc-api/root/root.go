// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logLevel           string
	logColor           bool
	databaseConnection string
	maxStartupTime     string
	telemetryAddress   string
	jsonrpcApiAddress  string
	cfg                *config.JsonrpcConfig
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceJsonrpc,
	Short:   "Runs cartesi-rollups-" + config.ServiceJsonrpc,
	Long:    "Runs cartesi-rollups-" + config.ServiceJsonrpc + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	Cmd.Flags().StringVar(&jsonrpcApiAddress, "jsonrpc-address", ":10011", "Jsonrpc API service address and port")
	cobra.CheckErr(viper.BindPFlag(config.JSONRPC_API_ADDRESS, Cmd.Flags().Lookup("jsonrpc-address")))

	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10005", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "Tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadJsonrpcConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	createInfo := jsonrpc.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceJsonrpc,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceJsonrpc, cfg.LogLevel),
			LogColor:             cfg.LogColor,
			EnableSignalHandling: false,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
		},
		Config: *cfg,
	}
	var err error
	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.String())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	jsonrpcService, err := jsonrpc.Create(ctx, &createInfo)
	cobra.CheckErr(err)

	cobra.CheckErr(jsonrpcService.Serve())
}
