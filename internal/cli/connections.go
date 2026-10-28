// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/ethclient"
)

// DialBlockchain opens the configured endpoint through the protected HTTP client.
// The caller owns the returned client and its cleanup.
func DialBlockchain(ctx context.Context, endpoint config.SafeURL) (*ethclient.Client, error) {
	authorization, err := config.HTTPAuthorizationOption()
	if err != nil {
		return nil, err
	}
	maxRetries, err := config.GetBlockchainHttpMaxRetries()
	if err != nil {
		return nil, err
	}
	minWait, err := config.GetBlockchainHttpRetryMinWait()
	if err != nil {
		return nil, err
	}
	maxWait, err := config.GetBlockchainHttpRetryMaxWait()
	if err != nil {
		return nil, err
	}
	timeout, err := config.GetBlockchainHttpRequestTimeout()
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("CARTESI_BLOCKCHAIN_HTTP_REQUEST_TIMEOUT must be positive")
	}
	//nolint:forbidigo // Protected CLI connection gateway with configured authentication and retries.
	return ethutil.NewEthClient(ctx, endpoint.Raw(), slog.Default(), ethutil.RetryConfig{
		MaxRetries: maxRetries, RetryMinWait: minWait, RetryMaxWait: maxWait, RequestTimeout: timeout,
	}, authorization)
}

// OpenRepository keeps raw DSNs in this gateway. The database constructor
// protects startup diagnostics. The caller owns the repository's cleanup.
func OpenRepository(ctx context.Context, endpoint config.SafeURL) (repository.Repository, error) {
	return factory.NewRepositoryFromConnectionString(ctx, endpoint.Raw()) //nolint:forbidigo // CLI database connection gateway.
}
