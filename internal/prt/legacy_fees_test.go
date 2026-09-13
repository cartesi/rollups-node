// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
)

func TestCreateWiresLegacyFees(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			viper.Set(config.PRT_AUTH_KIND, "private_key")
			viper.Set(config.PRT_AUTH_PRIVATE_KEY, "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
			t.Cleanup(func() {
				viper.Set(config.PRT_AUTH_KIND, nil)
				viper.Set(config.PRT_AUTH_PRIVATE_KEY, nil)
			})
			raw, err := json.Marshal(PersistentConfig{
				ChainID: 42, DefaultBlock: model.DefaultBlock_Finalized, ClaimSubmissionEnabled: true,
			})
			require.NoError(t, err)
			repo := &prtBlockPolicyCreateRepository{}
			repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).Return(raw, nil).Once()
			client := &ethClientMock{}
			client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
			if legacy {
				client.On("SuggestGasPrice", mock.Anything).Return(big.NewInt(101), nil).Once()
				client.On("SuggestGasPrice", mock.Anything).Return(big.NewInt(102), nil).Once()
			}
			svc, err := Create(t.Context(), &CreateInfo{
				Config: config.PrtConfig{
					PrtPollingInterval: time.Hour,
					BlockchainId:       42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
					BlockchainLegacyEnabled: legacy,
				},
				EthClient: client, Repository: repo, AdapterFactory: &adapterFactoryMock{},
			})
			require.NoError(t, err)
			s := svc.(*Service)
			for sequence := int64(1); sequence <= 2; sequence++ {
				opts, err := s.txOptsFactory.NewTransactOpts(t.Context())
				require.NoError(t, err)
				require.Zero(t, opts.GasLimit, "keep estimation enabled")
				if legacy {
					require.Equal(t, big.NewInt(100+sequence), opts.GasPrice)
				} else {
					require.Nil(t, opts.GasPrice)
				}
			}
			if !legacy {
				client.AssertNotCalled(t, "SuggestGasPrice", mock.Anything)
			}
			client.AssertExpectations(t)
			repo.AssertExpectations(t)
		})
	}
}

func TestCreateWiresLegacyFeesWithDefaultClient(t *testing.T) {
	viper.Set(config.PRT_AUTH_KIND, "private_key")
	viper.Set(config.PRT_AUTH_PRIVATE_KEY, "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	t.Cleanup(func() {
		viper.Set(config.PRT_AUTH_KIND, nil)
		viper.Set(config.PRT_AUTH_PRIVATE_KEY, nil)
	})
	backend := &prtLegacyFeeRPC{}
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	endpoint, err := config.ToURLFromString(httpServer.URL)
	require.NoError(t, err)
	raw, err := json.Marshal(PersistentConfig{
		ChainID: 42, DefaultBlock: model.DefaultBlock_Finalized, ClaimSubmissionEnabled: true,
	})
	require.NoError(t, err)
	repo := &prtBlockPolicyCreateRepository{}
	repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).Return(raw, nil).Once()
	svc, err := Create(t.Context(), &CreateInfo{
		Config: config.PrtConfig{
			PrtPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
			BlockchainHttpEndpoint: endpoint, BlockchainHttpRequestTimeout: time.Second,
			FeatureClaimSubmissionEnabled: true, BlockchainLegacyEnabled: true,
		},
		Repository: repo,
	})
	require.NoError(t, err)
	s := svc.(*Service)
	client, ok := s.client.(*ethclient.Client)
	require.True(t, ok)
	t.Cleanup(client.Close)
	factory, ok := s.adapterFactory.(*DefaultAdapterFactory)
	require.True(t, ok)
	require.Same(t, client, factory.client)
	for sequence := int64(1); sequence <= 2; sequence++ {
		opts, err := s.txOptsFactory.NewTransactOpts(t.Context())
		require.NoError(t, err)
		require.Zero(t, opts.GasLimit, "keep estimation enabled")
		require.Equal(t, big.NewInt(100+sequence), opts.GasPrice)
	}
	require.EqualValues(t, 2, backend.prices.Load())
	repo.AssertExpectations(t)
}

type prtLegacyFeeRPC struct {
	prices atomic.Int32
}

func (*prtLegacyFeeRPC) ChainId(context.Context) (*hexutil.Big, error) { //nolint:revive // RPC registration requires eth_chainId.
	return (*hexutil.Big)(big.NewInt(42)), nil
}

func (r *prtLegacyFeeRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(100 + int64(r.prices.Add(1)))), nil
}
