// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

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
			s, err := Create(t.Context(), &CreateInfo{
				CreateInfo: service.CreateInfo{Context: t.Context(), PollInterval: time.Hour},
				Config: config.PrtConfig{
					BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
					BlockchainLegacyEnabled: legacy,
				},
				EthClient: client, Repository: repo, AdapterFactory: &adapterFactoryMock{},
			})
			require.NoError(t, err)
			t.Cleanup(func() { s.Cancel(); s.Ticker.Stop() })
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
