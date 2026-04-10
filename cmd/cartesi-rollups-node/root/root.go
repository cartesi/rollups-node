// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/internal/repository/factory"
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
	logLevelAdvancer       string
	logLevelClaimer        string
	logLevelEvmReader      string
	logLevelJsonrpc        string
	logLevelPrt            string
	logLevelValidator      string
	logLevelEvents         string
	defaultBlockString     string
	blockchainHttpEndpoint string
	blockchainWsEndpoint   string
	databaseConnection     string
	advancerPollInterval   string
	validatorPollInterval  string
	claimerPollInterval    string
	prtPollInterval        string
	maxStartupTime         string
	enableInputReader      bool
	enableInspect          bool
	enableJsonrpc          bool
	enableSubmission       bool
	enableMachineHashCheck bool
	jsonrpcApiAddress      string
	inspectAddress         string
	telemetryAddress       string
	machinelogLevel        string
	cfg                    *config.NodeConfig
	maxBlockRange          uint64
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceNode,
	Short:   "Runs cartesi-rollups-" + config.ServiceNode,
	Long:    "Runs cartesi-rollups-" + config.ServiceNode + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	Cmd.Flags().StringVarP(&defaultBlockString, "default-block", "d", "finalized",
		"Default block to be used when fetching new blocks.\nOne of 'latest', 'safe', 'pending', 'finalized'")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_DEFAULT_BLOCK, Cmd.Flags().Lookup("default-block")))

	Cmd.Flags().StringVar(&jsonrpcApiAddress, "jsonrpc-address", ":10011", "Jsonrpc API service address and port")
	cobra.CheckErr(viper.BindPFlag(config.JSONRPC_API_ADDRESS, Cmd.Flags().Lookup("jsonrpc-address")))

	Cmd.Flags().StringVar(&inspectAddress, "inspect-address", ":10012", "Inspect service address and port")
	cobra.CheckErr(viper.BindPFlag(config.INSPECT_ADDRESS, Cmd.Flags().Lookup("inspect-address")))

	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10000", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "Tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().StringVar(&logLevelAdvancer, "log-level-advancer", "",
		"Override log level for the advancer service (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_ADVANCER, Cmd.Flags().Lookup("log-level-advancer")))

	Cmd.Flags().StringVar(&logLevelClaimer, "log-level-claimer", "",
		"Override log level for the claimer service (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_CLAIMER, Cmd.Flags().Lookup("log-level-claimer")))

	Cmd.Flags().StringVar(&logLevelEvmReader, "log-level-evm-reader", "",
		"Override log level for the evm-reader service (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_EVM_READER, Cmd.Flags().Lookup("log-level-evm-reader")))

	Cmd.Flags().StringVar(&logLevelJsonrpc, "log-level-jsonrpc-api", "",
		"Override log level for the jsonrpc-api service (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_JSONRPC_API, Cmd.Flags().Lookup("log-level-jsonrpc-api")))

	Cmd.Flags().StringVar(&logLevelPrt, "log-level-prt", "",
		"Override log level for the prt service (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_PRT, Cmd.Flags().Lookup("log-level-prt")))

	Cmd.Flags().StringVar(&logLevelValidator, "log-level-validator", "",
		"Override log level for the validator service (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_VALIDATOR, Cmd.Flags().Lookup("log-level-validator")))

	Cmd.Flags().StringVar(&logLevelEvents, "log-level-events", "",
		"Log level for event system messages: publish, subscribe, tick triggers (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_EVENTS, Cmd.Flags().Lookup("log-level-events")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&blockchainHttpEndpoint, "blockchain-http-endpoint", "", "Blockchain HTTP endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_HTTP_ENDPOINT, Cmd.Flags().Lookup("blockchain-http-endpoint")))

	Cmd.Flags().StringVar(&blockchainWsEndpoint, "blockchain-ws-endpoint", "", "Blockchain WS Endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_WS_ENDPOINT, Cmd.Flags().Lookup("blockchain-ws-endpoint")))

	Cmd.Flags().StringVar(&advancerPollInterval, "advancer-poll-interval", "30", "Advancer safety-net poll interval in seconds")
	cobra.CheckErr(viper.BindPFlag(config.ADVANCER_POLLING_INTERVAL, Cmd.Flags().Lookup("advancer-poll-interval")))

	Cmd.Flags().StringVar(&validatorPollInterval, "validator-poll-interval", "30", "Validator safety-net poll interval in seconds")
	cobra.CheckErr(viper.BindPFlag(config.VALIDATOR_POLLING_INTERVAL, Cmd.Flags().Lookup("validator-poll-interval")))

	Cmd.Flags().StringVar(&claimerPollInterval, "claimer-poll-interval", "3", "Claimer safety-net poll interval in seconds")
	cobra.CheckErr(viper.BindPFlag(config.CLAIMER_POLLING_INTERVAL, Cmd.Flags().Lookup("claimer-poll-interval")))

	Cmd.Flags().StringVar(&prtPollInterval, "prt-poll-interval", "3", "PRT safety-net poll interval in seconds")
	cobra.CheckErr(viper.BindPFlag(config.PRT_POLLING_INTERVAL, Cmd.Flags().Lookup("prt-poll-interval")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

	Cmd.Flags().BoolVar(&enableInputReader, "input-reader", true, "Enable or disable the input reader (for external input readers)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_INPUT_READER_ENABLED, Cmd.Flags().Lookup("input-reader")))

	Cmd.Flags().BoolVar(&enableInspect, "inspect-enabled", true, "Enable or disable the inspect service")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_INSPECT_ENABLED, Cmd.Flags().Lookup("inspect-enabled")))

	Cmd.Flags().BoolVar(&enableJsonrpc, "jsonrpc-enabled", true, "Enable or disable the jsonrpc api service")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_JSONRPC_API_ENABLED, Cmd.Flags().Lookup("jsonrpc-enabled")))

	Cmd.Flags().BoolVar(&enableMachineHashCheck, "machine-hash-check", true,
		"Enable or disable machine hash check (DO NOT USE IN PRODUCTION)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_MACHINE_HASH_CHECK_ENABLED, Cmd.Flags().Lookup("machine-hash-check")))

	Cmd.Flags().BoolVar(&enableSubmission, "claim-submission", true, "Enable or disable claim submission (reader mode)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_CLAIM_SUBMISSION_ENABLED, Cmd.Flags().Lookup("claim-submission")))

	Cmd.Flags().StringVar(&machinelogLevel, "machine-log-level", "info",
		"Remote Machine log level: trace, debug, info, warning, error, fatal")
	cobra.CheckErr(viper.BindPFlag(config.JSONRPC_MACHINE_LOG_LEVEL, Cmd.Flags().Lookup("machine-log-level")))

	Cmd.Flags().Uint64Var(&maxBlockRange, "max-block-range", 0,
		"Maximum number of blocks in a single query. large queries will be split automatically. Zero for unlimited.")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_MAX_BLOCK_RANGE, Cmd.Flags().Lookup("max-block-range")))

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadNodeConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func newEthClient(ctx context.Context, svcName string) (*ethclient.Client, error) {
	level := config.ResolveServiceLogLevel(svcName, cfg.LogLevel)
	logger := service.NewLogger(level, cfg.LogColor).With("service", svcName)

	authOpt, err := config.HTTPAuthorizationOption()
	if err != nil {
		return nil, err
	}

	return ethutil.NewEthClient(ctx, cfg.BlockchainHttpEndpoint.Raw(), logger,
		ethutil.RetryConfig{
			MaxRetries:   cfg.BlockchainHttpMaxRetries,
			RetryMinWait: cfg.BlockchainHttpRetryMinWait,
			RetryMaxWait: cfg.BlockchainHttpRetryMaxWait,
		}, authOpt)
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	createInfo := node.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceNode,
			LogLevel:             cfg.LogLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
		},
		Config: *cfg,
	}

	var err error
	createInfo.ReaderClient, err = newEthClient(ctx, config.ServiceEvmReader)
	cobra.CheckErr(err)

	wsEndpoint := cfg.BlockchainWsEndpoint.Raw()
	createInfo.ReaderWSClient, err = ethclient.DialContext(ctx, wsEndpoint)
	cobra.CheckErr(ethutil.RedactEndpointFromError(err, wsEndpoint))

	createInfo.ClaimerClient, err = newEthClient(ctx, config.ServiceClaimer)
	cobra.CheckErr(err)

	createInfo.PrtClient, err = newEthClient(ctx, config.ServicePrt)
	cobra.CheckErr(err)

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	nodeService, err := node.Create(ctx, &createInfo)
	cobra.CheckErr(err)
	nodeService.LogConfig(createInfo.Config)

	cobra.CheckErr(nodeService.Serve())
}
