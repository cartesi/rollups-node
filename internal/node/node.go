// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"

	"github.com/cartesi/rollups-node/internal/node/config"
	"github.com/cartesi/rollups-node/internal/services"
)

// Setup creates the Node top-level supervisor.
func Setup(ctx context.Context, c config.NodeConfig, workDir string) (services.Service, error) {
	// checks
	err := validateChainId(ctx, c.BlockchainID, c.BlockchainHttpEndpoint.Value)
	if err != nil {
		return nil, err
	}

	if c.FeatureMachineHashCheckEnabled {
		if err := validateMachineHash(
			ctx,
			c.SnapshotDir,
			c.ContractsApplicationAddress,
			c.BlockchainHttpEndpoint.Value,
		); err != nil {
			return nil, err
		}
	}

	// create service
	return newSupervisorService(c, workDir), nil
}
