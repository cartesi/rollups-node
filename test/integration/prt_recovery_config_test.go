// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func loadRecoveryFixtureConfig() (*config.EvmreaderConfig, *config.PrtConfig, error) {
	// These services run in-process, without telemetry listeners. Match the
	// combined node's initialization, including defaults not exported by make env.
	config.SetDefaults()
	node, err := config.LoadNodeConfig()
	if err != nil {
		return nil, nil, err
	}
	return node.ToEvmreaderConfig(), node.ToPrtConfig(), nil
}

// Run without endtoendtests: configuration must load before any database or L1
// access, with the same defaults and environment precedence as the node binary.
func TestPrtRecoveryFixtureConfig(t *testing.T) {
	viper.Reset()
	viper.AutomaticEnv()
	t.Cleanup(func() {
		viper.Reset()
		viper.AutomaticEnv()
		config.SetDefaults()
	})
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "CARTESI_") {
			t.Setenv(key, "")
		}
	}
	t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, "http://localhost:8545")
	t.Setenv(config.BLOCKCHAIN_ID, "31337")
	t.Setenv(config.DATABASE_CONNECTION, "postgres://localhost/unused")

	reader, prt, err := loadRecoveryFixtureConfig()
	require.NoError(t, err)
	require.Equal(t, model.DefaultBlock_Finalized, reader.BlockchainDefaultBlock)
	require.Equal(t, reader.BlockchainDefaultBlock, prt.BlockchainDefaultBlock)
	require.Equal(t, uint64(31337), reader.BlockchainId)
	require.Equal(t, reader.BlockchainId, prt.BlockchainId)
	require.Equal(t, 120*time.Second, prt.BlockchainHttpRequestTimeout)
	require.True(t, prt.FeatureClaimSubmissionEnabled)
	require.Empty(t, reader.EvmReaderTelemetryAddress, "in-process services do not bind telemetry listeners")
	require.Empty(t, prt.PrtTelemetryAddress)

	t.Setenv(config.BLOCKCHAIN_DEFAULT_BLOCK, "latest")
	t.Setenv(config.FEATURE_CLAIM_SUBMISSION_ENABLED, "false")
	t.Setenv(config.BLOCKCHAIN_HTTP_REQUEST_TIMEOUT, "45")
	reader, prt, err = loadRecoveryFixtureConfig()
	require.NoError(t, err)
	require.Equal(t, model.DefaultBlock_Latest, reader.BlockchainDefaultBlock)
	require.Equal(t, reader.BlockchainDefaultBlock, prt.BlockchainDefaultBlock)
	require.False(t, prt.FeatureClaimSubmissionEnabled)
	require.Equal(t, 45*time.Second, prt.BlockchainHttpRequestTimeout)
}
