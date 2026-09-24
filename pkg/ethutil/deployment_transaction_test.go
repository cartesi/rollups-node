// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorumfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iselfhostedapplicationfactory"
)

func TestDeploymentsHandleBroadcastOnlyAndFailedReceipts(t *testing.T) {
	appAddress := common.HexToAddress("0x01")
	consensusAddress := common.HexToAddress("0x02")
	factoryAddress := common.HexToAddress("0x03")
	app := &ApplicationDeployment{FactoryAddress: factoryAddress}
	selfhosted := &SelfhostedApplicationDeployment{FactoryAddress: factoryAddress}
	prt := &PRTApplicationDeployment{ApplicationDeployment: ApplicationDeployment{FactoryAddress: factoryAddress}}
	authority := &AuthorityDeployment{FactoryAddress: factoryAddress}
	quorum := &QuorumDeployment{FactoryAddress: factoryAddress}

	for _, variant := range []struct {
		name     string
		metadata *bind.MetaData
		method   string
		outputs  []any
		deploy   func(context.Context, *ethclient.Client, *bind.TransactOpts, TransactionRunner) (common.Address, error)
	}{
		{
			name: "application", metadata: iapplicationfactory.IApplicationFactoryMetaData,
			method: "calculateApplicationAddress", outputs: []any{appAddress},
			deploy: func(
				ctx context.Context, client *ethclient.Client, opts *bind.TransactOpts, runner TransactionRunner,
			) (common.Address, error) {
				address, result, err := app.DeployWithTransaction(ctx, client, NewStaticTransactOptsFactory(opts), runner)
				if result != nil {
					return address, fmt.Errorf("unexpected confirmed deployment result")
				}
				return address, err
			},
		},
		{
			name: "selfhosted", metadata: iselfhostedapplicationfactory.ISelfHostedApplicationFactoryMetaData,
			method: "calculateAddresses", outputs: []any{appAddress, consensusAddress},
			deploy: func(
				ctx context.Context, client *ethclient.Client, opts *bind.TransactOpts, runner TransactionRunner,
			) (common.Address, error) {
				address, result, err := selfhosted.DeployWithTransaction(ctx, client, NewStaticTransactOptsFactory(opts), runner)
				if result != nil {
					return address, fmt.Errorf("unexpected confirmed deployment result")
				}
				return address, err
			},
		},
		{
			name: "prt", metadata: idaveappfactory.IDaveAppFactoryMetaData,
			method: "calculateDaveAppAddress", outputs: []any{appAddress, consensusAddress},
			deploy: func(
				ctx context.Context, client *ethclient.Client, opts *bind.TransactOpts, runner TransactionRunner,
			) (common.Address, error) {
				address, result, err := prt.DeployWithTransaction(ctx, client, NewStaticTransactOptsFactory(opts), runner)
				if result != nil {
					return address, fmt.Errorf("unexpected confirmed deployment result")
				}
				return address, err
			},
		},
		{
			name: "authority", metadata: iauthorityfactory.IAuthorityFactoryMetaData,
			method: "calculateAuthorityAddress", outputs: []any{consensusAddress}, deploy: authority.DeployWithTransaction,
		},
		{
			name: "quorum", metadata: iquorumfactory.IQuorumFactoryMetaData,
			method: "calculateQuorumAddress", outputs: []any{consensusAddress}, deploy: quorum.DeployWithTransaction,
		},
	} {
		t.Run(variant.name, func(t *testing.T) {
			for _, noWait := range []bool{true, false} {
				t.Run(fmt.Sprintf("no_wait_%t", noWait), func(t *testing.T) {
					parsed, err := variant.metadata.GetAbi()
					require.NoError(t, err)
					method := parsed.Methods[variant.method]
					data, err := method.Outputs.Pack(variant.outputs...)
					require.NoError(t, err)
					backend := &deploymentCallBackend{responses: map[string]hexutil.Bytes{string(method.ID): data}}
					if variant.name == "selfhosted" {
						for _, name := range []string{"getApplicationFactory", "getAuthorityFactory"} {
							getter := parsed.Methods[name]
							getterData, err := getter.Outputs.Pack(factoryAddress)
							require.NoError(t, err)
							backend.responses[string(getter.ID)] = getterData
						}
					}
					server := rpc.NewServer()
					require.NoError(t, server.RegisterName("eth", backend))
					defer server.Stop()
					client := ethclient.NewClient(rpc.DialInProc(server))
					defer client.Close()
					opts := &bind.TransactOpts{
						From: common.HexToAddress("0x04"), Nonce: big.NewInt(0),
						GasPrice: big.NewInt(1), GasLimit: 1_000_000, NoSend: true,
						Signer: func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
					}
					called := false
					var hash common.Hash
					address, err := variant.deploy(t.Context(), client, opts,
						func(
							ctx context.Context, txOpts *bind.TransactOpts, build func(*bind.TransactOpts) (*types.Transaction, error),
						) (*types.Receipt, error) {
							called = true
							require.Equal(t, t.Context(), ctx)
							tx, err := build(txOpts)
							require.NoError(t, err)
							hash = tx.Hash()
							if noWait {
								return nil, nil
							}
							return &types.Receipt{Status: types.ReceiptStatusFailed, TxHash: hash}, nil
						})
					require.True(t, called)
					if noWait {
						require.NoError(t, err)
						require.Equal(t, variant.outputs[0], address, "return the primary address computed before broadcast")
					} else {
						require.Zero(t, address)
						require.ErrorContains(t, err, hash.Hex())
						require.ErrorContains(t, err, "failed")
					}
				})
			}
		})
	}
}

