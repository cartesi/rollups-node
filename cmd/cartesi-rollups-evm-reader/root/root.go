// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/hashicorp/go-retryablehttp"

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

func createEthClient(ctx context.Context, endpoint string, logger *slog.Logger) (*ethclient.Client, error) {
	rclient := retryablehttp.NewClient()
	rclient.Logger = logger
	rclient.RetryMax = int(cfg.BlockchainHttpMaxRetries)
	rclient.RetryWaitMin = cfg.BlockchainHttpRetryMinWait
	rclient.RetryWaitMax = cfg.BlockchainHttpRetryMaxWait

	clientOptions := []rpc.ClientOption{
		rpc.WithHTTPClient(rclient.StandardClient()),
	}

	authOpt, err := config.HTTPAuthorizationOption()
	cobra.CheckErr(err)
	if authOpt != nil {
		clientOptions = append(clientOptions, authOpt)
	}

	rpcClient, err := rpc.DialOptions(ctx, endpoint, clientOptions...)
	if err != nil {
		return nil, err
	}

	return ethclient.NewClient(rpcClient), nil
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
	createInfo.EthClient, err = createEthClient(ctx, cfg.BlockchainHttpEndpoint.String(), logger)
	cobra.CheckErr(err)

	createInfo.EthWsClient, err = ethclient.DialContext(ctx, cfg.BlockchainWsEndpoint.String())
	cobra.CheckErr(err)

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.String())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	readerService, err := evmreader.Create(ctx, &createInfo)
	cobra.CheckErr(err)

	cobra.CheckErr(readerService.Serve())
}
