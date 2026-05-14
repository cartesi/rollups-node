// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateUsesPersistedDefaultBlock(t *testing.T) {
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
			BlockchainDefaultBlock:        model.DefaultBlock_Finalized,
			BlockchainId:                  42,
			FeatureClaimSubmissionEnabled: true,
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
