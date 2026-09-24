// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorumfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iselfhostedapplicationfactory"
)

const deploymentCLIProcessEnv = "CARTESI_TEST_DEPLOYMENT_CLI_PROCESS"

func TestDeploymentBroadcastPlainOutput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	tx := types.NewTx(&types.LegacyTx{})
	address := common.HexToAddress("0x1234")
	require.NoError(t, writeDeploymentBroadcast(cmd, tx, address))
	require.Equal(t, "Transaction broadcast: "+tx.Hash().Hex()+
		"\nPredicted address (deployment not confirmed): "+address.Hex()+"\n", stdout.String())
	require.Empty(t, stderr.String())
}

// Run the actual command in a child process. Existing deployment handlers use
// cobra.CheckErr, and this also checks Cobra's default stdout/stderr selection.
func TestDeploymentCLIProcess(t *testing.T) {
	if os.Getenv(deploymentCLIProcessEnv) != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			Cmd.SetArgs(os.Args[i+1:])
			break
		}
	}
	if err := Cmd.ExecuteContext(t.Context()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestDeploymentCLIReportsPredictedAddress(t *testing.T) {
	testExecutable, err := os.Executable()
	require.NoError(t, err)
	factoryAddress := common.HexToAddress("0x1000")
	inputBoxAddress := common.HexToAddress("0x2000")
	predictedAddress := common.HexToAddress("0x3000")
	secondaryAddress := common.HexToAddress("0x4000")
	applicationArgs := []string{"application", "--register=false", "--template-hash=" + (common.Hash{}).Hex()}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "authority", args: []string{"authority", "--authority-factory", factoryAddress.Hex()}},
		{name: "quorum", args: []string{"quorum", "--quorum-factory", factoryAddress.Hex(), "--validator", testValidatorAddress}},
		{name: "selfhosted", args: append(append([]string{}, applicationArgs...), "--factory", factoryAddress.Hex())},
		{name: "application", args: append(append([]string{}, applicationArgs...), "--factory", factoryAddress.Hex(),
			"--consensus", inputBoxAddress.Hex())},
		{name: "prt", args: append(append([]string{}, applicationArgs...), "--prt", "--prt-factory", factoryAddress.Hex())},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := &deploymentCLIBackend{
				responses: map[string]hexutil.Bytes{},
				sent:      make(chan *types.Transaction, 2), predictedAddress: predictedAddress, secondaryAddress: secondaryAddress,
			}
			for _, response := range []struct {
				metadata *bind.MetaData
				method   string
				outputs  []any
			}{
				{iauthorityfactory.IAuthorityFactoryMetaData, "calculateAuthorityAddress", []any{predictedAddress}},
				{iquorumfactory.IQuorumFactoryMetaData, "calculateQuorumAddress", []any{predictedAddress}},
				{iapplicationfactory.IApplicationFactoryMetaData, "calculateApplicationAddress", []any{predictedAddress}},
				{iselfhostedapplicationfactory.ISelfHostedApplicationFactoryMetaData, "calculateAddresses",
					[]any{predictedAddress, secondaryAddress}},
				{iselfhostedapplicationfactory.ISelfHostedApplicationFactoryMetaData, "getApplicationFactory", []any{factoryAddress}},
				{iselfhostedapplicationfactory.ISelfHostedApplicationFactoryMetaData, "getAuthorityFactory", []any{factoryAddress}},
				{idaveappfactory.IDaveAppFactoryMetaData, "calculateDaveAppAddress", []any{predictedAddress, secondaryAddress}},
				{iinputbox.IInputBoxMetaData, "getDeploymentBlockNumber", []any{big.NewInt(1)}},
				{iconsensus.IConsensusMetaData, "getEpochLength", []any{big.NewInt(10)}},
				{iconsensus.IConsensusMetaData, "getClaimStagingPeriod", []any{big.NewInt(0)}},
				{iquorum.IQuorumMetaData, "numOfValidators", []any{big.NewInt(1)}},
			} {
				parsed, err := response.metadata.GetAbi()
				require.NoError(t, err)
				method := parsed.Methods[response.method]
				data, err := method.Outputs.Pack(response.outputs...)
				require.NoError(t, err)
				backend.responses[string(method.ID)] = data
			}
			rpcServer := rpc.NewServer()
			require.NoError(t, rpcServer.RegisterName("eth", backend))
			defer rpcServer.Stop()
			server := httptest.NewServer(rpcServer)
			defer server.Close()
			key, err := crypto.GenerateKey()
			require.NoError(t, err)
			t.Setenv(deploymentCLIProcessEnv, "1")
			t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, server.URL)
			t.Setenv(config.AUTH_KIND, "private_key")
			t.Setenv(config.AUTH_PRIVATE_KEY, hexutil.Encode(crypto.FromECDSA(key)))
			t.Setenv(config.BLOCKCHAIN_GAS_LIMIT, "0")
			t.Setenv(config.CONTRACTS_INPUT_BOX_ADDRESS, inputBoxAddress.Hex())
			t.Setenv(config.DATABASE_CONNECTION, "invalid")
			args := append([]string{"-test.run=^TestDeploymentCLIProcess$", "--"}, test.args...)
			args = append(args, "--no-wait", "--json")
			process := exec.CommandContext(t.Context(), testExecutable, args...)
			var stdout, stderr bytes.Buffer
			process.Stdout = &stdout
			process.Stderr = &stderr
			require.NoError(t, process.Run(), stderr.String())
			require.Len(t, backend.sent, 1)
			tx := <-backend.sent
			require.Equal(t, factoryAddress, *tx.To())
			var result map[string]string
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			require.Equal(t, map[string]string{
				"transaction_hash": tx.Hash().Hex(), "status": "broadcast", "predicted_address": predictedAddress.Hex(),
			}, result)
			require.Contains(t, stderr.String(), tx.Hash().Hex())
			require.Zero(t, backend.receiptReads.Load())
			require.Zero(t, backend.postBroadcastReads.Load(), "no deployment verification before mining")
		})
	}
}

