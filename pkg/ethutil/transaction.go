// Copyright (c) Gabriel de Quadros Ligneul
// SPDX-License-Identifier: MIT (see LICENSE)

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

const PollInterval = 100 * time.Millisecond

// Prepare the transaction, send it, and wait for the receipt.
func sendTransaction(
	ctx context.Context,
	client *ethclient.Client,
	signer Signer,
	txValue *big.Int,
	gasLimit uint64,
	doSend func(txOpts *bind.TransactOpts) (*types.Transaction, error),
) (*types.Receipt, error) {
	txOpts, err := _prepareTransaction(ctx, client, signer, txValue, gasLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare transaction: %v", err)
	}
	tx, err := doSend(txOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to send dapp address: %v", err)
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
	signer Signer,
	txValue *big.Int,
	gasLimit uint64,
) (*bind.TransactOpts, error) {
	nonce, err := client.PendingNonceAt(ctx, signer.Account())
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %v", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %v", err)
	}
	tx, err := signer.MakeTransactor()
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}
	tx.Nonce = big.NewInt(int64(nonce))
	tx.Value = txValue
	tx.GasLimit = gasLimit
	tx.GasPrice = gasPrice
	return tx, nil
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
			return nil, fmt.Errorf("fail to recover transaction: %v", err)
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
		return nil, fmt.Errorf("failed to get receipt: %v", err)
	}
	if receipt.Status == 0 {
		reason, err := _traceTransaction(ctx, client, tx.Hash())
		if err != nil {
			return nil, fmt.Errorf("transaction failed; failed to get reason: %v", err)
		}
		return nil, fmt.Errorf("transaction failed: %v", reason)
	}
	return receipt, err
}

// Call the Ethereum node using the RPC client directly because the ethclient struct doesn't have a
// binding for the trace API. More details in: https://github.com/ethereum/go-ethereum/issues/17341
func _traceTransaction(
	ctx context.Context,
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
