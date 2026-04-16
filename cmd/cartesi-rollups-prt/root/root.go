// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/ethutil"
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
	cfg                *config.PrtConfig
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServicePrt,
	Short:   "Runs cartesi-rollups-" + config.ServicePrt,
	Long:    "Runs cartesi-rollups-" + config.ServicePrt + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.PRT_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"Tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cli.AddFlagStrVar(flags, &pollInterval, "poll-interval", config.PRT_POLLING_INTERVAL,
		"Poll interval")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadPrtConfig()
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
			Name:                 config.ServicePrt,
			LogLevel:             config.ResolveServiceLogLevel(config.ServicePrt, cfg.LogLevel),
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.PrtTelemetryAddress,
			PollInterval:         cfg.PrtPollingInterval,
		},
		Config: *cfg,
	}
	logger := service.NewServiceLogger(&createInfo.CreateInfo)
	createInfo.CreateInfo.Logger = logger

	var err error
	authOpt, err := config.HTTPAuthorizationOption()
	cli.CheckErr(logger, err)
	createInfo.EthClient, err = ethutil.NewEthClient(
		ctx, cfg.BlockchainHttpEndpoint.Raw(), logger,
		ethutil.RetryConfig{
			MaxRetries:   cfg.BlockchainHttpMaxRetries,
			RetryMinWait: cfg.BlockchainHttpRetryMinWait,
			RetryMaxWait: cfg.BlockchainHttpRetryMaxWait,
		}, authOpt)
	cli.CheckErr(logger, err)

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer createInfo.Repository.Close()

	prtService, err := prt.Create(ctx, &createInfo)
	cli.CheckErr(logger, err)

	cli.CheckErr(logger, prtService.Serve())
}
