// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/ethclient"
)

// DialBlockchain opens the configured endpoint through the protected HTTP client.
// The caller owns the returned client and its cleanup.
func DialBlockchain(ctx context.Context, endpoint config.SafeURL) (*ethclient.Client, error) {
	return ethutil.DialEthClient(ctx, endpoint.Raw()) //nolint:forbidigo // Protected CLI connection gateway.
}

// OpenRepository keeps raw DSNs in this gateway. The database constructor
// protects startup diagnostics. The caller owns the repository's cleanup.
func OpenRepository(ctx context.Context, endpoint config.SafeURL) (repository.Repository, error) {
	return factory.NewRepositoryFromConnectionString(ctx, endpoint.Raw()) //nolint:forbidigo // CLI database connection gateway.
}