type deploymentCallBackend struct {
	responses map[string]hexutil.Bytes
}

func (b *deploymentCallBackend) Call(_ context.Context, call prtFactoryCall, _ string) (hexutil.Bytes, error) {
	if len(call.Input) >= 4 {
		if response, found := b.responses[string(call.Input[:4])]; found {
			return response, nil
		}
	}
	return nil, fmt.Errorf("unexpected contract call: %x", call.Input)
}

func (*deploymentCallBackend) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	return hexutil.Bytes{}, nil
}

func TestDefaultDeploymentRunnerKeepsHashAndChecksReceipt(t *testing.T) {
	tx := types.NewTx(&types.LegacyTx{Nonce: 7, Gas: 12345, GasPrice: big.NewInt(1)})
	for _, test := range []struct {
		name         string
		sendError    error
		receiptError error
		status       uint64
		wantError    string
	}{
		{name: "mined", status: types.ReceiptStatusSuccessful},
		{name: "reverted", status: types.ReceiptStatusFailed, wantError: "failed"},
		{name: "broadcast error", sendError: errors.New("connection closed"), wantError: "failed to broadcast"},
		{name: "wait timeout", receiptError: errors.New("provider unavailable"), wantError: "outcome is unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := &deploymentReceiptBackend{
				sendError: test.sendError, receiptError: test.receiptError,
				receipt: &types.Receipt{
					TxHash: tx.Hash(), Status: test.status, BlockNumber: big.NewInt(1), Logs: []*types.Log{},
				},
			}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", backend))
			defer server.Stop()
			client := ethclient.NewClient(rpc.DialInProc(server))
			defer client.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			opts := &bind.TransactOpts{GasLimit: 12345}
			receipt, err := runDeploymentTransaction(ctx, client, opts, func(prepared *bind.TransactOpts) (*types.Transaction, error) {
				require.True(t, prepared.NoSend)
				require.Equal(t, opts.GasLimit, prepared.GasLimit)
				require.Equal(t, ctx, prepared.Context)
				return tx, nil
			}, nil)
			require.False(t, opts.NoSend, "the caller's options must remain unchanged")
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.ErrorContains(t, err, tx.Hash().Hex())
				require.Nil(t, receipt)
			} else {
				require.NoError(t, err)
				require.Equal(t, tx.Hash(), receipt.TxHash)
			}
		})
	}
}

type deploymentReceiptBackend struct {
	sendError    error
	receiptError error
	receipt      *types.Receipt
}

func (b *deploymentReceiptBackend) SendRawTransaction(context.Context, hexutil.Bytes) (common.Hash, error) {
	return b.receipt.TxHash, b.sendError
}

func (b *deploymentReceiptBackend) GetTransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return b.receipt, b.receiptError
}

func TestSelfhostedDefaultPreservesTransactionPreparation(t *testing.T) {
	parsed, err := iselfhostedapplicationfactory.ISelfHostedApplicationFactoryMetaData.GetAbi()
	require.NoError(t, err)
	backend := &selfhostedTransactionBackend{deploymentCallBackend: &deploymentCallBackend{responses: map[string]hexutil.Bytes{}}}
	for _, name := range []string{"calculateAddresses", "getApplicationFactory", "getAuthorityFactory"} {
		method := parsed.Methods[name]
		outputs := []any{common.HexToAddress("0x01")}
		if name == "calculateAddresses" {
			outputs = append(outputs, common.HexToAddress("0x02"))
		}
		data, err := method.Outputs.Pack(outputs...)
		require.NoError(t, err)
		backend.responses[string(method.ID)] = data
	}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	defer server.Stop()
	client := ethclient.NewClient(rpc.DialInProc(server))
	defer client.Close()
	opts := &bind.TransactOpts{
		From: common.HexToAddress("0x03"), Nonce: big.NewInt(99), GasPrice: big.NewInt(88), Value: big.NewInt(77), GasLimit: 12345,
		Signer: func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
	}
	deployment := &SelfhostedApplicationDeployment{FactoryAddress: common.HexToAddress("0x04")}
	_, _, err = deployment.Deploy(t.Context(), client, NewStaticTransactOptsFactory(opts))
	require.ErrorContains(t, err, "stop after transaction preparation")
	require.NotNil(t, backend.tx)
	require.Equal(t, uint64(7), backend.tx.Nonce())
	require.Equal(t, big.NewInt(42), backend.tx.GasPrice())
	require.Zero(t, backend.tx.Value().Sign())
	require.Equal(t, opts.GasLimit, backend.tx.Gas())
	require.Equal(t, big.NewInt(99), opts.Nonce, "the original caller options are not changed")
}

type selfhostedTransactionBackend struct {
	*deploymentCallBackend
	tx *types.Transaction
}

func (*selfhostedTransactionBackend) GetTransactionCount(context.Context, common.Address, string) hexutil.Uint64 {
	return 7
}

func (*selfhostedTransactionBackend) GasPrice(context.Context) hexutil.Uint64 {
	return 42
}

func (b *selfhostedTransactionBackend) SendRawTransaction(_ context.Context, data hexutil.Bytes) (common.Hash, error) {
	b.tx = new(types.Transaction)
	if err := b.tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, err
	}
	return b.tx.Hash(), fmt.Errorf("stop after transaction preparation")
}
