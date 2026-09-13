// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateExplainsRequiredPRTSigner(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "submission disabled"
		if enabled {
			name = "submission enabled"
		}
		t.Run(name, func(t *testing.T) {
			viper.Set(config.PRT_AUTH_KIND, "mnemonic_file")
			viper.Set(config.PRT_AUTH_MNEMONIC, "")
			viper.Set(config.PRT_AUTH_MNEMONIC_FILE, filepath.Join(t.TempDir(), "missing-mnemonic"))
			t.Cleanup(func() {
				viper.Set(config.PRT_AUTH_KIND, nil)
				viper.Set(config.PRT_AUTH_MNEMONIC, nil)
				viper.Set(config.PRT_AUTH_MNEMONIC_FILE, nil)
			})
			raw, err := json.Marshal(PersistentConfig{
				ChainID: 42, DefaultBlock: model.DefaultBlock_Finalized, ClaimSubmissionEnabled: enabled,
			})
			require.NoError(t, err)
			repo := &prtBlockPolicyCreateRepository{}
			repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).Return(raw, nil).Once()
			client := &ethClientMock{}
			client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
			svc, err := Create(t.Context(), &CreateInfo{
				Config: config.PrtConfig{
					PrtPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: enabled, BlockchainHttpRequestTimeout: time.Second,
				},
				Repository: repo, EthClient: client, AdapterFactory: &adapterFactoryMock{},
			})
			if enabled {
				require.Nil(t, svc)
				require.ErrorIs(t, err, os.ErrNotExist, "preserve the underlying credential error")
				require.ErrorContains(t, err, "PRT submission is enabled and requires signer configuration (CARTESI_PRT_AUTH_*)")
				require.ErrorContains(t, err, "the standalone node always starts PRT")
			} else {
				require.NoError(t, err, "reader mode must not require signer credentials")
				require.Nil(t, svc.(*Service).txOptsFactory)
			}
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}
