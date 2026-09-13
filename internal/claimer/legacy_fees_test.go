// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
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
	"github.com/cartesi/rollups-node/pkg/service"
)

func TestCreateWiresLegacyFees(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
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
			client := ethclient.NewClient(rpc.DialInProc(server))
			t.Cleanup(client.Close)
			s, err := Create(t.Context(), &CreateInfo{
				CreateInfo: service.CreateInfo{Context: t.Context(), PollInterval: time.Hour},
				Config: config.ClaimerConfig{
					BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
					BlockchainLegacyEnabled: legacy,
				},
				EthConn: client, Repository: repo,
			})
			require.NoError(t, err)
			t.Cleanup(func() { s.Cancel(); s.Ticker.Stop() })
			blockchain, ok := s.blockchain.(*claimerBlockchain)
			require.True(t, ok)
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

type claimerLegacyFeeRPC struct {
	chainIDRPC
	prices atomic.Int32
}

func (r *claimerLegacyFeeRPC) GasPrice(context.Context) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(100 + int64(r.prices.Add(1)))), nil
}
