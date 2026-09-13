// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

const transactionSignalTestChild = "CARTESI_CLI_TRANSACTION_SIGNAL_TEST_CHILD"

func TestTransactHashWriteFailureStopsBroadcast(t *testing.T) {
	t.Parallel()
	cmd, stdout, _ := transactionCommand()
	cmd.SetErr(failingTransactionWriter{})
	client := &transactionRPC{}
	tx := transactionFixture()
	actual, receipt, err := Transact(t.Context(), cmd, client, &bind.TransactOpts{},
		func(*bind.TransactOpts) (*types.Transaction, error) { return tx, nil })
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.ErrorContains(t, err, "write transaction hash before broadcast")
	require.Same(t, tx, actual)
	require.Nil(t, receipt)
	require.Zero(t, client.sends)
	require.Zero(t, client.receipts)
	require.Empty(t, stdout.String())
}

// Signals must reach only the subprocess. Other tests can have active
// transaction contexts in the parent process at the same time.
func TestTransactSignalCancellation(t *testing.T) {
	if os.Getenv(transactionSignalTestChild) == "1" {
		runTransactionSignalChild(t)
		return
	}

	executable, err := os.Executable()
	require.NoError(t, err)
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			const childTimeout = 10 * time.Second
			ctx, cancel := context.WithTimeout(t.Context(), childTimeout)
			defer cancel()
			child := exec.CommandContext(ctx, executable, "-test.run=^TestTransactSignalCancellation$")
			child.Env = append(os.Environ(), transactionSignalTestChild+"=1")
			var stderr bytes.Buffer
			child.Stderr = &stderr
			stdout, err := child.StdoutPipe()
			require.NoError(t, err)
			require.NoError(t, child.Start())
			t.Cleanup(func() {
				cancel()
				if child.ProcessState == nil {
					_ = child.Wait()
				}
			})

			// The child acknowledges the receipt request after Transact installs
			// its signal handler. No sleep or process-start timing is required.
			scanner := bufio.NewScanner(stdout)
			require.True(t, scanner.Scan(), "child exited before requesting a receipt")
			require.Equal(t, "waiting-for-receipt", scanner.Text())
			require.NoError(t, child.Process.Signal(signal))
			require.NoError(t, child.Wait(), stderr.String())
			require.Contains(t, stderr.String(), transactionFixture().Hash().Hex())
			require.Contains(t, stderr.String(), "outcome unknown")
			require.Contains(t, stderr.String(), "context canceled")
		})
	}
}

func runTransactionSignalChild(t *testing.T) {
	t.Helper()
	cmd, _, _ := transactionCommand()
	cmd.SetErr(os.Stderr)
	client := &transactionRPC{
		send: func(context.Context, *types.Transaction) error { return nil },
		receipt: func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
			_, err := fmt.Fprintln(os.Stdout, "waiting-for-receipt")
			require.NoError(t, err)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	tx := transactionFixture()
	actual, receipt, err := Transact(t.Context(), cmd, client, &bind.TransactOpts{},
		func(*bind.TransactOpts) (*types.Transaction, error) { return tx, nil })
	require.ErrorIs(t, err, context.Canceled)
	require.Same(t, tx, actual)
	require.Nil(t, receipt)
	require.Equal(t, 1, client.sends)
	require.Equal(t, 1, client.receipts)
	_, writeErr := fmt.Fprintln(os.Stderr, err)
	require.NoError(t, writeErr)
}
