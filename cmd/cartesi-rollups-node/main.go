// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/services"
)

func main() {
	startTime := time.Now()
	var s []services.Service

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// add Redis first
	s = append(s, newRedis())

	// add services without dependencies
	s = append(s, newGraphQLServer())
	s = append(s, newIndexer())
	s = append(s, newStateServer())

	// start either the server manager or host runner
	if config.GetCartesiFeatureHostMode() {
		s = append(s, newHostRunner())
	} else {
		s = append(s, newServerManager())
	}

	// enable claimer if reader mode is disabled
	if !config.GetCartesiFeatureReaderMode() {
		s = append(s, newAuthorityClaimer())
	}

	// add services with dependencies
	s = append(s, newAdvanceRunner()) // Depends on the server-manager/host-runner
	s = append(s, newDispatcher())    // Depends on the state server
	s = append(s, newInspectServer()) // Depends on the server-manager/host-runner

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
	supervisor := newSupervisorService(s)
	if err := supervisor.Start(ctx, ready); err != nil {
		config.ErrorLogger.Print(err)
	}
}
