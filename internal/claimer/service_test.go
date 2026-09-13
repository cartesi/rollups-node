// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSetupPersistentConfigChecksInitializedValue(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, concurrent := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/concurrent=%t", enabled, concurrent), func(t *testing.T) {
				s, repo, _ := newServiceMock(t)
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
				repo.On("LoadNodeConfigRaw", mock.Anything, ClaimerConfigKey).
					Return(([]byte)(nil), time.Time{}, time.Time{}, repository.ErrNotFound).Once()
				repo.On("InitializeNodeConfigRaw", mock.Anything, ClaimerConfigKey, raw).Return(nil).Once()
				repo.On("LoadNodeConfigRaw", mock.Anything, ClaimerConfigKey).
					Return(storedRaw, time.Time{}, time.Time{}, nil).Once()
				got, err := setupPersistentConfig(t.Context(), s.Logger, repo, &config.ClaimerConfig{
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

func TestSubmitterAddress(t *testing.T) {
	address := common.HexToAddress("0x1234")
	tests := []struct {
		name        string
		blockchain  iclaimerBlockchain
		wantAddress common.Address
		wantEnabled bool
	}{
		{name: "disabled"},
		{
			name:        "enabled",
			blockchain:  &claimerBlockchainMock{submitterAddress: address, hasSubmitter: true},
			wantAddress: address,
			wantEnabled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{blockchain: test.blockchain}
			gotAddress, gotEnabled := s.SubmitterAddress()
			require.Equal(t, test.wantAddress, gotAddress)
			require.Equal(t, test.wantEnabled, gotEnabled)
		})
	}
}

func TestCreateRejectsConfigBeforeSignerResolution(t *testing.T) {
	// Invalid auth must not hide a saved-config error or resolve a signing key.
	viper.Set(config.AUTH_KIND, "invalid")
	t.Cleanup(func() { viper.Set(config.AUTH_KIND, nil) })
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
		{"missing chain", `{"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true}`, true, "non-null DefaultBlock and ChainID"},
		{"invalid policy", `{"ChainID":42,"DefaultBlock":"invalid","ClaimSubmissionEnabled":true}`, true, "invalid DefaultBlock"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &claimerCreateRepositoryMock{}
			repo.On("LoadNodeConfigRaw", mock.Anything, ClaimerConfigKey).
				Return([]byte(test.raw), time.Time{}, time.Time{}, nil).Once()
			info := &CreateInfo{
				CreateInfo: service.CreateInfo{Name: "claimer", Context: t.Context(), PollInterval: time.Hour},
				Config: config.ClaimerConfig{
					BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: test.requestedEnabled, BlockchainHttpRequestTimeout: time.Second,
				},
				EthConn: newTestEthClient(t, 42), Repository: repo,
			}
			svc, err := Create(t.Context(), info)
			created := info.Impl.(*Service)
			t.Cleanup(func() { created.Cancel(); created.Ticker.Stop() })
			require.Nil(t, svc)
			require.ErrorContains(t, err, test.want)
			repo.AssertNumberOfCalls(t, "SaveNodeConfigRaw", 0)
			repo.AssertExpectations(t)
		})
	}
}

func TestCreateUsesMatchingPersistedDefaultBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	persistedConfig := PersistentConfig{
		DefaultBlock:           model.DefaultBlock_Latest,
		ClaimSubmissionEnabled: false,
		ChainID:                42,
	}
	rawConfig, err := json.Marshal(persistedConfig)
	require.NoError(t, err)

	repo := &claimerCreateRepositoryMock{}
	repo.On("LoadNodeConfigRaw", mock.Anything, ClaimerConfigKey).
		Return(rawConfig, time.Now(), time.Now(), nil).Once()

	s, err := Create(ctx, &CreateInfo{
		CreateInfo: service.CreateInfo{
			Context:      ctx,
			PollInterval: time.Hour,
		},
		Config: config.ClaimerConfig{
			BlockchainDefaultBlock:        model.DefaultBlock_Latest,
			BlockchainId:                  42,
			FeatureClaimSubmissionEnabled: false,
		},
		EthConn:    newTestEthClient(t, 42),
		Repository: repo,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if s.Ticker != nil {
			s.Ticker.Stop()
		}
		if s.Cancel != nil {
			s.Cancel()
		}
	})

	blockchain, ok := s.blockchain.(*claimerBlockchain)
	require.True(t, ok)
	assert.Equal(t, model.DefaultBlock_Latest, blockchain.defaultBlock)
	assert.False(t, s.submissionEnabled)

	repo.AssertExpectations(t)
	repo.AssertNumberOfCalls(t, "SaveNodeConfigRaw", 0)
}
