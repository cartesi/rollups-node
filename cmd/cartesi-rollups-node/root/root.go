// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"time"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/spf13/cobra"
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
	defaultBlockString     string
	blockchainHTTPEndpoint string
	databaseConnection     string
	evmReaderPollInterval  string
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
	jsonrpcAPIAddress      string
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
	flags := Cmd.Flags()

	config.SetDefaults()

	cli.AddFlagStrVarP(flags, &defaultBlockString, "default-block", "d", config.BLOCKCHAIN_DEFAULT_BLOCK,
		"Default block to be used when fetching new blocks.\nOne of 'latest', 'safe', 'pending', 'finalized'")
	cli.AddFlagStrVar(flags, &jsonrpcAPIAddress, "jsonrpc-address", config.JSONRPC_API_ADDRESS,
		"Jsonrpc API service address and port")
	cli.AddFlagStrVar(flags, &inspectAddress, "inspect-address", config.INSPECT_ADDRESS,
		"Inspect service address and port")
	cli.AddFlagStrVar(flags, &telemetryAddress, "telemetry-address", config.NODE_TELEMETRY_ADDRESS,
		"Health check and metrics address and port")
	cli.AddFlagStrVar(flags, &logLevel, "log-level", config.LOG_LEVEL,
		"Log level: debug, info, warn or error")
	cli.AddFlagBoolVar(flags, &logColor, "log-color", config.LOG_COLOR,
		"Tint the logs (colored output)")
	cli.AddFlagStrVar(flags, &logLevelAdvancer, "log-level-advancer", config.LOG_LEVEL_ADVANCER,
		"Override log level for the advancer service (default: inherit --log-level)")
	cli.AddFlagStrVar(flags, &logLevelClaimer, "log-level-claimer", config.LOG_LEVEL_CLAIMER,
		"Override log level for the claimer service (default: inherit --log-level)")
	cli.AddFlagStrVar(flags, &logLevelEvmReader, "log-level-evm-reader", config.LOG_LEVEL_EVM_READER,
		"Override log level for the evm-reader service (default: inherit --log-level)")
	cli.AddFlagStrVar(flags, &logLevelJsonrpc, "log-level-jsonrpc-api", config.LOG_LEVEL_JSONRPC_API,
		"Override log level for the jsonrpc-api service (default: inherit --log-level)")
	cli.AddFlagStrVar(flags, &logLevelPrt, "log-level-prt", config.LOG_LEVEL_PRT,
		"Override log level for the prt service (default: inherit --log-level)")
	cli.AddFlagStrVar(flags, &logLevelValidator, "log-level-validator", config.LOG_LEVEL_VALIDATOR,
		"Override log level for the validator service (default: inherit --log-level)")
	cli.AddFlagStrVar(flags, &databaseConnection, "database-connection", config.DATABASE_CONNECTION,
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cli.AddFlagStrVar(flags, &blockchainHTTPEndpoint, "blockchain-http-endpoint", config.BLOCKCHAIN_HTTP_ENDPOINT,
		"Blockchain HTTP endpoint")
	cli.AddFlagStrVar(flags, &evmReaderPollInterval, "evm-reader-poll-interval", config.EVM_READER_POLLING_INTERVAL,
		"EVM reader poll interval")
	cli.AddFlagStrVar(flags, &advancerPollInterval, "advancer-poll-interval", config.ADVANCER_POLLING_INTERVAL,
		"Advancer poll interval")
	cli.AddFlagStrVar(flags, &validatorPollInterval, "validator-poll-interval", config.VALIDATOR_POLLING_INTERVAL,
		"Validator poll interval")
	cli.AddFlagStrVar(flags, &claimerPollInterval, "claimer-poll-interval", config.CLAIMER_POLLING_INTERVAL,
		"Claimer poll interval")
	cli.AddFlagStrVar(flags, &prtPollInterval, "prt-poll-interval", config.PRT_POLLING_INTERVAL,
		"PRT poll interval")
	cli.AddFlagStrVar(flags, &maxStartupTime, "max-startup-time", config.MAX_STARTUP_TIME,
		"Maximum startup time in seconds")
	cli.AddFlagBoolVar(flags, &enableInputReader, "input-reader", config.FEATURE_INPUT_READER_ENABLED,
		"Enable or disable the input reader (for external input readers)")
	cli.AddFlagBoolVar(flags, &enableInspect, "inspect-enabled", config.FEATURE_INSPECT_ENABLED,
		"Enable or disable the inspect service")
	cli.AddFlagBoolVar(flags, &enableJsonrpc, "jsonrpc-enabled", config.FEATURE_JSONRPC_API_ENABLED,
		"Enable or disable the jsonrpc api service")
	cli.AddFlagBoolVar(flags, &enableMachineHashCheck, "machine-hash-check", config.FEATURE_MACHINE_HASH_CHECK_ENABLED,
		"Enable or disable machine hash check (DO NOT USE IN PRODUCTION)")
	cli.AddFlagBoolVar(flags, &enableSubmission, "claim-submission", config.FEATURE_CLAIM_SUBMISSION_ENABLED,
		"Enable or disable claim submission (reader mode)")
	cli.AddFlagStrVar(flags, &machinelogLevel, "machine-log-level", config.JSONRPC_MACHINE_LOG_LEVEL,
		"Remote Machine log level: trace, debug, info, warning, error, fatal")
	cli.AddFlagUint64Var(flags, &maxBlockRange, "max-block-range", config.BLOCKCHAIN_MAX_BLOCK_RANGE,
		"Maximum number of blocks in a single query. large queries will be split automatically. Zero for unlimited.")

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

func newEthClient(ctx context.Context, svcName string, requestTimeout time.Duration) (*ethclient.Client, error) {
	level := config.ResolveServiceLogLevel(svcName, cfg.LogLevel)
	logger := service.NewLogger(level, cfg.LogColor).With("service", svcName)

	authOpt, err := config.HTTPAuthorizationOption()
	if err != nil {
		return nil, err
	}

	return ethutil.NewEthClient(ctx, cfg.BlockchainHttpEndpoint.Raw(), logger,
		ethutil.RetryConfig{
			MaxRetries:     cfg.BlockchainHttpMaxRetries,
			RetryMinWait:   cfg.BlockchainHttpRetryMinWait,
			RetryMaxWait:   cfg.BlockchainHttpRetryMaxWait,
			RequestTimeout: requestTimeout,
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
			TelemetryAddress:     cfg.NodeTelemetryAddress,
		},
		Config: *cfg,
	}
	logger := service.NewServiceLogger(&createInfo.CreateInfo)
	createInfo.CreateInfo.Logger = logger

	var err error
	createInfo.ReaderClient, err = newEthClient(ctx, config.ServiceEvmReader, cfg.BlockchainHttpRequestTimeout)
	cli.CheckErr(logger, err)

	createInfo.ClaimerClient, err = newEthClient(ctx, config.ServiceClaimer, cfg.BlockchainHttpRequestTimeout)
	cli.CheckErr(logger, err)

	createInfo.PrtClient, err = newEthClient(ctx, config.ServicePrt, cfg.BlockchainHttpRequestTimeout)
	cli.CheckErr(logger, err)

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer createInfo.Repository.Close()

	nodeService, err := node.Create(ctx, &createInfo)
	cli.CheckErr(logger, err)
	nodeService.LogConfig(createInfo.Config)

	cli.CheckErr(logger, nodeService.Serve())
}