type deploymentCLIBackend struct {
	responses          map[string]hexutil.Bytes
	sent               chan *types.Transaction
	predictedAddress   common.Address
	secondaryAddress   common.Address
	broadcast          atomic.Bool
	receiptReads       atomic.Int32
	postBroadcastReads atomic.Int32
}

func (*deploymentCLIBackend) ChainId(context.Context) *hexutil.Big { //nolint:revive // Ethereum RPC method eth_chainId.
	return (*hexutil.Big)(big.NewInt(31337))
}

func (*deploymentCLIBackend) GetBlockByNumber(context.Context, string, bool) *types.Header {
	return &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(1), GasLimit: 1_000_000}
}

func (*deploymentCLIBackend) GasPrice(context.Context) *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1))
}

func (*deploymentCLIBackend) GetTransactionCount(context.Context, common.Address, string) hexutil.Uint64 {
	return 0
}

func (*deploymentCLIBackend) EstimateGas(context.Context, map[string]json.RawMessage) hexutil.Uint64 {
	return 500_000
}

func (b *deploymentCLIBackend) GetCode(_ context.Context, address common.Address, _ string) hexutil.Bytes {
	if b.broadcast.Load() {
		b.postBroadcastReads.Add(1)
	}
	if address == b.predictedAddress || address == b.secondaryAddress {
		return hexutil.Bytes{}
	}
	return hexutil.Bytes{0x01}
}

func (b *deploymentCLIBackend) Call(_ context.Context, call struct {
	Input hexutil.Bytes `json:"input"`
}, _ string) (hexutil.Bytes, error) {
	if b.broadcast.Load() {
		b.postBroadcastReads.Add(1)
	}
	if len(call.Input) >= 4 {
		if response, found := b.responses[string(call.Input[:4])]; found {
			return response, nil
		}
	}
	return nil, fmt.Errorf("unexpected contract call: %x", call.Input)
}

func (b *deploymentCLIBackend) SendRawTransaction(_ context.Context, raw hexutil.Bytes) (common.Hash, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	b.sent <- &tx
	b.broadcast.Store(true)
	return tx.Hash(), nil
}

func (b *deploymentCLIBackend) GetTransactionReceipt(_ context.Context, hash common.Hash) *types.Receipt {
	b.receiptReads.Add(1)
	return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(10), Logs: []*types.Log{}}
}
