// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
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
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVar(flags, &jsonrpcApiAddress, "jsonrpc-address", config.JSONRPC_API_ADDRESS,
		"Jsonrpc API service address and port")
	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.JSONRPC_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"Tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")

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
		ServiceConfigs: service.ServiceConfigs{
			Name:                 config.ServiceJsonrpc,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceJsonrpc, cfg.LogLevel),
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.JsonrpcTelemetryAddress,
		},
		Config: *cfg,
	}
	logger := service.NewServiceLogger(&createInfo.ServiceConfigs)
	createInfo.ServiceConfigs.Logger = logger

	var err error
	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer createInfo.Repository.Close()

	jsonrpcService, err := jsonrpc.Create(ctx, &createInfo)
	cli.CheckErr(logger, err)

	cli.CheckErr(logger, jsonrpcService.Serve())
}
