// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

type legacyFeeFactory struct {
	TransactOptsFactory
	client ethereum.GasPricer
}

// WithLegacyFees forces legacy transactions with a fresh suggested gas price
// for each transaction. Without this wrapper, the binding selects the fee
// format from the caller's options and the network's latest block.
// Gas estimation, signing, nonce selection, value, and NoSend are unchanged.
func WithLegacyFees(factory TransactOptsFactory, client ethereum.GasPricer) TransactOptsFactory {
	return &legacyFeeFactory{TransactOptsFactory: factory, client: client}
}

func (f *legacyFeeFactory) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := f.TransactOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return nil, err
	}
	price, err := f.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("suggest legacy gas price: %w", err)
	}
	// Zero is valid for a zero-fee network; a missing or negative price is not.
	if price == nil || price.Sign() < 0 {
		return nil, fmt.Errorf("suggest legacy gas price: invalid price %v", price)
	}
	prepared := *opts
	prepared.GasPrice = price
	prepared.GasFeeCap = nil
	prepared.GasTipCap = nil
	return &prepared, nil
}
