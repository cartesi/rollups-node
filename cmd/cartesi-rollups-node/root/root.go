// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/inspect"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/manager"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

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

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	// Create shared components

	name := config.ServiceNode
	logger := service.NewLogger(name, cfg.LogLevel, cfg.LogColor)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cli.CheckErr(logger, err)
	defer repo.Close()

	// create Machine Manager using the same logger used by Advancer
	advCfg := cfg.ToAdvancerConfig()
	advLogger := service.NewLogger(config.ServiceAdvancer, advCfg.LogLevel, advCfg.LogColor)
	machineManager := manager.NewMachineManager(
		repo,
		advLogger,
		cfg.FeatureMachineHashCheckEnabled,
		cfg.AdvancerInputBatchSize,
	)
	defer machineManager.Close()

	// Create factories of services

	factories := []service.FactoryFunction{
		func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
			return evmreader.Create(ctx, &evmreader.CreateInfo{
				Config:     *cfg.ToEvmreaderConfig(),
				Repository: repo,
			})
		},
		func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
			adv, err := advancer.Create(ctx, &advancer.CreateInfo{
				Config:     *advCfg,
				Logger:     advLogger,
				Repository: repo,
				Machines:   machineManager,
				Supervisor: sup,
			})
			return adv, err
		},
		func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
			return validator.Create(ctx, &validator.CreateInfo{
				Config:     *cfg.ToValidatorConfig(),
				Repository: repo,
			})
		},
		func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
			return claimer.Create(ctx, &claimer.CreateInfo{
				Config:     *cfg.ToClaimerConfig(),
				Repository: repo,
			})
		},
		func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
			return prt.Create(ctx, &prt.CreateInfo{
				Config:     *cfg.ToPrtConfig(),
				Repository: repo,
			})
		},
	}

	if cfg.FeatureInspectEnabled {
		factories = append(factories,
			func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
				return inspect.Create(ctx, &inspect.CreateInfo{
					Config:     *advCfg,
					Repository: repo,
					Machines:   machineManager,
				})
			},
		)
	}
	if cfg.FeatureJsonrpcApiEnabled {
		factories = append(factories,
			func(ctx context.Context, sup *service.Supervisor) (service.SupervisedService, error) {
				cfg := jsonrpc.CreateInfo{
					Config:     *cfg.ToJsonrpcConfig(),
					Repository: repo,
				}
				return jsonrpc.Create(ctx, &cfg)
			},
		)
	}

	logger.Info("Created", "config", cfg)

	supCfg := &service.SupervisorConfigs{
		BaseConfigs:          service.BaseConfigs{Name: name, Logger: logger},
		EnableSignalHandling: true,
		TelemetryCreate:      true,
		TelemetryAddress:     cfg.NodeTelemetryAddress,
		Factories:            factories,
	}
	sup, err := service.NewSupervisor(ctx, supCfg)
	cli.CheckErr(logger, err)
	defer sup.Close()
	cli.CheckErr(logger, sup.Serve())
}
