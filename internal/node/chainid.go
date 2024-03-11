// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

const defaultTimeout = 3 * time.Second

// Checks if the chain id from the configuration matches the chain id reported
// by the Ethereum node. If they don't, it returns an error.
func validateChainId(ctx context.Context, chainId uint64, ethereumNodeAddr string) error {
	remoteChainId, err := getChainId(ctx, ethereumNodeAddr)
	if err != nil {
		return err
	} else if chainId != remoteChainId {
		return fmt.Errorf(
			"chainId mismatch; expected %v but Ethereum node returned %v",
			chainId,
			remoteChainId,
		)
	}
	return nil
}

func getChainId(ctx context.Context, ethereumNodeAddr string) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	client, err := ethclient.Dial(ethereumNodeAddr)
	if err != nil {
		return 0, fmt.Errorf("create RPC client: %w", err)
	}
	chainId, err := client.ChainID(ctx)
	if err != nil {
		return 0, fmt.Errorf("get chain id: %w", err)
	}
	return chainId.Uint64(), nil
}
