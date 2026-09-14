// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/repository"
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
	RunE:    run,
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

func run(cmd *cobra.Command, args []string) error {
	return cli.RunSingleService(cli.SingleServiceOptions{
		Command:            cmd,
		Name:               config.ServiceJsonrpc,
		LogLevel:           cfg.LogLevel,
		LogColor:           cfg.LogColor,
		MaxStartupTime:     cfg.MaxStartupTime,
		DatabaseConnection: cfg.DatabaseConnection,
		TelemetryAddress:   cfg.JsonrpcTelemetryAddress,
		Create: func(ctx context.Context, logger *slog.Logger, repo repository.Repository) (service.SupervisedService, error) {
			return jsonrpc.Create(ctx, &jsonrpc.CreateInfo{Config: *cfg, Logger: logger, Repository: repo})
		},
	})
}
