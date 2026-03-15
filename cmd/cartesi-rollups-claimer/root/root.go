// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/events"
	eventsPostgres "github.com/cartesi/rollups-node/internal/events/postgres"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	repoPostgres "github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	Cmd.Flags().StringVarP(&defaultBlockString, "default-block", "d", "finalized",
		"Default block to be used when fetching new blocks.\nOne of 'latest', 'safe', 'pending', 'finalized'")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_DEFAULT_BLOCK, Cmd.Flags().Lookup("default-block")))

	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10004", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&blockchainHttpEndpoint, "blockchain-http-endpoint", "", "Blockchain http endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_HTTP_ENDPOINT, Cmd.Flags().Lookup("blockchain-http-endpoint")))

	Cmd.Flags().StringVar(&pollInterval, "poll-interval", "7", "Poll interval")
	cobra.CheckErr(viper.BindPFlag(config.CLAIMER_POLLING_INTERVAL, Cmd.Flags().Lookup("poll-interval")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

	Cmd.Flags().BoolVar(&enableSubmission, "claim-submission", true, "Enable or disable claim submission (reader mode)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_CLAIM_SUBMISSION_ENABLED, Cmd.Flags().Lookup("claim-submission")))

	Cmd.Flags().Uint64Var(&maxBlockRange, "max-block-range", 0,
		"Maximum number of blocks in a single query. large queries will be split automatically. Zero for unlimited.")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_MAX_BLOCK_RANGE, Cmd.Flags().Lookup("max-block-range")))

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

	logLevel := config.ResolveServiceLogLevel(config.ServiceClaimer, cfg.LogLevel)
	createInfo := claimer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceClaimer,
			LogLevel:             logLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
			PollInterval:         cfg.ClaimerPollingInterval,
		},
		Config: *cfg,
	}

	logger := service.NewLogger(logLevel, cfg.LogColor).With("service", config.ServiceClaimer)
	authOpt, err := config.HTTPAuthorizationOption()
	cobra.CheckErr(err)
	createInfo.EthConn, err = ethutil.NewEthClient(
		ctx, cfg.BlockchainHttpEndpoint.Raw(), logger,
		ethutil.RetryConfig{
			MaxRetries:   cfg.BlockchainHttpMaxRetries,
			RetryMinWait: cfg.BlockchainHttpRetryMinWait,
			RetryMaxWait: cfg.BlockchainHttpRetryMaxWait,
		}, authOpt)
	cobra.CheckErr(err)

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	// Wire PostgreSQL event publisher and subscriber.
	pool := createInfo.Repository.(*repoPostgres.PostgresRepository).Pool()
	publisher := eventsPostgres.NewPublisher(pool, logger)
	createInfo.Publisher = publisher

	connStr := cfg.DatabaseConnection.Raw()
	subscriber := eventsPostgres.NewSubscriber(connStr, logger, nil)
	defer subscriber.Close()
	notifCh := subscriber.Subscribe(
		events.ChannelClaimComputed,
		events.ChannelClaimSubmitted,
		events.ChannelAppStateChanged,
	)
	createInfo.CreateInfo.EventChannel = events.Coalesce(notifCh)

	claimerService, err := claimer.Create(ctx, &createInfo)
	cobra.CheckErr(err)
	claimerService.LogConfig(createInfo.Config)

	go func() { _ = subscriber.Listen(claimerService.Context) }()

	err = claimerService.Serve()
	cobra.CheckErr(err)
}
