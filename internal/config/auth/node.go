// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package auth

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/pkg/ethutil"
)

// GetNodeTransactOptsFactories resolves both standalone submitting roles before
// any service starts. Split binaries enforce the same boundary at registration.
// Observer-only startup must not call this function.
func GetNodeTransactOptsFactories(
	ctx context.Context, chainID *big.Int,
) (claimer, prt ethutil.TransactOptsFactory, err error) {
	claimer, err = GetTransactOptsFactory(ctx, chainID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve claimer signer: %w", err)
	}
	prt, err = GetPrtTransactOptsFactory(ctx, chainID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve prt signer: %w", err)
	}
	if claimer.From() == prt.From() {
		return nil, nil, fmt.Errorf("claimer and prt require distinct submitting addresses; both resolve to %s", claimer.From())
	}
	return claimer, prt, nil
}
