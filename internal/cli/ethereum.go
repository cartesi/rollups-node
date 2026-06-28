// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"errors"
	"math/big"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func GetTransactOpts(ctx context.Context, chainId *big.Int) (*bind.TransactOpts, error) {
	txOpts, err := auth.GetTransactOpts(ctx, chainId)
	if err != nil {
		return nil, err
	}

	gasLimit, err := config.GetBlockchainGasLimit()
	if err != nil && !errors.Is(err, config.ErrNotDefined) {
		return nil, err
	}

	if gasLimit > 0 {
		txOpts.GasLimit = gasLimit
	}
	return txOpts, err
}
