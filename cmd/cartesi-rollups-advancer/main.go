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
	"time"

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
		Run:   run,
	}
)

func init() {
	flags := Cmd.Flags()
	flags.BytesHex("application-address", nil, "")
	flags.String("server-address", "", "")
	flags.String("snapshot", "", "")
	flags.Int64("snapshot-input-index", -1, "")
	flags.Uint64("machine-inc-cycles", 50_000_000, "")
	flags.Uint64("machine-max-cycles", 5_000_000_000, "")
	flags.Uint64("machine-advance-timeout", 60, "")
	flags.Uint64("machine-inspect-timeout", 10, "")
}

func main() {
	err := Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func getDatabase(ctx context.Context, c config.NodeConfig) (*repository.Database, error) {
	err := startup.ValidateSchema(c)
	if err != nil {
		return nil, fmt.Errorf("invalid database schema: %w", err)
	}

	database, err := repository.Connect(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	return database, nil
}

func run(cmd *cobra.Command, args []string) {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := config.FromEnv()
	startup.ConfigLogs(c)

	slog.Info("Starting the Cartesi Rollups Node Advancer", "version", buildVersion, "config", c)

	database, err := getDatabase(ctx, c)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	defer database.Close()

	repo := &repository.AdvancerRepository{Database: database}

	machines, err := machines.Load(ctx, c, repo)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	defer machines.Close()

	advancer, err := advancer.New(machines, repo)

	poller, err := advancer.Poller(5 * time.Second)

	ready := make(chan struct{}, 1)

	if err := poller.Start(ctx, ready); err != nil {
		slog.Error("advancer exited with an error", "error", err)
		os.Exit(1)
	}
}
