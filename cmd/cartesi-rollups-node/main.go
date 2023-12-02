// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/services"
)

func main() {
	var s []services.Service

	// Start Redis first
	s = append(s, newRedis())

	// Start services without dependencies
	s = append(s, newGraphQLServer())
	s = append(s, newIndexer())
	s = append(s, newStateServer())

	// Start either the server manager or host runner
	if config.GetFeatureHostMode() {
		s = append(s, newHostRunner())
	} else {
		s = append(s, newServerManager())
	}

	// Enable claimer if reader mode is disabled
	if !config.GetFeatureReaderMode() {
		s = append(s, newAuthorityClaimer())
	}

	// Start services with dependencies
	s = append(s, newAdvanceRunner()) // Depends on the server-manager/host-runner
	s = append(s, newDispatcher())    // Depends on the state server
	s = append(s, newInspectServer()) // Depends on the server-manager/host-runner

	services.Run(context.Background(), s)
}
