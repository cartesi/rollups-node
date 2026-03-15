// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/advancer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/events"
	eventsPostgres "github.com/cartesi/rollups-node/internal/events/postgres"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	repoPostgres "github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logLevel               string
	logColor               bool
	databaseConnection     string
	pollInterval           string
	maxStartupTime         string
	telemetryAddress       string
	inspectAddress         string
	machinelogLevel        string
	enableMachineHashCheck bool
	enableInspect          bool
	cfg                    *config.AdvancerConfig
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceAdvancer,
	Short:   "Runs cartesi-rollups-" + config.ServiceAdvancer,
	Long:    "Runs cartesi-rollups-" + config.ServiceAdvancer + " in standalone mode",
	Run:     run,
	Version: version.BuildVersion,
}

func init() {
	Cmd.Flags().StringVar(&inspectAddress, "inspect-address", ":10012", "Inspect service address and port")
	cobra.CheckErr(viper.BindPFlag(config.INSPECT_ADDRESS, Cmd.Flags().Lookup("inspect-address")))

	Cmd.Flags().StringVar(&telemetryAddress, "telemetry-address", ":10002", "Health check and metrics address and port")
	cobra.CheckErr(viper.BindPFlag(config.TELEMETRY_ADDRESS, Cmd.Flags().Lookup("telemetry-address")))

	Cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(config.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))

	Cmd.Flags().BoolVar(&logColor, "log-color", true, "Tint the logs (colored output)")
	cobra.CheckErr(viper.BindPFlag(config.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&pollInterval, "poll-interval", "7", "Poll interval")
	cobra.CheckErr(viper.BindPFlag(config.ADVANCER_POLLING_INTERVAL, Cmd.Flags().Lookup("poll-interval")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

	Cmd.Flags().BoolVar(&enableInspect, "inspect-enabled", true, "Enable or disable the inspect service")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_INSPECT_ENABLED, Cmd.Flags().Lookup("inspect-enabled")))

	Cmd.Flags().BoolVar(&enableMachineHashCheck, "machine-hash-check", true,
		"Enable or disable machine hash check (DO NOT USE IN PRODUCTION)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_MACHINE_HASH_CHECK_ENABLED, Cmd.Flags().Lookup("machine-hash-check")))

	Cmd.Flags().StringVar(&machinelogLevel, "machine-log-level", "info",
		"Remote Machine log level: trace, debug, info, warning, error, fatal")
	cobra.CheckErr(viper.BindPFlag(config.JSONRPC_MACHINE_LOG_LEVEL, Cmd.Flags().Lookup("machine-log-level")))

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadAdvancerConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	logLevel := config.ResolveServiceLogLevel(config.ServiceAdvancer, cfg.LogLevel)
	createInfo := advancer.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceAdvancer,
			LogLevel:             logLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
			PollInterval:         cfg.AdvancerPollingInterval,
		},
		Config: *cfg,
	}
	var err error
	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	// Wire PostgreSQL event publisher and subscriber.
	logger := service.NewLogger(logLevel, cfg.LogColor).With("service", config.ServiceAdvancer)
	pool := createInfo.Repository.(*repoPostgres.PostgresRepository).Pool()
	publisher := eventsPostgres.NewPublisher(pool, logger)
	createInfo.Publisher = publisher

	eventsConnStr := cfg.DatabaseEventsConnection.Raw()
	if eventsConnStr == "" {
		eventsConnStr = cfg.DatabaseConnection.Raw()
	}
	subscriber := eventsPostgres.NewSubscriber(eventsConnStr, logger, nil)
	defer subscriber.Close()
	notifCh := subscriber.Subscribe(
		events.ChannelInputReceived,
		events.ChannelEpochClosed,
		events.ChannelAppStateChanged,
	)
	createInfo.CreateInfo.EventChannel = events.Coalesce(notifCh)

	advancerService, err := advancer.Create(ctx, &createInfo)
	cobra.CheckErr(err)
	advancerService.LogConfig(createInfo.Config)

	go func() { _ = subscriber.Listen(advancerService.Context) }()

	cobra.CheckErr(advancerService.Serve())
}
