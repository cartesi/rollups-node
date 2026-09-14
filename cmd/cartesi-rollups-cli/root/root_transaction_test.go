// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

const (
	rootCommandProcess = "CARTESI_TEST_ROOT_COMMAND_PROCESS"
	rootTestChainID    = 31337
	rootTestGasLimit   = 500000
	rootTestPrivateKey = "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
)

// Run each invocation in a fresh process. The real command tree has global
// Cobra flags and Viper bindings. Do not reset or replace either in these tests.
func TestRootCommandProcess(t *testing.T) {
	if os.Getenv(rootCommandProcess) != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			Cmd.SetArgs(os.Args[i+1:])
			if err := Cmd.ExecuteContext(t.Context()); err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	t.Fatal("missing subprocess argument separator")
}

func runRootCommandProcess(t *testing.T, extraEnv []string, args ...string) (string, string, error) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	const processTimeout = 20 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), processTimeout)
	defer cancel()
	processArgs := append([]string{"-test.run=^TestRootCommandProcess$", "--"}, args...)
	process := exec.CommandContext(ctx, executable, processArgs...)
	// Each covered child must have its own output directory. Concurrent children
	// can otherwise collide when they write coverage metadata at process exit.
	process.Env = append(os.Environ(), rootCommandProcess+"=1", "GOCOVERDIR="+t.TempDir())
	process.Env = append(process.Env, extraEnv...)
	var stdout, stderr bytes.Buffer
	process.Stdout, process.Stderr = &stdout, &stderr
	err = process.Run()
	return stdout.String(), stderr.String(), err
}

func TestRootTransactionHelp(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"send", "execute", "deposit erc20", "foreclose", "prove-drive-root", "withdraw", "refund",
		"deploy application", "deploy authority", "deploy quorum",
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			args := append(strings.Fields(command), "--help")
			stdout, stderr, err := runRootCommandProcess(t, nil, args...)
			require.NoError(t, err, stderr)
			require.Empty(t, stderr)
			require.Contains(t, stdout, "Usage:")
			help := strings.Join(strings.Fields(stdout), " ")
			require.Contains(t, help, "--gas-limit")
			require.Contains(t, help, "Zero estimates gas; a nonzero value skips estimation.")
			require.Contains(t, help, "--no-wait")
			require.Contains(t, help, "Return after broadcast without checking execution success")
			require.Contains(t, help, "--wait-timeout")
			require.NotContains(t, help, "--async")
		})
	}
}

func TestRootHelpRemainsAvailable(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runRootCommandProcess(t, nil, "--help")
	require.NoError(t, err, stderr)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Usage:")
	require.Contains(t, stdout, "Available Commands:")
}

func TestRootTransactionErrorsKeepHashWithoutUsage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		sendErr error
		want    string
	}{
		{name: "broadcast", sendErr: errors.New("broadcast response lost"), want: "broadcast response lost"},
		{name: "mined failure", want: "receipt status 0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			backend := &rootTransactionRPC{sendErr: test.sendErr, sent: make(chan *types.Transaction, 1)}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", backend))
			t.Cleanup(server.Stop)
			httpServer := httptest.NewServer(server)
			t.Cleanup(httpServer.Close)
			inputFile := filepath.Join(t.TempDir(), "input.hex")
			require.NoError(t, os.WriteFile(inputFile, []byte("0xaabb\n"), 0o600))
			application := common.HexToAddress("0x01")
			stdout, stderr, err := runRootCommandProcess(t, []string{
				config.AUTH_KIND + "=private_key",
				config.AUTH_PRIVATE_KEY + "=" + rootTestPrivateKey,
			}, "refund", application.Hex(), "7", "--input-file", inputFile,
				"--blockchain-http-endpoint", httpServer.URL, "--gas-limit", "500000", "--yes", "--json")
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, 1, exitErr.ExitCode(), stderr)
			require.Empty(t, stdout, "a failed transaction must not produce a success result")
			require.Len(t, backend.sent, 1, stderr)
			tx := <-backend.sent
			require.Equal(t, &application, tx.To())
			require.EqualValues(t, rootTestGasLimit, tx.Gas())
			_, err = types.Sender(types.LatestSignerForChainID(big.NewInt(rootTestChainID)), tx)
			require.NoError(t, err, "the provider must receive a signed transaction")
			appABI, err := iapplication.IApplicationMetaData.GetAbi()
			require.NoError(t, err)
			calldata, err := appABI.Pack("issueRefund", big.NewInt(7), []byte{0xaa, 0xbb})
			require.NoError(t, err)
			require.Equal(t, calldata, tx.Data())
			require.Contains(t, stderr, "Transaction hash: "+tx.Hash().Hex())
			require.Contains(t, stderr, "Error: refund:")
			require.Contains(t, stderr, test.want)
			require.GreaterOrEqual(t, strings.Count(stderr, tx.Hash().Hex()), 2,
				"the returned error must retain the hash in addition to the pre-broadcast diagnostic")
			require.NotContains(t, stderr, "Usage:", "runtime errors must not dump command help")
		})
	}
}

// This local RPC server lets the actual refund binding sign and submit its
// calldata. It reports either a lost broadcast response or a failed receipt.
// No request reaches a blockchain or a database.
type rootTransactionRPC struct {
	sendErr error
	sent    chan *types.Transaction
}

func (*rootTransactionRPC) ChainId(context.Context) hexutil.Uint64 { //nolint:revive // RPC method eth_chainId.
	return rootTestChainID
}

func (*rootTransactionRPC) GetBlockByNumber(context.Context, string, bool) *types.Header {
	return &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), GasLimit: rootTestGasLimit}
}

func (*rootTransactionRPC) GasPrice(context.Context) hexutil.Uint64 { return 1 }

func (*rootTransactionRPC) GetTransactionCount(context.Context, common.Address, string) hexutil.Uint64 {
	return 0
}

func (r *rootTransactionRPC) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	select {
	case r.sent <- tx:
	default:
		return tx.Hash(), errors.New("unexpected repeated broadcast")
	}
	return tx.Hash(), r.sendErr
}

func (*rootTransactionRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) *types.Receipt {
	return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusFailed, BlockNumber: big.NewInt(1), Logs: []*types.Log{}}
}
