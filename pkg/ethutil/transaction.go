// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type TransactOptsFactory interface {
	From() common.Address
	NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error)
}

type staticTransactOptsFactory struct {
	opts *bind.TransactOpts
}

func (f *staticTransactOptsFactory) From() common.Address {
	return f.opts.From
}

func (f *staticTransactOptsFactory) NewTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts := *f.opts
	opts.Context = ctx
	return &opts, nil
}

func NewStaticTransactOptsFactory(txOpts *bind.TransactOpts) TransactOptsFactory {
	return &staticTransactOptsFactory{opts: txOpts}
}

const PollInterval = 500 * time.Millisecond

// Prepare the transaction, send it, and wait for the receipt.
func sendTransaction(
	ctx context.Context,
	client *ethclient.Client,
	txOptsFactory TransactOptsFactory,
	txValue *big.Int,
	doSend func(txOpts *bind.TransactOpts) (*types.Transaction, error),
) (*types.Receipt, error) {
	txOpts, err := _prepareTransaction(ctx, client, txOptsFactory, txValue)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare transaction: %w", err)
	}
	tx, err := doSend(txOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}
	receipt, err := _waitForTransaction(ctx, client, tx)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

// Prepare the blockchain transaction.
func _prepareTransaction(
	ctx context.Context,
	client *ethclient.Client,
	txOptsFactory TransactOptsFactory,
	txValue *big.Int,
) (*bind.TransactOpts, error) {
	nonce, err := client.PendingNonceAt(ctx, txOptsFactory.From())
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}
	nonceBigInt := &big.Int{}
	nonceBigInt.SetUint64(nonce)

	txOpts, err := txOptsFactory.NewTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction options: %w", err)
	}
	txOpts.Nonce = nonceBigInt
	txOpts.Value = txValue
	txOpts.GasPrice = gasPrice
	return txOpts, nil
}

// Wait for transaction to be included in a block. Return the transaction receipt.
func _waitForTransaction(
	ctx context.Context,
	client *ethclient.Client,
	tx *types.Transaction,
) (*types.Receipt, error) {
	for {
		_, isPending, err := client.TransactionByHash(ctx, tx.Hash())
		if err != nil {
			return nil, fmt.Errorf("fail to recover transaction: %w", err)
		}
		if !isPending {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(PollInterval):
			continue
		}
	}
	receipt, err := client.TransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get receipt: %w", err)
	}
	if receipt.Status == types.ReceiptStatusFailed {
		reason, err := _traceTransaction(client, tx.Hash())
		if err != nil {
			return nil, fmt.Errorf("transaction failed; failed to get reason: %w", err)
		}
		return nil, fmt.Errorf("transaction failed: %v", reason)
	}
	return receipt, err
}

// Call the Ethereum node using the RPC client directly because the ethclient struct doesn't have a
// binding for the trace API. More details in: https://github.com/ethereum/go-ethereum/issues/17341
func _traceTransaction(
	client *ethclient.Client,
	hash common.Hash,
) (string, error) {
	var result json.RawMessage
	err := client.Client().Call(&result, "debug_traceTransaction", hash)
	if err != nil {
		return "", err
	}
	return string(result), nil
}
