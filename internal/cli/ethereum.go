// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"errors"
	"math/big"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func GetTransactOpts(ctx context.Context, chainId *big.Int) (*bind.TransactOpts, error) {
	factory, err := auth.GetTransactOptsFactory(ctx, chainId)
	if err != nil {
		return nil, err
	}
	return GetTransactOptsFromFactory(ctx, factory)
}

func GetTransactOptsFromFactory(
	ctx context.Context,
	factory ethutil.TransactOptsFactory,
) (*bind.TransactOpts, error) {
	txOpts, err := factory.NewTransactOpts(ctx)
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
	return txOpts, nil
}
