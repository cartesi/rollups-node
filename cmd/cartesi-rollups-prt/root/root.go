// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/events"
	eventsPostgres "github.com/cartesi/rollups-node/internal/events/postgres"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10003", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "Tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().String("log-level-events", "",
		"Log level for event system messages: publish, subscribe, tick triggers (default: inherit --log-level)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL_EVENTS, Cmd.Flags().Lookup("log-level-events")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&pollInterval, "poll-interval", "3", "Safety-net poll interval in seconds")
	cobra.CheckErr(viper.BindPFlag(config.PRT_POLLING_INTERVAL, Cmd.Flags().Lookup("poll-interval")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

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

	logLevel := config.ResolveServiceLogLevel(config.ServicePrt, cfg.LogLevel)
	createInfo := prt.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServicePrt,
			LogLevel:             logLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
			PollInterval:         cfg.PrtPollingInterval,
		},
		Config: *cfg,
	}

	var err error
	logger := service.NewLogger(logLevel, cfg.LogColor).With("service", config.ServicePrt)
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

	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	// Wire PostgreSQL event publisher and subscriber.
	eventsLogLevel, hasEventsOverride := config.ResolveEventsLogLevel(logLevel)
	eventsLogger := service.NewLogger(eventsLogLevel, cfg.LogColor).With("service", config.ServicePrt)
	if hasEventsOverride {
		createInfo.CreateInfo.EventLogger = eventsLogger
	}
	pool, err := eventsPostgres.PoolFromRepository(createInfo.Repository)
	cobra.CheckErr(err)
	w := eventsPostgres.Wire(pool, cfg.DatabaseConnection.Raw(), cfg.DatabaseEventsConnection.Raw(),
		eventsLogger,
		events.ChannelClaimComputed, events.ChannelSettleSubmitted,
		events.ChannelJoinSubmitted, events.ChannelAppStateChanged)
	defer w.Subscriber.Close()
	createInfo.Publisher = w.Publisher
	createInfo.CreateInfo.EventChannel = w.Signal

	prtService, err := prt.Create(ctx, &createInfo)
	cobra.CheckErr(err)
	prtService.LogConfig(createInfo.Config)

	w.StartListener(prtService.Context)

	cobra.CheckErr(prtService.Serve())
}
