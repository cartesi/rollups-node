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
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/services/startup"
	"github.com/spf13/cobra"
)

const CMD_NAME = "node"

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
	Cmd          = &cobra.Command{
		Use:   CMD_NAME,
		Short: "Runs the Cartesi Rollups Node",
		Long:  "Runs the Cartesi Rollups Node as a single process",
		RunE:  run,
	}
)

func run(cmd *cobra.Command, args []string) error {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := config.FromEnv()

	// setup log
	startup.ConfigLogs(config.LogLevel, config.LogPrettyEnabled)
	slog.Info("Starting the Cartesi Rollups Node", "version", buildVersion, "config", config)

	database, err := repository.Connect(ctx, config.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("Node couldn't connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	_, err = startup.SetupNodePersistentConfig(ctx, database, config)
	if err != nil {
		slog.Error("Node exited with an error", "error", err)
		os.Exit(1)
	}

	// create the node supervisor
	supervisor, err := node.Setup(ctx, config, "", database)
	if err != nil {
		slog.Error("Node exited with an error", "error", err)
		os.Exit(1)
	}

	// logs startup time
	ready := make(chan struct{}, 1)
	go func() {
		select {
		case <-ready:
			duration := time.Since(startTime)
			slog.Info("Node is ready", "after", duration)
		case <-ctx.Done():
		}
	}()

	// start supervisor
	if err := supervisor.Start(ctx, ready); err != nil {
		slog.Error("Node exited with an error", "error", err)
		os.Exit(1)
	}

	return err
}
