// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"
)

const (
	defaultReceiptTimeout = 2 * time.Minute
	receiptPollInterval   = 500 * time.Millisecond
)

// AddTransactionFlags installs the receipt policy for one transaction command.
// Commands with dependent steps must also reject incompatible --no-wait use.
func AddTransactionFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("no-wait", false, "Return after broadcast without checking execution success")
	cmd.Flags().Duration("wait-timeout", defaultReceiptTimeout, "Maximum time to wait for a mined receipt")
	previousRun := cmd.PreRunE
	cmd.PreRunE = func(command *cobra.Command, args []string) error {
		_, _, err := transactionPolicy(command)
		if err != nil {
			return err
		}
		if previousRun != nil {
			return previousRun(command, args)
		}
		return nil
	}
	previousHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		if flag := command.Flags().Lookup("gas-limit"); flag != nil {
			flag.Hidden = false
		}
		previousHelp(command, args)
	})
}

func transactionPolicy(cmd *cobra.Command) (bool, time.Duration, error) {
	noWait, err := cmd.Flags().GetBool("no-wait")
	if err != nil {
		return false, 0, err
	}
	timeout, err := cmd.Flags().GetDuration("wait-timeout")
	if err != nil {
		return false, 0, err
	}
	if timeout <= 0 {
		return false, 0, fmt.Errorf("wait-timeout must be positive")
	}
	return noWait, timeout, nil
}

// TransactionClient provides fee suggestions, broadcast, and receipt lookup.
type TransactionClient interface {
	ethereum.GasPricer
	SendTransaction(context.Context, *types.Transaction) error
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
}

// Transact prepares and signs once, broadcasts once, and optionally checks mining.
// A nil receipt with no error means broadcast-only, not confirmed execution.
// After signing, the transaction is returned even if sending or waiting fails.
// GasLimit is preserved: zero estimates gas; a manual limit skips estimation.
func Transact(
	ctx context.Context,
	cmd *cobra.Command,
	client TransactionClient,
	opts *bind.TransactOpts,
	build func(*bind.TransactOpts) (*types.Transaction, error),
) (*types.Transaction, *types.Receipt, error) {
	noWait, timeout, err := transactionPolicy(cmd)
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	preparedOpts, err := prepareTransactionFees(ctx, client, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare transaction fees: %w", err)
	}
	preparedOpts.NoSend = true
	tx, err := build(preparedOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare transaction: %w", err)
	}
	if tx == nil {
		return nil, nil, fmt.Errorf("prepare transaction: no signed transaction")
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Transaction hash: %s\n", tx.Hash().Hex()); err != nil {
		return tx, nil, fmt.Errorf("write transaction hash before broadcast: %w", err)
	}
	// Before broadcast, keep normal signal handling for prompts, stdin, and
	// signers that capture their own context. No transaction has been sent.
	// After this point, cancellation must report the known transaction hash.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := client.SendTransaction(ctx, tx); err != nil {
		return tx, nil, fmt.Errorf("broadcast transaction %s: %w; check its receipt before retrying", tx.Hash().Hex(), err)
	}
	if noWait {
		return tx, nil, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	receipt, err := waitForReceipt(waitCtx, client, tx.Hash())
	return tx, receipt, err
}

func waitForReceipt(ctx context.Context, client TransactionClient, hash common.Hash) (*types.Receipt, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("transaction %s outcome unknown: %w; check its receipt before retrying", hash.Hex(), err)
		}
		receipt, err := client.TransactionReceipt(ctx, hash)
		switch {
		case err == nil && receipt != nil:
			if receipt.TxHash != hash {
				return nil, fmt.Errorf("transaction %s outcome unknown: provider returned a different receipt hash", hash.Hex())
			}
			if receipt.Status != types.ReceiptStatusSuccessful {
				return receipt, fmt.Errorf("transaction %s failed in block %v (receipt status %d)",
					hash.Hex(), receipt.BlockNumber, receipt.Status)
			}
			return receipt, nil
		case err != nil && !errors.Is(err, ethereum.NotFound):
			return nil, fmt.Errorf("transaction %s outcome unknown: %w; check its receipt before retrying", hash.Hex(), err)
		}
		// A missing receipt is normal while the transaction is pending. Do not
		// infer failure or resubmit from mempool visibility on another RPC node.
		timer := time.NewTimer(receiptPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("transaction %s outcome unknown: %w; check its receipt before retrying", hash.Hex(), ctx.Err())
		case <-timer.C:
		}
	}
}

// TransactionResult distinguishes broadcast from successful mined execution.
// A mined result does not assert finality or application input processing.
type TransactionResult struct {
	TransactionHash string `json:"transaction_hash"`
	Status          string `json:"status"`
	BlockNumber     string `json:"block_number,omitempty"`
}

func NewTransactionResult(tx *types.Transaction, receipt *types.Receipt) TransactionResult {
	result := TransactionResult{TransactionHash: tx.Hash().Hex(), Status: "broadcast"}
	if receipt != nil {
		result.Status = "mined"
		if receipt.BlockNumber != nil {
			result.BlockNumber = fmt.Sprintf("0x%x", receipt.BlockNumber)
		}
	}
	return result
}

func WriteTransactionResult(cmd *cobra.Command, tx *types.Transaction, receipt *types.Receipt) error {
	result := NewTransactionResult(tx, receipt)
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		err = encoder.Encode(result)
	} else {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Transaction %s: %s\n", result.Status, result.TransactionHash)
	}
	if err != nil {
		return fmt.Errorf("write transaction result: %w", err)
	}
	return nil
}
