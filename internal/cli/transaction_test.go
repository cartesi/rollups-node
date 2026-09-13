// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type transactionRPC struct {
	send     func(context.Context, *types.Transaction) error
	receipt  func(context.Context, common.Hash) (*types.Receipt, error)
	sends    int
	receipts int
}

func (*transactionRPC) SuggestGasPrice(context.Context) (*big.Int, error) {
	return nil, errors.New("unexpected gas price request")
}

func (r *transactionRPC) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	r.sends++
	return r.send(ctx, tx)
}

func (r *transactionRPC) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	r.receipts++
	return r.receipt(ctx, hash)
}

func transactionCommand() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "test-transaction"}
	AddTransactionFlags(cmd)
	cmd.Flags().Bool("json", false, "JSON output")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd, stdout, stderr
}

func transactionFixture() *types.Transaction {
	return types.NewTx(&types.LegacyTx{Gas: 21000, GasPrice: big.NewInt(1)})
}

func TestTransactPolicy(t *testing.T) {
	t.Parallel()
	for _, noWait := range []bool{false, true} {
		name := "wait"
		if noWait {
			name = "no_wait"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd, stdout, stderr := transactionCommand()
			if noWait {
				require.NoError(t, cmd.Flags().Set("no-wait", "true"))
			}
			tx := transactionFixture()
			opts := &bind.TransactOpts{GasLimit: 123456}
			var built bool
			client := &transactionRPC{
				send: func(_ context.Context, sent *types.Transaction) error {
					require.True(t, built)
					require.Same(t, tx, sent)
					require.Contains(t, stderr.String(), tx.Hash().Hex(), "hash must be available before broadcast")
					return nil
				},
				receipt: func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
					require.False(t, noWait, "no-wait must not query receipts")
					require.Equal(t, tx.Hash(), hash)
					return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
				},
			}
			actual, receipt, err := Transact(t.Context(), cmd, client, opts,
				func(prepared *bind.TransactOpts) (*types.Transaction, error) {
					require.NotSame(t, opts, prepared)
					require.True(t, prepared.NoSend)
					require.Equal(t, opts.GasLimit, prepared.GasLimit)
					require.NoError(t, prepared.Context.Err())
					built = true
					return tx, nil
				})
			require.NoError(t, err)
			require.Same(t, tx, actual)
			require.False(t, opts.NoSend, "do not mutate caller options")
			require.Equal(t, 1, client.sends)
			require.Empty(t, stdout.String(), "the command owns its final result")
			if noWait {
				require.Nil(t, receipt)
				require.Zero(t, client.receipts)
			} else {
				require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
				require.Equal(t, 1, client.receipts)
			}
		})
	}
}

func TestTransactErrorsKeepHash(t *testing.T) {
	t.Parallel()
	rpcError := errors.New("RPC response lost")
	tx := transactionFixture()
	for _, test := range []struct {
		name      string
		sendError error
		receipt   *types.Receipt
		readError error
		want      string
		cause     error
	}{
		{name: "broadcast", sendError: rpcError, want: "check its receipt before retrying", cause: rpcError},
		{name: "read", readError: rpcError, want: "outcome unknown", cause: rpcError},
		{name: "manual_gas_mined_failure", receipt: &types.Receipt{
			TxHash: tx.Hash(), Status: types.ReceiptStatusFailed, BlockNumber: big.NewInt(9)}, want: "failed in block 9"},
		{name: "wrong_receipt", receipt: &types.Receipt{TxHash: common.HexToHash("0x99")}, want: "different receipt hash"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cmd, _, stderr := transactionCommand()
			client := &transactionRPC{
				send:    func(context.Context, *types.Transaction) error { return test.sendError },
				receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return test.receipt, test.readError },
			}
			actual, _, err := Transact(t.Context(), cmd, client, &bind.TransactOpts{GasLimit: tx.Gas()},
				func(*bind.TransactOpts) (*types.Transaction, error) { return tx, nil })
			require.ErrorContains(t, err, test.want)
			require.ErrorContains(t, err, tx.Hash().Hex())
			if test.cause != nil {
				require.ErrorIs(t, err, test.cause)
			}
			require.Same(t, tx, actual)
			require.Contains(t, stderr.String(), tx.Hash().Hex())
			require.Equal(t, 1, client.sends, "never resubmit on send or receipt errors")
		})
	}
}

func TestTransactPendingTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	for _, cancelWait := range []bool{false, true} {
		name := "timeout"
		if cancelWait {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd, _, _ := transactionCommand()
			require.NoError(t, cmd.Flags().Set("wait-timeout", "10ms"))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			client := &transactionRPC{
				send: func(context.Context, *types.Transaction) error { return nil },
				receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
					if cancelWait {
						cancel()
					}
					return nil, ethereum.NotFound
				},
			}
			tx := transactionFixture()
			actual, receipt, err := Transact(ctx, cmd, client, &bind.TransactOpts{},
				func(*bind.TransactOpts) (*types.Transaction, error) { return tx, nil })
			require.ErrorContains(t, err, "outcome unknown")
			require.ErrorContains(t, err, tx.Hash().Hex())
			if cancelWait {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			require.Same(t, tx, actual)
			require.Nil(t, receipt)
			require.Equal(t, 1, client.sends)
			require.Equal(t, 1, client.receipts)
		})
	}
}

func TestTransactPreparationStopsBeforeBroadcast(t *testing.T) {
	t.Parallel()
	cmd, stdout, stderr := transactionCommand()
	estimateError := errors.New("execution reverted during estimation")
	client := &transactionRPC{}
	tx, receipt, err := Transact(t.Context(), cmd, client, &bind.TransactOpts{},
		func(opts *bind.TransactOpts) (*types.Transaction, error) {
			require.Zero(t, opts.GasLimit, "preserve the default estimation policy")
			return nil, estimateError
		})
	require.ErrorIs(t, err, estimateError)
	require.Nil(t, tx)
	require.Nil(t, receipt)
	require.Zero(t, client.sends)
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}

func TestTransactionFlagValidation(t *testing.T) {
	t.Parallel()
	for _, timeout := range []string{"0s", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			t.Parallel()
			cmd, _, _ := transactionCommand()
			cmd.RunE = func(*cobra.Command, []string) error { t.Fatal("must reject before command runs"); return nil }
			cmd.SetArgs([]string{"--wait-timeout", timeout})
			require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "wait-timeout must be positive")
		})
	}
	t.Run("preserve_command_validation", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{Use: "test", PreRunE: func(*cobra.Command, []string) error { return io.ErrUnexpectedEOF }}
		AddTransactionFlags(cmd)
		require.ErrorIs(t, cmd.PreRunE(cmd, nil), io.ErrUnexpectedEOF)
	})
}

type failingTransactionWriter struct{}

func (failingTransactionWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWriteTransactionResult(t *testing.T) {
	t.Parallel()
	for _, asJSON := range []bool{false, true} {
		name := "text"
		if asJSON {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd, stdout, stderr := transactionCommand()
			if asJSON {
				require.NoError(t, cmd.Flags().Set("json", "true"))
			}
			tx := transactionFixture()
			require.NoError(t, WriteTransactionResult(cmd, tx, nil))
			require.Contains(t, stdout.String(), tx.Hash().Hex())
			require.Contains(t, stdout.String(), "broadcast")
			require.Empty(t, stderr.String())
			if asJSON {
				var result map[string]string
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
				require.NotContains(t, result, "block_number")
			}
			cmd.SetOut(failingTransactionWriter{})
			require.ErrorIs(t, WriteTransactionResult(cmd, tx, nil), io.ErrClosedPipe)
		})
	}
	result := NewTransactionResult(transactionFixture(), &types.Receipt{BlockNumber: big.NewInt(16)})
	require.Equal(t, "mined", result.Status)
	require.Equal(t, "0x10", result.BlockNumber)
}
