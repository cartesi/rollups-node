// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
)

func TestLegacyFeeFactorySignedTransactions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		force      bool
		baseFee    *big.Int
		gasLimit   uint64
		wantType   uint8
		wantPrices int32
	}{
		{name: "automatic dynamic", baseFee: big.NewInt(10), wantType: types.DynamicFeeTxType},
		{name: "automatic legacy", wantType: types.LegacyTxType, wantPrices: 2},
		{name: "forced legacy estimated", force: true, baseFee: big.NewInt(10), wantType: types.LegacyTxType, wantPrices: 2},
		{
			name: "forced legacy manual gas", force: true, baseFee: big.NewInt(10), gasLimit: 123456,
			wantType: types.LegacyTxType, wantPrices: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			backend := &legacyFeeRPC{baseFee: test.baseFee}
			server := rpc.NewServer()
			require.NoError(t, server.RegisterName("eth", backend))
			t.Cleanup(server.Stop)
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			key, err := crypto.GenerateKey()
			require.NoError(t, err)
			chainID := big.NewInt(31337)
			opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
			require.NoError(t, err)
			opts.Nonce = big.NewInt(7)
			opts.Value = big.NewInt(19)
			opts.GasLimit = test.gasLimit
			opts.NoSend = true
			factory := NewStaticTransactOptsFactory(opts)
			if test.force {
				// A forced format must not leave incompatible dynamic fee fields.
				opts.GasFeeCap, opts.GasTipCap = big.NewInt(20), big.NewInt(2)
				factory = WithLegacyFees(factory, client)
			}
			require.Equal(t, opts.From, factory.From())
			inputBox, err := iinputbox.NewIInputBox(common.HexToAddress("0x01"), client)
			require.NoError(t, err)
			for transaction := int64(1); transaction <= 2; transaction++ {
				prepared, err := factory.NewTransactOpts(t.Context())
				require.NoError(t, err)
				require.Equal(t, t.Context(), prepared.Context)
				tx, err := inputBox.AddInput(prepared, common.HexToAddress("0x02"), []byte{3})
				require.NoError(t, err)
				require.Equal(t, test.wantType, tx.Type())
				require.Equal(t, opts.Nonce.Uint64(), tx.Nonce())
				require.Equal(t, opts.Value, tx.Value())
				require.Equal(t, chainID, tx.ChainId())
				from, err := types.Sender(types.LatestSignerForChainID(chainID), tx)
				require.NoError(t, err)
				require.Equal(t, opts.From, from)
				if test.wantType == types.LegacyTxType {
					require.Equal(t, big.NewInt(100+transaction), tx.GasPrice(), "fetch a fresh price for each transaction")
				}
				if test.gasLimit == 0 {
					require.EqualValues(t, 75000, tx.Gas())
				} else {
					require.Equal(t, test.gasLimit, tx.Gas())
				}
			}
			require.Nil(t, opts.GasPrice, "do not mutate the source options")
			if test.force {
				require.Equal(t, big.NewInt(20), opts.GasFeeCap)
				require.Equal(t, big.NewInt(2), opts.GasTipCap)
			}
			require.Equal(t, test.wantPrices, backend.prices.Load())
			if test.gasLimit == 0 {
				require.EqualValues(t, 2, backend.estimates.Load())
			} else {
				require.Zero(t, backend.estimates.Load())
			}
		})
	}
}

type legacyFeeRPC struct {
	baseFee   *big.Int
	prices    atomic.Int32
	estimates atomic.Int32
}

func (r *legacyFeeRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(100 + int64(r.prices.Add(1)))), nil
}

func (r *legacyFeeRPC) GetBlockByNumber(context.Context, string, bool) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0), BaseFee: r.baseFee}, nil
}

func (*legacyFeeRPC) MaxPriorityFeePerGas(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(2)), nil
}

