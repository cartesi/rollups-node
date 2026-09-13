// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deposit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20metadata"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20portal"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"
)

const (
	testApplication = "0x0000000000000000000000000000000000000001"
	testPortal      = "0x0000000000000000000000000000000000000002"
	testToken       = "0x0000000000000000000000000000000000000003"
	testInputBox    = "0x0000000000000000000000000000000000000004"
	testGasLimit    = 90000
	approveFlag     = "--approve"
)

// depositRPC exercises the generated token and portal bindings. The service
// counts receipt reads so a no-wait command cannot pass by waiting on an
// immediately successful receipt.
type depositRPC struct {
	sent             chan *types.Transaction
	nonce            atomic.Uint64
	receipts         atomic.Int32
	estimates        atomic.Int32
	chainReads       atomic.Int32
	failed           bool
	transactions     sync.Map
	approvalLogs     []*types.Log
	depositLogs      []*types.Log
	inputBoxReads    atomic.Int32
	inputBoxBlock    atomic.Value
	inputBoxResponse hexutil.Bytes
}

func (r *depositRPC) ChainId(context.Context) (*hexutil.Big, error) { //nolint:revive // Ethereum RPC method eth_chainId.
	r.chainReads.Add(1)
	return (*hexutil.Big)(big.NewInt(31337)), nil
}

func (*depositRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), GasLimit: testGasLimit}, nil
}

func (*depositRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(1)), nil
}

func (*depositRPC) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	return hexutil.Bytes{0x01}, nil
}

func (r *depositRPC) GetTransactionCount(context.Context, common.Address, string) (hexutil.Uint64, error) {
	return hexutil.Uint64(r.nonce.Load()), nil
}

func (r *depositRPC) EstimateGas(context.Context, map[string]json.RawMessage) (hexutil.Uint64, error) {
	r.estimates.Add(1)
	return testGasLimit, nil
}

func (r *depositRPC) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	r.sent <- &tx
	r.transactions.Store(tx.Hash(), &tx)
	r.nonce.Add(1)
	return tx.Hash(), nil
}

func (r *depositRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	r.receipts.Add(1)
	status := uint64(types.ReceiptStatusSuccessful)
	if r.failed {
		status = types.ReceiptStatusFailed
	}
	logs := r.depositLogs
	if value, ok := r.transactions.Load(hash); ok && *value.(*types.Transaction).To() == common.HexToAddress(testToken) {
		logs = r.approvalLogs
	}
	if logs == nil {
		logs = []*types.Log{}
	}
	return &types.Receipt{TxHash: hash, Status: status, BlockNumber: big.NewInt(10),
		Logs: logs, GasUsed: testGasLimit, CumulativeGasUsed: testGasLimit}, nil
}

func (r *depositRPC) Call(_ context.Context, args map[string]json.RawMessage, block string) (hexutil.Bytes, error) {
	r.inputBoxReads.Add(1)
	r.inputBoxBlock.Store(block)
	var target common.Address
	var data hexutil.Bytes
	if err := json.Unmarshal(args["to"], &target); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(args["input"], &data); err != nil {
		return nil, err
	}
	parsed, err := iapplication.IApplicationMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	if target != common.HexToAddress(testApplication) || !bytes.Equal(data, parsed.Methods["getInputBox"].ID) {
		return nil, fmt.Errorf("unexpected contract call")
	}
	return r.inputBoxResponse, nil
}

func setupDepositRPC(t *testing.T, service *depositRPC) {
	t.Helper()
	erc20Cmd.SetContext(t.Context())
	previousSilenceUsage := Cmd.SilenceUsage
	Cmd.SilenceUsage = true
	t.Cleanup(func() { Cmd.SilenceUsage = previousSilenceUsage })
	rpcServer := rpc.NewServer()
	require.NoError(t, rpcServer.RegisterName("eth", service))
	server := httptest.NewServer(rpcServer)
	t.Cleanup(server.Close)
	t.Cleanup(rpcServer.Stop)
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, server.URL)
	t.Setenv(config.AUTH_KIND, "private_key")
	t.Setenv(config.AUTH_PRIVATE_KEY, hexutil.Encode(crypto.FromECDSA(key)))
	t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "0")
	t.Setenv(config.DATABASE_CONNECTION, "invalid-database")
	signer := crypto.PubkeyToAddress(key.PublicKey)
	service.approvalLogs = []*types.Log{approvalLog(t, signer, common.HexToAddress(testPortal), big.NewInt(100))}
	service.depositLogs = []*types.Log{depositInputLog(t, signer)}
	appABI, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	service.inputBoxResponse, err = appABI.Methods["getInputBox"].Outputs.Pack(common.HexToAddress(testInputBox))
	require.NoError(t, err)
	erc20Cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		require.NoError(t, flag.Value.Set(flag.DefValue))
		flag.Changed = false
	})
}

