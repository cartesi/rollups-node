// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/events"
	eventsPostgres "github.com/cartesi/rollups-node/internal/events/postgres"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	repoPostgres "github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logLevel               string
	logColor               bool
	defaultBlockString     string
	blockchainHttpEndpoint string
	blockchainWsEndpoint   string
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
	Cmd.Flags().StringVarP(&defaultBlockString, "default-block", "d", "finalized",
		"Default block to be used when fetching new blocks.\nOne of 'latest', 'safe', 'pending', 'finalized'")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_DEFAULT_BLOCK, Cmd.Flags().Lookup("default-block")))

	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10001", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "Tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&blockchainHttpEndpoint, "blockchain-http-endpoint", "", "Blockchain http endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_HTTP_ENDPOINT, Cmd.Flags().Lookup("blockchain-http-endpoint")))

	Cmd.Flags().StringVar(&blockchainWsEndpoint, "blockchain-ws-endpoint", "", "Blockchain WS Endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_WS_ENDPOINT, Cmd.Flags().Lookup("blockchain-ws-endpoint")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

	Cmd.Flags().BoolVar(&enableInputReader, "input-reader", true, "Enable or disable the input reader (for external input readers)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_INPUT_READER_ENABLED, Cmd.Flags().Lookup("input-reader")))

	Cmd.Flags().Uint64Var(&maxBlockRange, "max-block-range", 0,
		"Maximum number of blocks in a single query. large queries will be split automatically. Zero for unlimited.")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_MAX_BLOCK_RANGE, Cmd.Flags().Lookup("max-block-range")))

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

	logLevel := config.ResolveServiceLogLevel(config.ServiceEvmReader, cfg.LogLevel)
	createInfo := evmreader.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceEvmReader,
			LogLevel:             logLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
		},
		Config: *cfg,
	}

	var err error
	logger := service.NewLogger(logLevel, cfg.LogColor).With("service", config.ServiceEvmReader)
	authOpt, err := config.HTTPAuthorizationOption()
	cobra.CheckErr(err)
	createInfo.EthClient, err = ethutil.NewEthClient(
		ctx, cfg.BlockchainHttpEndpoint.Raw(), logger,
		ethutil.RetryConfig{
			MaxRetries:   cfg.BlockchainHttpMaxRetries,
			RetryMinWait: cfg.BlockchainHttpRetryMinWait,
			RetryMaxWait: cfg.BlockchainHttpRetryMaxWait,
		}, authOpt)
	cobra.CheckErr(err)

	wsEndpoint := cfg.BlockchainWsEndpoint.Raw()
	createInfo.EthWsClient, err = ethclient.DialContext(ctx, wsEndpoint)
	cobra.CheckErr(ethutil.RedactEndpointFromError(err, wsEndpoint))

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	// Wire PostgreSQL event publisher and subscriber.
	pool := createInfo.Repository.(*repoPostgres.PostgresRepository).Pool()
	publisher := eventsPostgres.NewPublisher(pool, logger)
	createInfo.Publisher = publisher

	connStr := cfg.DatabaseConnection.Raw()
	subscriber := eventsPostgres.NewSubscriber(connStr, logger)
	defer subscriber.Close()
	appChangeCh := subscriber.Subscribe(events.ChannelAppStateChanged)
	createInfo.AppChangeSignal = events.Coalesce(appChangeCh)

	readerService, err := evmreader.Create(ctx, &createInfo)
	cobra.CheckErr(err)
	readerService.LogConfig(createInfo.Config)

	go func() { _ = subscriber.Listen(readerService.Context) }()

	cobra.CheckErr(readerService.Serve())
}
