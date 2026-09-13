// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"context"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
)

func TestTransactLegacyFeePolicy(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, manualGas := range []bool{false, true} {
			for _, noWait := range []bool{false, true} {
				t.Run(fmt.Sprintf("legacy_%t/manual_gas_%t/no_wait_%t", legacy, manualGas, noWait), func(t *testing.T) {
					viper.Reset()
					config.SetDefaults()
					t.Cleanup(func() { viper.Reset(); config.SetDefaults() })
					viper.Set(config.AUTH_KIND, "private_key")
					viper.Set(config.AUTH_PRIVATE_KEY, testPrivateKey)
					viper.Set(config.BLOCKCHAIN_LEGACY_ENABLED, legacy)
					if manualGas {
						viper.Set(config.BLOCKCHAIN_GAS_LIMIT, "123456")
					}
					backend := &cliLegacyFeeRPC{sent: make(chan *types.Transaction, 2)}
					server := rpc.NewServer()
					require.NoError(t, server.RegisterName("eth", backend))
					t.Cleanup(server.Stop)
					client := ethclient.NewClient(rpc.DialInProc(server))
					t.Cleanup(client.Close)
					chainID := big.NewInt(31337)
					opts, err := GetTransactOpts(t.Context(), chainID)
					require.NoError(t, err)
					opts.Nonce, opts.Value = big.NewInt(9), big.NewInt(8)
					inputBox, err := iinputbox.NewIInputBox(common.HexToAddress("0x01"), client)
					require.NoError(t, err)
					cmd, _, _ := transactionCommand()
					require.NoError(t, cmd.Flags().Set("no-wait", fmt.Sprint(noWait)))
					for sequence := int64(1); sequence <= 2; sequence++ {
						tx, receipt, err := Transact(t.Context(), cmd, client, opts,
							func(prepared *bind.TransactOpts) (*types.Transaction, error) {
								require.True(t, prepared.NoSend)
								require.Equal(t, t.Context(), prepared.Context)
								return inputBox.AddInput(prepared, common.HexToAddress("0x02"), []byte{3})
							})
						require.NoError(t, err)
						require.Len(t, backend.sent, 1, "broadcast exactly once")
						sent := <-backend.sent
						require.Equal(t, tx.Hash(), sent.Hash())
						if legacy {
							require.EqualValues(t, types.LegacyTxType, sent.Type())
							require.Equal(t, big.NewInt(100+sequence), sent.GasPrice())
						} else {
							require.EqualValues(t, types.DynamicFeeTxType, sent.Type())
						}
						from, err := types.Sender(types.LatestSignerForChainID(chainID), sent)
						require.NoError(t, err)
						require.Equal(t, opts.From, from)
						require.Equal(t, opts.Nonce.Uint64(), sent.Nonce())
						require.Equal(t, opts.Value, sent.Value())
						if manualGas {
							require.EqualValues(t, 123456, sent.Gas())
						} else {
							require.EqualValues(t, 75000, sent.Gas())
						}
						if noWait {
							require.Nil(t, receipt)
						} else {
							require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
						}
					}
					require.False(t, opts.NoSend)
					require.Nil(t, opts.GasPrice, "do not cache the fee in caller options")
					if legacy {
						require.EqualValues(t, 2, backend.prices.Load())
					} else {
						require.Zero(t, backend.prices.Load())
					}
					if manualGas {
						require.Zero(t, backend.estimates.Load())
					} else {
						require.EqualValues(t, 2, backend.estimates.Load())
					}
					if noWait {
						require.Zero(t, backend.receipts.Load())
					} else {
						require.EqualValues(t, 2, backend.receipts.Load())
					}
				})
			}
		}
	}
}

type cliLegacyFeeRPC struct {
	prices    atomic.Int32
	estimates atomic.Int32
	receipts  atomic.Int32
	sent      chan *types.Transaction
}

func (r *cliLegacyFeeRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(100 + int64(r.prices.Add(1)))), nil
}

func (*cliLegacyFeeRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0), BaseFee: big.NewInt(10)}, nil
}

func (*cliLegacyFeeRPC) MaxPriorityFeePerGas(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(2)), nil
}

func (*cliLegacyFeeRPC) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	return hexutil.Bytes{1}, nil
}

func (r *cliLegacyFeeRPC) EstimateGas(context.Context, map[string]any) (hexutil.Uint64, error) {
	r.estimates.Add(1)
	return 75000, nil
}

func (r *cliLegacyFeeRPC) SendRawTransaction(_ context.Context, data hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, err
	}
	r.sent <- tx
	return tx.Hash(), nil
}

func (r *cliLegacyFeeRPC) GetTransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	r.receipts.Add(1)
	return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1), Logs: []*types.Log{}}, nil
}

func TestTransactLegacyFeeFailureDoesNotSign(t *testing.T) {
	viper.Reset()
	config.SetDefaults()
	t.Cleanup(func() { viper.Reset(); config.SetDefaults() })
	viper.Set(config.BLOCKCHAIN_LEGACY_ENABLED, true)
	client := &transactionRPC{}
	cmd, _, stderr := transactionCommand()
	tx, receipt, err := Transact(t.Context(), cmd, client, &bind.TransactOpts{},
		func(*bind.TransactOpts) (*types.Transaction, error) {
			t.Fatal("must not sign without a gas price")
			return nil, nil
		})
	require.ErrorContains(t, err, "prepare transaction fees: suggest legacy gas price")
	require.Nil(t, tx)
	require.Nil(t, receipt)
	require.Empty(t, stderr.String())
	require.Zero(t, client.sends)
	require.Zero(t, client.receipts)
}
