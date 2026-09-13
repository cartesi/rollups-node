// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func pinnedCallOpts(ctx context.Context, block uint64) *bind.CallOpts {
	return &bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(block)}
}
