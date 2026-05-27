// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
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
	blockchainHTTPEndpoint string
	pollInterval           string
	databaseConnection     string
	maxStartupTime         string
	enableInputReader      bool
	telemetryAddress       string
	cfg                    *config.EvmreaderConfig
	maxBlockRange          uint64
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceEvmReader,
	Short:   "Runs cartesi-rollups-" + config.ServiceEvmReader,
	Long:    "Runs cartesi-rollups-" + config.ServiceEvmReader + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVarP(flags, &defaultBlockString, "default-block", "d", config.BLOCKCHAIN_DEFAULT_BLOCK,
		"Default block to be used when fetching new blocks.\nOne of 'latest', 'safe', 'pending', 'finalized'")
	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.EVM_READER_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"Tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cli.AddFlagStrVar(flags, &blockchainHTTPEndpoint, "blockchain-http-endpoint", config.BLOCKCHAIN_HTTP_ENDPOINT,
		"Blockchain http endpoint")
	cli.AddFlagStrVar(flags, &pollInterval, "poll-interval", config.EVM_READER_POLLING_INTERVAL,
		"Poll interval")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")
	cli.AddFlagBoolVar(flags, &enableInputReader, "input-reader", config.FEATURE_INPUT_READER_ENABLED,
		"Enable or disable the input reader (for external input readers)")
	cli.AddFlagUint64Var(flags, &maxBlockRange, "max-block-range", config.BLOCKCHAIN_MAX_BLOCK_RANGE,
		"Maximum number of blocks in a single query. large queries will be split automatically. Zero for unlimited.")

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadEvmreaderConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	createInfo := evmreader.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceEvmReader,
			LogLevel:             config.ResolveServiceLogLevel(config.ServiceEvmReader, cfg.LogLevel),
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.EvmReaderTelemetryAddress,
			PollInterval:         cfg.EvmReaderPollingInterval,
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
			MaxRetries:     cfg.BlockchainHttpMaxRetries,
			RetryMinWait:   cfg.BlockchainHttpRetryMinWait,
			RetryMaxWait:   cfg.BlockchainHttpRetryMaxWait,
			RequestTimeout: cfg.BlockchainHttpRequestTimeout,
		}, authOpt)
	cli.CheckErr(logger, err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer repo.Close()
	createInfo.Repository = repo

	readerService, err := evmreader.Create(ctx, &createInfo)
	cli.CheckErr(logger, err)
	readerService.LogConfig(createInfo.Config)

	cli.CheckErr(logger, readerService.Serve())
}
