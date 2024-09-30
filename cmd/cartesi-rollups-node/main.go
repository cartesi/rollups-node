// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/node/startup"
	"github.com/cartesi/rollups-node/internal/repository"
)

var (
	// Should be overridden during the final release build with ldflags
	// to contain the actual version number
	buildVersion = "devel"
)

func main() {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := config.FromEnv()

	// setup log
	startup.ConfigLogs(c.LogLevel, c.LogPrettyEnabled)
	slog.Info("Starting the Cartesi Rollups Node", "version", buildVersion, "config", c)

	err := startup.ValidateSchema(c.PostgresEndpoint.Value, c.PostgresSslDisabled)
	if err != nil {
		slog.Error("Node exited with an error", "error", err)
		os.Exit(1)
	}

	database, err := repository.Connect(ctx, c.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("Node couldn't connect to the database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	_, err = startup.SetupNodePersistentConfig(ctx, database, c.EvmReaderDefaultBlock,
		c.ContractsInputBoxAddress, uint64(c.ContractsInputBoxDeploymentBlockNumber),
		c.BlockchainID)
	if err != nil {
		slog.Error("Node exited with an error", "error", err)
		os.Exit(1)
	}

	// create the node supervisor
	supervisor, err := node.Setup(ctx, c, "", database)
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
}
