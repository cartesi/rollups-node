// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services/startup"
	"github.com/cartesi/rollups-node/internal/validator"
	"github.com/spf13/cobra"
)

const CMD_NAME = "validator"

var (
	buildVersion = "devel"
	Cmd          = &cobra.Command{
		Use:   CMD_NAME,
		Short: "Runs Validator",
		Long:  "Runs Validator in standalone mode",
		Run:   run,
	}
	inputBoxDeploymentBlockNumber int64
	pollingInterval               int64
	postgresEndpoint              string
	verbose                       bool
)

func init() {
	Cmd.Flags().Int64VarP(&inputBoxDeploymentBlockNumber,
		"inputbox-block-number",
		"n",
		-1,
		"Input Box deployment block number",
	)
	Cmd.Flags().Int64VarP(
		&pollingInterval,
		"polling-interval",
		"",
		-1,
		"the amount of seconds to wait before trying to finish epochs for all applications",
	)
	Cmd.Flags().StringVarP(&postgresEndpoint, "postgres-endpoint", "p", "", "Postgres endpoint")
	Cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
}

func run(cmd *cobra.Command, args []string) {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := config.FromEnv()

	// Override configs
	if inputBoxDeploymentBlockNumber >= 0 {
		c.ContractsInputBoxDeploymentBlockNumber = inputBoxDeploymentBlockNumber
	}
	if pollingInterval > 0 {
		c.ValidatorPollingInterval = time.Duration(pollingInterval) * time.Second
	}
	if verbose {
		c.LogLevel = slog.LevelDebug
	}
	if postgresEndpoint != "" {
		c.PostgresEndpoint = config.Redacted[string]{Value: postgresEndpoint}
	}

	startup.ConfigLogs(c.LogLevel, c.LogPrettyEnabled)

	slog.Info("Starting the Cartesi Rollups Node Validator", "version", buildVersion, "config", c)

	// Validate Schema
	err := startup.ValidateSchema(c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("failed to validate database schema", "error", err)
		os.Exit(1)
	}

	database, err := repository.Connect(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("failed to connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	_, err = startup.SetupNodePersistentConfig(ctx, database, c)
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	service := validator.NewValidatorService(
		database,
		uint64(c.ContractsInputBoxDeploymentBlockNumber),
		c.ValidatorPollingInterval,
	)

	ready := make(chan struct{}, 1)
	go func() {
		select {
		case <-ready:
			duration := time.Since(startTime)
			slog.Info("validator is ready", "after", duration)
		case <-ctx.Done():
		}
	}()

	// start service
	if err := service.Start(ctx, ready); err != nil {
		slog.Error("validator exited with an error", "error", err)
		os.Exit(1)
	}
}