func approvalLog(t *testing.T, owner, spender common.Address, value *big.Int) *types.Log {
	t.Helper()
	parsed, err := ierc20metadata.IERC20MetadataMetaData.GetAbi()
	require.NoError(t, err)
	event := parsed.Events["Approval"]
	data, err := event.Inputs.NonIndexed().Pack(value)
	require.NoError(t, err)
	return &types.Log{Address: common.HexToAddress(testToken), Data: data,
		Topics: []common.Hash{event.ID, common.BytesToHash(owner.Bytes()), common.BytesToHash(spender.Bytes())}}
}

func depositInputLog(t *testing.T, signer common.Address) *types.Log {
	t.Helper()
	inputABI, err := inputs.InputsMetaData.GetAbi()
	require.NoError(t, err)
	payload := bytes.Join([][]byte{common.HexToAddress(testToken).Bytes(), signer.Bytes(),
		common.LeftPadBytes(big.NewInt(100).Bytes(), common.HashLength)}, nil)
	input, err := inputABI.Pack("EvmAdvance", big.NewInt(31337), common.HexToAddress(testApplication),
		common.HexToAddress(testPortal), big.NewInt(10), big.NewInt(20), big.NewInt(0), big.NewInt(7), payload)
	require.NoError(t, err)
	parsed, err := iinputbox.IInputBoxMetaData.GetAbi()
	require.NoError(t, err)
	event := parsed.Events["InputAdded"]
	data, err := event.Inputs.NonIndexed().Pack(input)
	require.NoError(t, err)
	return &types.Log{Address: common.HexToAddress(testInputBox), Data: data,
		Topics: []common.Hash{event.ID, common.BytesToHash(common.HexToAddress(testApplication).Bytes()), common.BigToHash(big.NewInt(7))}}
}

func TestDepositTransactionPolicy(t *testing.T) {
	for _, tt := range []struct {
		name             string
		extra            []string
		failed           bool
		wantErr          string
		wantTransactions int
		wantReceipts     int32
	}{
		{name: "default waits", wantTransactions: 1, wantReceipts: 1},
		{name: "no wait", extra: []string{"--no-wait"}, wantTransactions: 1},
		{name: "approval then deposit", extra: []string{approveFlag}, wantTransactions: 2, wantReceipts: 2},
		{name: "approval failure stops deposit", extra: []string{approveFlag}, failed: true,
			wantErr: "failed in block", wantTransactions: 1, wantReceipts: 1},
		{name: "deposit failure", failed: true, wantErr: "failed in block", wantTransactions: 1, wantReceipts: 1},
		{name: "reject approval with no wait", extra: []string{approveFlag, "--no-wait"},
			wantErr: "--approve cannot be combined with --no-wait"},
		{name: "reject invalid timeout", extra: []string{"--wait-timeout", "0s"}, wantErr: "wait-timeout must be positive"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &depositRPC{sent: make(chan *types.Transaction, 2), failed: tt.failed}
			setupDepositRPC(t, service)
			var stdout, stderr bytes.Buffer
			Cmd.SetOut(&stdout)
			Cmd.SetErr(&stderr)
			erc20Cmd.SetContext(t.Context())
			args := append([]string{"erc20", testApplication, "--portal", testPortal, "--token", testToken,
				"--amount", "100", "--yes", "--json"}, tt.extra...)
			Cmd.SetArgs(args)
			err := Cmd.ExecuteContext(t.Context())
			if tt.wantErr == "" {
				require.NoError(t, err, stderr.String())
			}
			require.Len(t, service.sent, tt.wantTransactions)
			require.Equal(t, tt.wantReceipts, service.receipts.Load())
			require.EqualValues(t, tt.wantTransactions, service.estimates.Load())
			if tt.wantTransactions == 0 {
				require.Zero(t, service.chainReads.Load())
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.NotContains(t, stdout.String(), `"transaction_hash"`)
				return
			}
			require.NoError(t, err)
			if tt.wantTransactions == 2 {
				approval := <-service.sent
				require.Equal(t, common.HexToAddress(testToken), *approval.To())
				tokenABI, err := ierc20metadata.IERC20MetadataMetaData.GetAbi()
				require.NoError(t, err)
				require.Equal(t, tokenABI.Methods["approve"].ID, approval.Data()[:4])
			}
			deposit := <-service.sent
			require.Equal(t, common.HexToAddress(testPortal), *deposit.To())
			portalABI, err := ierc20portal.IErc20PortalMetaData.GetAbi()
			require.NoError(t, err)
			require.Equal(t, portalABI.Methods["depositErc20Tokens"].ID, deposit.Data()[:4])
			var result map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			require.Equal(t, deposit.Hash().Hex(), result["transaction_hash"])
			if tt.wantReceipts == 0 {
				require.Zero(t, service.inputBoxReads.Load())
				require.Equal(t, "broadcast", result["status"])
				require.NotContains(t, result, "block_number")
			} else {
				require.EqualValues(t, 1, service.inputBoxReads.Load())
				require.Equal(t, "0xa", service.inputBoxBlock.Load())
				require.Equal(t, "mined", result["status"])
				require.Equal(t, "0xa", result["block_number"])
			}
		})
	}
}
