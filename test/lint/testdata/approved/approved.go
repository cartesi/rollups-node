// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package approved

import (
	"context"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

func connect(endpoint config.SafeURL) {
	// These ordinary uses must pass without suppressions.
	_ = endpoint.String()
	_, _ = ethutil.DialEthClient(context.Background(), "http://localhost:8545")
	_, _ = cli.DialBlockchain(context.Background(), endpoint)
	_, _ = cli.OpenRepository(context.Background(), endpoint)
}
