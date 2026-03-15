// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/events"
	eventsPostgres "github.com/cartesi/rollups-node/internal/events/postgres"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	repoPostgres "github.com/cartesi/rollups-node/internal/repository/postgres"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/cartesi/rollups-node/internal/version"
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
	cfg                *config.ValidatorConfig
)

var Cmd = &cobra.Command{
	Use:     "cartesi-rollups-" + config.ServiceValidator,
	Short:   "Runs cartesi-rollups-" + config.ServiceValidator,
	Long:    "Runs cartesi-rollups-" + config.ServiceValidator + " in standalone mode",
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

	Cmd.Flags().StringVar(&databaseConnection, "database-connection", "",
		"Database connection string in the URL format\n(eg.: 'postgres://user:password@hostname:port/database') ")
	cobra.CheckErr(viper.BindPFlag(config.DATABASE_CONNECTION, Cmd.Flags().Lookup("database-connection")))

	Cmd.Flags().StringVar(&pollInterval, "poll-interval", "7", "Poll interval")
	cobra.CheckErr(viper.BindPFlag(config.VALIDATOR_POLLING_INTERVAL, Cmd.Flags().Lookup("poll-interval")))

	Cmd.Flags().StringVar(&maxStartupTime, "max-startup-time", "15", "Maximum startup time in seconds")
	cobra.CheckErr(viper.BindPFlag(config.MAX_STARTUP_TIME, Cmd.Flags().Lookup("max-startup-time")))

	// TODO: validate on preRunE
	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadValidatorConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MaxStartupTime)
	defer cancel()

	logLevel := config.ResolveServiceLogLevel(config.ServiceValidator, cfg.LogLevel)
	createInfo := validator.CreateInfo{
		CreateInfo: service.CreateInfo{
			Name:                 config.ServiceValidator,
			LogLevel:             logLevel,
			LogColor:             cfg.LogColor,
			EnableSignalHandling: true,
			TelemetryCreate:      true,
			TelemetryAddress:     cfg.TelemetryAddress,
			PollInterval:         cfg.ValidatorPollingInterval,
		},
		Config: *cfg,
	}
	var err error
	createInfo.Repository, err = factory.NewRepositoryFromConnectionString(ctx, cfg.DatabaseConnection.Raw())
	cobra.CheckErr(err)
	defer createInfo.Repository.Close()

	// Wire PostgreSQL event publisher and subscriber.
	logger := service.NewLogger(logLevel, cfg.LogColor).With("service", config.ServiceValidator)
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
		events.ChannelInputsProcessed,
		events.ChannelAppStateChanged,
	)
	createInfo.CreateInfo.EventChannel = events.Coalesce(notifCh)

	validatorService, err := validator.Create(ctx, &createInfo)
	cobra.CheckErr(err)
	validatorService.LogConfig(createInfo.Config)

	go func() { _ = subscriber.Listen(validatorService.Context) }()

	cobra.CheckErr(validatorService.Serve())
}
