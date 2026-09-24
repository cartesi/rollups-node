// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/send"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/test/tooling/db"
)

const (
	sendTestGas  = 90000
	jsonFlag     = "--json"
	noWaitFlag   = "--no-wait"
	gasLimitFlag = "--gas-limit"
)

func TestSendUsesApplicationInputBox(t *testing.T) {
	t.Run("address without database", func(t *testing.T) { runSendInputBoxTest(t, false) })
	t.Run("name from database", func(t *testing.T) { runSendInputBoxTest(t, true) })
}

func runSendInputBoxTest(t *testing.T, byName bool) {
	t.Helper()
	inputBox := common.HexToAddress("0xb2")
	app := &model.Application{
		Name: "send-inputbox", IApplicationAddress: common.HexToAddress("0xc3"),
		IConsensusAddress: common.HexToAddress("0xd4"), IInputBoxAddress: common.HexToAddress("0xf6"),
		EpochLength: 10, ConsensusType: model.Consensus_Authority, Status: model.ApplicationStatus_OK,
	}
	appReference := app.IApplicationAddress.Hex()
	t.Setenv(config.DATABASE_CONNECTION, "invalid")
	if byName {
		endpoint, err := db.GetTestDatabaseEndpoint()
		if err != nil {
			t.Skipf("Skipping: %v", err)
		}
		release, err := db.LockTestPostgres(t.Context(), endpoint)
		require.NoError(t, err)
		t.Cleanup(release)
		require.NoError(t, db.SetupTestPostgres(endpoint))
		repo, err := factory.NewRepositoryFromConnectionString(t.Context(), endpoint)
		require.NoError(t, err)
		t.Cleanup(repo.Close)
		_, err = repo.CreateApplication(t.Context(), app, false)
		require.NoError(t, err)
		t.Setenv(config.DATABASE_CONNECTION, endpoint)
		appReference = app.Name
	}
	globalBox := common.HexToAddress("0xa1")
	t.Setenv(config.CONTRACTS_INPUT_BOX_ADDRESS, globalBox.Hex())
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	t.Setenv(config.AUTH_KIND, "private_key")
	t.Setenv(config.AUTH_PRIVATE_KEY, hexutil.Encode(crypto.FromECDSA(key)))
	t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "0")
	applicationABI, err := iapplication.IApplicationMetaData.GetAbi()
	require.NoError(t, err)
	getter := applicationABI.Methods["getInputBox"]

	parsed, err := iinputbox.IInputBoxMetaData.GetAbi()
	require.NoError(t, err)
	event := parsed.Events["InputAdded"]
	data, err := event.Inputs.NonIndexed().Pack([]byte("encoded input"))
	require.NoError(t, err)
	inputLog := func(box, application common.Address, index int64) *types.Log {
		return &types.Log{Address: box, Data: data, Topics: []common.Hash{
			event.ID, common.BytesToHash(application.Bytes()), common.BigToHash(big.NewInt(index)),
		}}
	}
	for _, test := range []struct {
		name         string
		flags        []string
		missingEvent bool
		noWait       bool
		json         bool
		zeroInputBox bool
		manualGas    bool
	}{
		{name: "decimal text"},
		{name: "hex JSON", flags: []string{jsonFlag}, json: true},
		{name: "broadcast only", flags: []string{jsonFlag, noWaitFlag}, json: true, noWait: true},
		{name: "reject unrelated events", missingEvent: true},
		{name: "manual gas", flags: []string{gasLimitFlag, "123456"}, manualGas: true},
		{name: "manual gas broadcast only", flags: []string{jsonFlag, noWaitFlag, gasLimitFlag, "123456"},
			json: true, noWait: true, manualGas: true},
		{name: "reject zero InputBox", zeroInputBox: true},
		{name: "reject zero InputBox with manual gas", flags: []string{gasLimitFlag, "123456"}, zeroInputBox: true, manualGas: true},
		{name: "reject zero InputBox without waiting", flags: []string{noWaitFlag}, zeroInputBox: true, noWait: true},
		{name: "reject zero InputBox with manual gas without waiting", flags: []string{noWaitFlag, gasLimitFlag, "123456"},
			zeroInputBox: true, noWait: true, manualGas: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := &sendRPC{
				sent: make(chan *types.Transaction, 2), application: app.IApplicationAddress,
				inputBox: inputBox, getterID: getter.ID,
				logs: []*types.Log{
					inputLog(globalBox, app.IApplicationAddress, 1),
					inputLog(inputBox, common.HexToAddress("0xe5"), 2),
				},
			}
			if !test.missingEvent {
				backend.logs = append(backend.logs, inputLog(inputBox, app.IApplicationAddress, 17))
			}
			if test.zeroInputBox {
				backend.inputBox = common.Address{}
				// Reject the destination before reading signer configuration.
				t.Setenv(config.AUTH_KIND, "invalid-signer")
			}
			rpcServer := rpc.NewServer()
			require.NoError(t, rpcServer.RegisterName("eth", backend))
			server := httptest.NewServer(rpcServer)
			t.Cleanup(server.Close)
			t.Cleanup(rpcServer.Stop)
			t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, server.URL)
			resetSendCommand(t)
			var stdout, stderr bytes.Buffer
			root.Cmd.SetOut(&stdout)
			root.Cmd.SetErr(&stderr)
			root.Cmd.SetArgs(append([]string{"send", appReference, "hello", "--yes"}, test.flags...))
			err := root.Cmd.ExecuteContext(t.Context())
			if test.zeroInputBox {
				require.ErrorContains(t, err, "zero InputBox address")
				require.EqualValues(t, 1, backend.getterCalls.Load())
				require.Zero(t, backend.chainReads.Load())
				require.Zero(t, backend.estimates.Load())
				require.Zero(t, backend.receipts.Load())
				require.Empty(t, backend.sent)
				require.Empty(t, stdout.String())
				return
			}
			require.Len(t, backend.sent, 1, stderr.String())
			tx := <-backend.sent
			require.Equal(t, inputBox, *tx.To(), "the contract selects the InputBox, not global or cached configuration")
			require.EqualValues(t, 1, backend.getterCalls.Load())
			method := parsed.Methods["addInput"]
			require.Equal(t, method.ID, tx.Data()[:4])
			args, decodeErr := method.Inputs.Unpack(tx.Data()[4:])
			require.NoError(t, decodeErr)
			require.Equal(t, app.IApplicationAddress, args[0])
			require.Equal(t, []byte("hello"), args[1])
			if test.manualGas {
				require.Zero(t, backend.estimates.Load())
				require.EqualValues(t, 123456, tx.Gas())
			} else {
				require.EqualValues(t, 1, backend.estimates.Load())
			}
			if test.noWait {
				require.Zero(t, backend.receipts.Load())
			} else {
				require.EqualValues(t, 1, backend.receipts.Load())
			}
			if test.missingEvent {
				require.ErrorContains(t, err, "no matching InputAdded event")
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.Empty(t, stdout.String())
				return
			}
			require.NoError(t, err, stderr.String())
			if !test.json {
				require.Contains(t, stdout.String(), "Index: 17 BlockNumber: 16")
				require.Contains(t, stdout.String(), tx.Hash().Hex())
				return
			}
			var result map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			require.Equal(t, tx.Hash().Hex(), result["transaction_hash"])
			if test.noWait {
				require.Equal(t, "broadcast", result["status"])
				require.NotContains(t, result, "input_index")
				require.NotContains(t, result, "block_number")
			} else {
				require.Equal(t, "mined", result["status"])
				require.Equal(t, "0x11", result["input_index"])
				require.Equal(t, "0x10", result["block_number"])
			}
		})
	}
}

