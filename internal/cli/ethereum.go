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
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func GetTransactOpts(ctx context.Context, chainID *big.Int) (*bind.TransactOpts, error) {
	factory, err := auth.GetTransactOptsFactory(ctx, chainID)
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

// prepareTransactionFees applies the CLI fee policy immediately before signing.
// Dependent transactions, such as approval followed by deposit, each obtain a
// fresh price. A false legacy setting preserves the binding's automatic choice.
func prepareTransactionFees(
	ctx context.Context,
	client ethereum.GasPricer,
	opts *bind.TransactOpts,
) (*bind.TransactOpts, error) {
	legacy, err := config.GetBlockchainLegacyEnabled()
	if err != nil && !errors.Is(err, config.ErrNotDefined) {
		return nil, err
	}
	factory := ethutil.NewStaticTransactOptsFactory(opts)
	if legacy {
		factory = ethutil.WithLegacyFees(factory, client)
	}
	return factory.NewTransactOpts(ctx)
}
