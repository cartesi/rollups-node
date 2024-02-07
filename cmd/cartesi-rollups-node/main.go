// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/services"
)

func main() {
	startTime := time.Now()
	var s []services.Service

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nodeConfig := config.NewNodeConfigFromEnv()
	nodeConfig.Validate()

	config.InitLog(nodeConfig)
	slog.Info("Starting Cartesi Rollups Node", "config", nodeConfig)

	sunodoValidatorEnabled := nodeConfig.CartesiExperimentalSunodoValidatorEnabled()
	if !sunodoValidatorEnabled {
		// add Redis first
		s = append(s, newRedis(nodeConfig))
	}

	// add services without dependencies
	s = append(s, newGraphQLServer(nodeConfig))
	s = append(s, newIndexer(nodeConfig))
	s = append(s, newStateServer(nodeConfig))

	// start either the server manager or host runner
	if nodeConfig.CartesiFeatureHostMode() {
		s = append(s, newHostRunner(nodeConfig))
	} else {
		s = append(s, newServerManager(nodeConfig))
	}

	// enable claimer if reader mode and sunodo validator mode are disabled
	if !nodeConfig.CartesiFeatureDisableClaimer() && !sunodoValidatorEnabled {
		s = append(s, newAuthorityClaimer(nodeConfig))
	}

	// add services with dependencies
	s = append(s, newAdvanceRunner(nodeConfig)) // Depends on the server-manager/host-runner
	s = append(s, newDispatcher(nodeConfig))    // Depends on the state server
	s = append(s, newInspectServer(nodeConfig)) // Depends on the server-manager/host-runner

	s = append(s, newHttpService(nodeConfig))

	ready := make(chan struct{}, 1)
	// logs startup time
	go func() {
		select {
		case <-ready:
			duration := time.Since(startTime)
			slog.Info("rollups-node is ready", "startupTime", duration)
		case <-ctx.Done():
		}
	}()

	// start supervisor
	supervisor := newSupervisorService(s)
	if err := supervisor.Start(ctx, ready); err != nil {
		slog.Error(err.Error())
	}
}
