// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"
	"math/big"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
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
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Latest} {
		for _, legacy := range []bool{false, true} {
			for _, injectClient := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/legacy=%t/injected=%t", policy, legacy, injectClient), func(t *testing.T) {
					viper.Set(config.AUTH_KIND, "private_key")
					viper.Set(config.AUTH_PRIVATE_KEY, "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
					t.Cleanup(func() {
						viper.Set(config.AUTH_KIND, nil)
						viper.Set(config.AUTH_PRIVATE_KEY, nil)
					})
					requested := PersistentConfig{
						ChainID: 42, DefaultBlock: policy, ClaimSubmissionEnabled: true,
						ResolvedSignerAddress: common.HexToAddress("0x70997970C51812dc3a010C7d01b50e0d17dc79C8"),
					}
					repo := &claimerCreateRepositoryMock{}
					repo.On("RegisterSubmitter", mock.Anything, ClaimerConfigKey, requested).Return(requested, nil).Once()
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
							BlockchainId:           42, BlockchainDefaultBlock: policy,
							BlockchainHttpEndpoint:        endpoint,
							FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
							BlockchainLegacyEnabled:  legacy,
							BlockchainTxMaxFeePerGas: "1000000000",
						},
						EthConn: client, Repository: repo,
					})
					require.NoError(t, err)
					blockchain, ok := s.(*Service).blockchain.(*claimerBlockchain)
					require.True(t, ok)
					if !injectClient {
						t.Cleanup(blockchain.client.Close)
					}
					impl := s.(*Service)
					require.NotNil(t, impl.publisher)
					require.NotNil(t, impl.publisher.sender)
					require.Equal(t, requested.DefaultBlock, impl.publisher.observationPolicy)
					require.False(t, impl.Ready(), "Serve waits for the contributing observer before publication")
					opts, err := blockchain.txOptsFactory.NewTransactOpts(t.Context())
					require.NoError(t, err)
					require.Nil(t, opts.GasPrice, "the signer factory does not perform unpinned fee reads")
					require.Zero(t, backend.prices.Load())
					repo.AssertExpectations(t)
				})
			}
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
