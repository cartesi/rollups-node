// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/node"
	"github.com/cartesi/rollups-node/internal/services"
)

func main() {
	startTime := time.Now()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nodeConfig := config.NewNodeConfigFromEnv()

	nodeConfig.Validate()
	config.InitLog(nodeConfig)
	s := node.NewNodeServices(nodeConfig)

	ready := make(chan struct{}, 1)
	// logs startup time
	go func() {
		select {
		case <-ready:
			duration := time.Since(startTime)
			config.InfoLogger.Printf("rollups-node: ready after %s", duration)
		case <-ctx.Done():
		}
	}()

	// start supervisor
	supervisor := services.SupervisorService{
		Name:     "rollups-node",
		Services: s,
	}

	if err := supervisor.Start(ctx, ready); err != nil {
		config.ErrorLogger.Print(err)
	}
}