func TestSendRejectsInputBoxOverride(t *testing.T) {
	resetSendCommand(t)
	// An unusable DSN proves the flag fails before database or chain work.
	t.Setenv(config.DATABASE_CONNECTION, "invalid")
	var stdout, stderr bytes.Buffer
	root.Cmd.SetOut(&stdout)
	root.Cmd.SetErr(&stderr)
	root.Cmd.SetArgs([]string{"send", "example", "hello", "--inputbox", common.HexToAddress("0xa1").Hex()})
	require.ErrorContains(t, root.Cmd.ExecuteContext(t.Context()), "--inputbox is not supported by send")
	require.Empty(t, stdout.String())
}

func resetSendCommand(t *testing.T) {
	t.Helper()
	for _, flags := range []*pflag.FlagSet{root.Cmd.PersistentFlags(), send.Cmd.Flags()} {
		flags.VisitAll(func(flag *pflag.Flag) {
			require.NoError(t, flag.Value.Set(flag.DefValue))
			flag.Changed = false
		})
	}
	send.Cmd.SetContext(t.Context())
	t.Cleanup(func() {
		root.Cmd.SetOut(nil)
		root.Cmd.SetErr(nil)
	})
}

type sendRPC struct {
	sent        chan *types.Transaction
	logs        []*types.Log
	application common.Address
	inputBox    common.Address
	getterID    []byte
	getterCalls atomic.Int32
	chainReads  atomic.Int32
	receipts    atomic.Int32
	estimates   atomic.Int32
}

type sendCall struct {
	To    common.Address `json:"to"`
	Input hexutil.Bytes  `json:"input"`
}

func (b *sendRPC) Call(_ context.Context, call sendCall, block string) (hexutil.Bytes, error) {
	if call.To != b.application || !bytes.Equal(call.Input, b.getterID) || block != "latest" {
		return nil, fmt.Errorf("unexpected application getter: %+v at %s", call, block)
	}
	b.getterCalls.Add(1)
	return common.LeftPadBytes(b.inputBox.Bytes(), common.HashLength), nil
}

func (b *sendRPC) ChainId(context.Context) *hexutil.Big { //nolint:revive // Ethereum RPC method eth_chainId.
	b.chainReads.Add(1)
	return (*hexutil.Big)(big.NewInt(31337))
}

func (*sendRPC) GetBlockByNumber(context.Context, string, bool) *types.Header {
	return &types.Header{Number: big.NewInt(16), Difficulty: big.NewInt(1), GasLimit: sendTestGas}
}

func (*sendRPC) GasPrice(context.Context) *hexutil.Big { return (*hexutil.Big)(big.NewInt(1)) }

func (*sendRPC) GetTransactionCount(context.Context, common.Address, string) hexutil.Uint64 { return 0 }

func (*sendRPC) GetCode(context.Context, common.Address, string) hexutil.Bytes {
	return hexutil.Bytes{1}
}

func (b *sendRPC) EstimateGas(context.Context, map[string]json.RawMessage) hexutil.Uint64 {
	b.estimates.Add(1)
	return sendTestGas
}

func (b *sendRPC) SendRawTransaction(ctx context.Context, data hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, err
	}
	select {
	case b.sent <- tx:
	case <-ctx.Done():
		return common.Hash{}, ctx.Err()
	}
	return tx.Hash(), nil
}

func (b *sendRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) *types.Receipt {
	b.receipts.Add(1)
	return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(16),
		Logs: b.logs, GasUsed: sendTestGas, CumulativeGasUsed: sendTestGas}
}
