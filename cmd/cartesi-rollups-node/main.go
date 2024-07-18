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
	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
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

	config := config.FromEnv()

	// setup log
	opts := &tint.Options{
		Level:      config.LogLevel,
		AddSource:  config.LogLevel == slog.LevelDebug,
		NoColor:    !config.LogPrettyEnabled || !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: "2006-01-02T15:04:05.000", // RFC3339 with milliseconds and without timezone
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("Starting the Cartesi Rollups Node", "version", buildVersion, "config", config)

	schemaManager, err := repository.NewSchemaManager(config.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("Node exited with an error", "error", err)
		schemaManager.Close()
		os.Exit(1)
	}
	err = schemaManager.ValidateSchemaVersion()
	if err != nil {
		slog.Error("Node exited with an error", "error", err)
		schemaManager.Close()
		os.Exit(1)
	}
	schemaManager.Close()

	// setup database
	nodePersistentConfig := NodePersistentConfig{
		DefaultBlock:            config.EvmReaderDefaultBlock,
		InputBoxDeploymentBlock: uint64(config.ContractsInputBoxDeploymentBlockNumber),
		InputBoxAddress:         common.HexToAddress(config.ContractsInputBoxAddress),
		ChainId:                 config.BlockchainID,
		IConsensusAddress:       common.HexToAddress(config.ContractsIConsensusAddress),
	}

	database, err := repository.Connect(ctx, config.PostgresEndpoint.Value)
	if err != nil {
		slog.Error("Node couldn't connect to the database", "error", err)
		database.Close()
		os.Exit(1)
	}
	err = database.InsertNodeConfig(ctx, &nodePersistentConfig)
	if err != nil {
		slog.Error("Node couldn't insert database config", "error", err)
		database.Close()
		os.Exit(1)
	}
	database.Close()

	// create the node supervisor
	supervisor, err := node.Setup(ctx, config, "")
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