func (*legacyFeeRPC) GetCode(context.Context, common.Address, string) (hexutil.Bytes, error) {
	return hexutil.Bytes{1}, nil
}

func (r *legacyFeeRPC) EstimateGas(context.Context, map[string]any) (hexutil.Uint64, error) {
	r.estimates.Add(1)
	return 75000, nil
}

type legacyGasPricer func(context.Context) (*big.Int, error)

func (p legacyGasPricer) SuggestGasPrice(ctx context.Context) (*big.Int, error) { return p(ctx) }

func TestLegacyFeeFactoryErrors(t *testing.T) {
	t.Parallel()
	cause := errors.New("provider unavailable")
	for _, test := range []struct {
		name  string
		price *big.Int
		err   error
	}{
		{name: "RPC error", err: cause},
		{name: "missing price"},
		{name: "negative price", price: big.NewInt(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			opts := &bind.TransactOpts{GasPrice: big.NewInt(2), GasFeeCap: big.NewInt(3), GasTipCap: big.NewInt(1)}
			factory := WithLegacyFees(NewStaticTransactOptsFactory(opts), legacyGasPricer(func(ctx context.Context) (*big.Int, error) {
				require.Equal(t, t.Context(), ctx)
				return test.price, test.err
			}))
			prepared, err := factory.NewTransactOpts(t.Context())
			require.Nil(t, prepared)
			require.ErrorContains(t, err, "suggest legacy gas price")
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			}
			require.Equal(t, big.NewInt(2), opts.GasPrice)
			require.Equal(t, big.NewInt(3), opts.GasFeeCap)
			require.Equal(t, big.NewInt(1), opts.GasTipCap)
		})
	}
}

func TestLegacyFeeFactoryAcceptsZeroPrice(t *testing.T) {
	t.Parallel()
	opts := &bind.TransactOpts{GasPrice: big.NewInt(2), GasFeeCap: big.NewInt(3), GasTipCap: big.NewInt(1)}
	factory := WithLegacyFees(NewStaticTransactOptsFactory(opts), legacyGasPricer(func(ctx context.Context) (*big.Int, error) {
		require.Equal(t, t.Context(), ctx)
		return big.NewInt(0), nil
	}))
	prepared, err := factory.NewTransactOpts(t.Context())
	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.Equal(t, big.NewInt(0), prepared.GasPrice)
	require.Nil(t, prepared.GasFeeCap)
	require.Nil(t, prepared.GasTipCap)
	require.Equal(t, big.NewInt(2), opts.GasPrice)
	require.Equal(t, big.NewInt(3), opts.GasFeeCap)
	require.Equal(t, big.NewInt(1), opts.GasTipCap)
}

type failedLegacyOptsFactory struct {
	TransactOptsFactory
	err error
}

func (f failedLegacyOptsFactory) NewTransactOpts(context.Context) (*bind.TransactOpts, error) {
	return nil, f.err
}

func TestLegacyFeeFactoryPreservesPreparationErrors(t *testing.T) {
	t.Parallel()
	cause := errors.New("signer unavailable")
	factory := WithLegacyFees(failedLegacyOptsFactory{err: cause}, legacyGasPricer(func(context.Context) (*big.Int, error) {
		t.Fatal("do not request a price when signer preparation fails")
		return nil, nil
	}))
	opts, err := factory.NewTransactOpts(t.Context())
	require.Nil(t, opts)
	require.ErrorIs(t, err, cause)
}

func TestLegacyFeeFactoryPreservesCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	factory := WithLegacyFees(NewStaticTransactOptsFactory(&bind.TransactOpts{}),
		legacyGasPricer(func(priceCtx context.Context) (*big.Int, error) {
			require.Equal(t, ctx, priceCtx)
			return nil, priceCtx.Err()
		}))
	opts, err := factory.NewTransactOpts(ctx)
	require.Nil(t, opts)
	require.ErrorIs(t, err, context.Canceled)
}
