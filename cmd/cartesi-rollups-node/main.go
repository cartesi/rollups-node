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
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

func main() {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := config.FromEnv()

	// setup log
	opts := &tint.Options{
		Level:     config.LogLevel,
		AddSource: config.LogLevel == slog.LevelDebug,
		NoColor:   !config.LogPretty || !isatty.IsTerminal(os.Stdout.Fd()),
	}
	handler := tint.NewHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("Starting the Cartesi Rollups Node", "config", config)

	// create the node supervisor
	supervisor, err := node.Setup(ctx, config)
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
