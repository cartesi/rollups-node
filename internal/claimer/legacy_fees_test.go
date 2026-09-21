// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

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
		for _, injectClient := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%t/injected=%t", legacy, injectClient), func(t *testing.T) {
				viper.Set(config.AUTH_KIND, "private_key")
				viper.Set(config.AUTH_PRIVATE_KEY, "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
				t.Cleanup(func() {
					viper.Set(config.AUTH_KIND, nil)
					viper.Set(config.AUTH_PRIVATE_KEY, nil)
				})
				raw, err := json.Marshal(PersistentConfig{
					ChainID: 42, DefaultBlock: model.DefaultBlock_Finalized, ClaimSubmissionEnabled: true,
				})
				require.NoError(t, err)
				repo := &claimerCreateRepositoryMock{}
				repo.On("LoadNodeConfigRaw", mock.Anything, ClaimerConfigKey).Return(raw, time.Time{}, time.Time{}, nil).Once()
				backend := &claimerLegacyFeeRPC{chainIDRPC: chainIDRPC{chainID: 42}}
				server := rpc.NewServer()
				require.NoError(t, server.RegisterName("eth", backend))
				t.Cleanup(server.Stop)
				httpServer := httptest.NewServer(server)
				t.Cleanup(httpServer.Close)
				endpoint, err := config.ToURLFromString(httpServer.URL)
				require.NoError(t, err)
				var client *ethclient.Client
				if injectClient {
					client = ethclient.NewClient(rpc.DialInProc(server))
					t.Cleanup(client.Close)
				}
				s, err := Create(t.Context(), &CreateInfo{
					Config: config.ClaimerConfig{
						ClaimerPollingInterval: time.Hour,
						BlockchainId:           42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
						BlockchainHttpEndpoint:        endpoint,
						FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
						BlockchainLegacyEnabled: legacy,
					},
					EthConn: client, Repository: repo,
				})
				require.NoError(t, err)
				blockchain, ok := s.(*Service).blockchain.(*claimerBlockchain)
				require.True(t, ok)
				if !injectClient {
					t.Cleanup(blockchain.client.Close)
				}
				for sequence := int64(1); sequence <= 2; sequence++ {
					opts, err := blockchain.txOptsFactory.NewTransactOpts(t.Context())
					require.NoError(t, err)
					require.Zero(t, opts.GasLimit, "keep estimation enabled")
					if legacy {
						require.Equal(t, big.NewInt(100+sequence), opts.GasPrice)
					} else {
						require.Nil(t, opts.GasPrice)
					}
				}
				if legacy {
					require.EqualValues(t, 2, backend.prices.Load())
				} else {
					require.Zero(t, backend.prices.Load())
				}
				repo.AssertExpectations(t)
			})
		}
	}
}

type claimerLegacyFeeRPC struct {
	chainIDRPC
	prices atomic.Int32
}

func (r *claimerLegacyFeeRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(100 + int64(r.prices.Add(1)))), nil
}
