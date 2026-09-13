// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSetupPersistentConfigChecksInitializedValue(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, concurrent := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/concurrent=%t", enabled, concurrent), func(t *testing.T) {
				s, repo := newPRTServiceMock()
				requested := PersistentConfig{
					DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42, ClaimSubmissionEnabled: enabled,
				}
				raw, err := json.Marshal(requested)
				require.NoError(t, err)
				stored := requested
				if concurrent {
					stored.ClaimSubmissionEnabled = !enabled
				}
				storedRaw, err := json.Marshal(stored)
				require.NoError(t, err)
				repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).
					Return(([]byte)(nil), time.Time{}, time.Time{}, repository.ErrNotFound).Once()
				repo.On("InitializeNodeConfigRaw", mock.Anything, PrtConfigKey, raw).Return(nil).Once()
				repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).
					Return(storedRaw, time.Time{}, time.Time{}, nil).Once()
				got, err := s.setupPersistentConfig(t.Context(), &config.PrtConfig{
					BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: enabled,
				})
				if concurrent {
					require.Nil(t, got)
					require.ErrorContains(t, err, "claim submission mode mismatch")
				} else {
					require.NoError(t, err)
					require.Equal(t, requested, *got)
				}
				repo.AssertNumberOfCalls(t, "SaveNodeConfigRaw", 0)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestCreateRejectsConfigBeforeSignerResolution(t *testing.T) {
	viper.Set(config.PRT_AUTH_KIND, "invalid")
	t.Cleanup(func() { viper.Set(config.PRT_AUTH_KIND, nil) })
	for _, test := range []struct {
		name             string
		raw              string
		requestedEnabled bool
		want             string
	}{
		{"chain mismatch", `{"ChainID":2,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true}`, true,
			"chain ID mismatch: database=2, configured=42"},
		{"policy mismatch", `{"ChainID":42,"DefaultBlock":"LATEST","ClaimSubmissionEnabled":true}`, true,
			"observation policy mismatch: database=LATEST, configured=FINALIZED"},
		{"enable submission", `{"ChainID":42,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":false}`, true,
			"claim submission mode mismatch: database=false, configured=true"},
		{"disable submission", `{"ChainID":42,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true}`, false,
			"claim submission mode mismatch: database=true, configured=false"},
		{"missing mode", `{"ChainID":42,"DefaultBlock":"FINALIZED"}`, true, "non-null ClaimSubmissionEnabled"},
		{"null mode", `{"ChainID":42,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":null}`, true,
			"non-null ClaimSubmissionEnabled"},
		{"null config", `null`, true, "non-null DefaultBlock and ChainID"},
		{"invalid policy", `{"ChainID":42,"DefaultBlock":"invalid","ClaimSubmissionEnabled":true}`, true, "invalid DefaultBlock"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &prtBlockPolicyCreateRepository{}
			repo.On("LoadNodeConfigRaw", mock.Anything, PrtConfigKey).Return([]byte(test.raw), nil).Once()
			client := &ethClientMock{}
			client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
			info := &CreateInfo{
				CreateInfo: service.CreateInfo{Name: "prt", Context: t.Context(), PollInterval: time.Hour},
				Config: config.PrtConfig{
					BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: test.requestedEnabled, BlockchainHttpRequestTimeout: time.Second,
				},
				EthClient: client, Repository: repo,
			}
			svc, err := Create(t.Context(), info)
			created := info.Impl.(*Service)
			t.Cleanup(func() { created.Cancel(); created.Ticker.Stop() })
			require.Nil(t, svc)
			require.ErrorContains(t, err, test.want)
			require.Nil(t, created.txOptsFactory)
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}
