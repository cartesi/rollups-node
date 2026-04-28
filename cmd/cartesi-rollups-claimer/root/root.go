// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
)

var (
	logLevel               string
	logColor               bool
	defaultBlockString     string
	blockchainHttpEndpoint string
	databaseConnection     string
	pollInterval           string
	maxStartupTime         string
	enableSubmission       bool
	telemetryAddress       string
	cfg                    *config.ClaimerConfig
	maxBlockRange          uint64
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceClaimer,
	Short:   "Runs cartesi-rollups-" + config.ServiceClaimer,
	Long:    "Runs cartesi-rollups-" + config.ServiceClaimer + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVarP(flags, &defaultBlockString, "default-block", "d", config.BLOCKCHAIN_DEFAULT_BLOCK,
		"Default block to be used when fetching new blocks.\nOne of 'latest', 'safe', 'pending', 'finalized'")
	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.CLAIMER_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cli.AddFlagStrVar(flags, &blockchainHttpEndpoint, "blockchain-http-endpoint", config.BLOCKCHAIN_HTTP_ENDPOINT,
		"Blockchain http endpoint")
	cli.AddFlagStrVar(flags, &pollInterval, "poll-interval", config.CLAIMER_POLLING_INTERVAL,
		"Poll interval")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")
	cli.AddFlagBoolVar(flags, &enableSubmission, "claim-submission", config.FEATURE_CLAIM_SUBMISSION_ENABLED,
		"Enable or disable claim submission (reader mode)")
	cli.AddFlagUint64Var(flags, &maxBlockRange, "max-block-range", config.BLOCKCHAIN_MAX_BLOCK_RANGE,
		"Maximum number of blocks in a single query. large queries will be split automatically. Zero for unlimited.")

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadClaimerConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	createInfo := claimer.CreateInfo{
		TickServiceConfigs: service.TickServiceConfigs{
			PollInterval:   cfg.ClaimerPollingInterval,
			ServiceConfigs: service.ServiceConfigs{
				Name:                 config.ServiceClaimer,
				LogLevel:             config.ResolveServiceLogLevel(config.ServiceClaimer, cfg.LogLevel),
				LogColor:             cfg.LogColor,
				EnableSignalHandling: true,
				TelemetryCreate:      true,
				TelemetryAddress:     cfg.ClaimerTelemetryAddress,
			},
		},
		Config: *cfg,
	}
	logger := service.NewServiceLogger(&createInfo.ServiceConfigs)
	createInfo.ServiceConfigs.Logger = logger

	authOpt, err := config.HTTPAuthorizationOption()
	cli.CheckErr(logger, err)
	createInfo.EthConn, err = ethutil.NewEthClient(
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

	claimerService, err := claimer.Create(ctx, &createInfo)
	cli.CheckErr(logger, err)

	err = claimerService.Serve()
	cli.CheckErr(logger, err)
}
