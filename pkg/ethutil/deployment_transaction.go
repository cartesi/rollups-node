// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// TransactionRunner controls transaction submission and receipt waiting. A nil
// receipt with no error means broadcast-only: the deployment is not yet confirmed.
type TransactionRunner func(
	context.Context,
	*bind.TransactOpts,
	func(*bind.TransactOpts) (*types.Transaction, error),
) (*types.Receipt, error)

func runDeploymentTransaction(
	ctx context.Context,
	client *ethclient.Client,
	opts *bind.TransactOpts,
	build func(*bind.TransactOpts) (*types.Transaction, error),
	runner TransactionRunner,
) (*types.Receipt, error) {
	var receipt *types.Receipt
	var err error
	if runner != nil {
		receipt, err = runner(ctx, opts, build)
	} else {
		prepared := *opts
		prepared.Context = ctx
		prepared.NoSend = true
		tx, buildErr := build(&prepared)
		if buildErr != nil {
			return nil, fmt.Errorf("failed to prepare deployment transaction: %w", buildErr)
		}
		if err := client.SendTransaction(ctx, tx); err != nil {
			return nil, fmt.Errorf("failed to broadcast deployment transaction %s: %w", tx.Hash(), err)
		}
		receipt, err = bind.WaitMined(ctx, client, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for deployment transaction %s; outcome is unknown: %w", tx.Hash(), err)
		}
	}
	if err != nil {
		return nil, err
	}
	if receipt != nil && receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("deployment transaction %s failed", receipt.TxHash)
	}
	return receipt, nil
}
