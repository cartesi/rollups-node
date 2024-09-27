// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/cartesi/rollups-node/internal/node/advancer"
	"github.com/cartesi/rollups-node/internal/node/advancer/machines"
	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/node/startup"
	"github.com/cartesi/rollups-node/internal/repository"

	"github.com/spf13/cobra"
)

const CMD_NAME = "advancer"

var (
	buildVersion = "devel"
	Cmd          = &cobra.Command{
		Use:   CMD_NAME,
		Short: "Runs the Advancer",
		Long:  "Runs the Advancer in standalone mode",
		RunE:  run,
	}
)

func main() {
	err := Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func getDatabase(ctx context.Context, endpoint string) (*repository.Database, error) {
	err := startup.ValidateSchema(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid database schema: %w", err)
	}

	database, err := repository.Connect(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	return database, nil
}

func run(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := config.GetAdvancerConfig()
	startup.ConfigLogs(c.LogLevel, c.LogPrettyEnabled)

	slog.Info("Starting the Cartesi Rollups Node Advancer", "version", buildVersion, "config", c)

	database, err := getDatabase(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		return err
	}
	defer database.Close()

	repo := &repository.MachineRepository{Database: database}

	machines, err := machines.Load(ctx, repo, c.MachineServerVerbosity)
	if err != nil {
		return fmt.Errorf("failed to load the machines: %w", err)
	}
	defer machines.Close()

	advancer, err := advancer.New(machines, repo)
	if err != nil {
		return fmt.Errorf("failed to create the advancer: %w", err)
	}

	poller, err := advancer.Poller(c.AdvancerPollingInterval)
	if err != nil {
		return fmt.Errorf("failed to create the advancer service: %w", err)
	}

	return poller.Start(ctx)
}
