// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cartesi/rollups-node/internal/model"
)

// advanceFixtureFinality supplies only the empty successors needed to
// observe already mined transactions. Repeated idle polls never advance time
// in blocks, unlike interval mining, which could finish the fixture's root.
func advanceFixtureFinality(
	ctx context.Context, client *ethclient.Client, policy model.DefaultBlock,
) (uint64, error) {
	return advanceFixtureFinalityWith(ctx, client, policy, anvilMine)
}
