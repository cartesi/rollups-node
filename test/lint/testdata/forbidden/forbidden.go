// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package forbidden

import (
	"context"

	configAlias "github.com/cartesi/rollups-node/internal/config"
	factoryAlias "github.com/cartesi/rollups-node/internal/repository/factory"
	ethAlias "github.com/ethereum/go-ethereum/ethclient"
	rpcAlias "github.com/ethereum/go-ethereum/rpc"
)

type endpointAlias = configAlias.URL

func connect(endpoint endpointAlias) {
	const publicEndpoint = "http://provider.invalid"
	_ = endpoint.Raw()
	_, _ = rpcAlias.Dial(publicEndpoint)
	_, _ = rpcAlias.DialHTTP(publicEndpoint)
	_, _ = rpcAlias.DialOptions(context.Background(), publicEndpoint)
	_, _ = rpcAlias.DialContext(context.Background(), publicEndpoint)
	_, _ = ethAlias.Dial(publicEndpoint)
	_, _ = ethAlias.DialContext(context.Background(), publicEndpoint)
	_, _ = factoryAlias.NewRepositoryFromConnectionString(context.Background(), "postgres://localhost/test")
}
