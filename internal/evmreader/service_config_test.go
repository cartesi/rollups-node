// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Create() entry point tests ---

func TestCreateWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, &CreateInfo{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestCreateAcceptsRequestTimeoutBelowPollingInterval(t *testing.T) {
	config.SetDefaults()
	logLevel, err := config.GetLogLevel()
	require.NoError(t, err)

	const chainID = uint64(1)
	pollInterval := 12 * time.Second
	requestTimeout := 5 * time.Second

	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer rpcServer.Close()

	client, err := ethclient.Dial(rpcServer.URL)
	require.NoError(t, err)
	defer client.Close()

	rawConfig, err := json.Marshal(PersistentConfig{
		DefaultBlock:       DefaultBlock_Finalized,
		InputReaderEnabled: true,
		ChainID:            chainID,
	})
	require.NoError(t, err)

	repo := newMockRepository()
	repo.On("LoadNodeConfigRaw", mock.Anything, EvmReaderConfigKey).
		Return(rawConfig, time.Now(), time.Now(), nil).Once()

	_, err = Create(t.Context(), &CreateInfo{
		Config: config.EvmreaderConfig{
			LogLevel:                     logLevel,
			BlockchainDefaultBlock:       DefaultBlock_Finalized,
			BlockchainHttpRequestTimeout: requestTimeout,
			BlockchainId:                 chainID,
			EvmReaderPollingInterval:     pollInterval,
			FeatureInputReaderEnabled:    true,
		},
		EthClient:  client,
		Repository: repo,
	})
	require.NoError(t, err)

	repo.AssertExpectations(t)
}

// --- fetchMostRecentHeader tests ---

func (s *EvmReaderSuite) TestFetchMostRecentHeaderRPCError() {
	s.client.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return((*types.Header)(nil), errors.New("RPC connection timeout"))

	_, err := s.evmReader.fetchMostRecentHeader(s.ctx, DefaultBlock_Finalized)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "failed to retrieve header")
	s.Require().ErrorContains(err, "RPC connection timeout")
}

func (s *EvmReaderSuite) TestFetchMostRecentHeaderNilHeader() {
	s.client.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return((*types.Header)(nil), nil)

	_, err := s.evmReader.fetchMostRecentHeader(s.ctx, DefaultBlock_Finalized)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "returned header is nil")
}

func (s *EvmReaderSuite) TestFetchMostRecentHeaderUnsupportedBlock() {
	_, err := s.evmReader.fetchMostRecentHeader(s.ctx, DefaultBlock("INVALID"))
	s.Require().Error(err)
	s.Require().ErrorContains(err, "not supported")
}

func (s *EvmReaderSuite) TestFetchMostRecentHeaderSuccess() {
	expected := &types.Header{Number: big.NewInt(42)}
	s.client.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(expected, nil)

	header, err := s.evmReader.fetchMostRecentHeader(s.ctx, DefaultBlock_Finalized)
	s.Require().NoError(err)
	s.Require().Equal(expected.Number.Uint64(), header)
}

// --- inputReaderEnabled feature flag tests ---

func (s *EvmReaderSuite) TestInputReaderDisabledSkipsInputChecks() {
	s.evmReader.inputReaderEnabled = false

	app := &Application{
		Name:                "test-app",
		IApplicationAddress: app1Addr,
		IInputBoxAddress:    inputBoxAddr,
		DataAvailability:    DataAvailability_InputBox[:],
		EpochLength:         10,
		LastInputCheckBlock: 100,
	}
	apps := []appContracts{{application: app}}

	repo := newMockRepository()
	s.evmReader.repository = repo

	s.evmReader.scanIConsensusInputs(s.ctx, apps, 200)

	repo.AssertNumberOfCalls(s.T(), "GetNumberOfInputs", 0)
	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
	repo.AssertNumberOfCalls(s.T(), "GetEpoch", 0)
}

func (s *EvmReaderSuite) TestInputReaderDisabledSkipsEpochChecks() {
	s.evmReader.inputReaderEnabled = false

	apps := []appContracts{{
		application: &Application{
			Name:                "test-app",
			IApplicationAddress: app1Addr,
			IConsensusAddress:   consensusAddr,
		},
	}}

	repo := newMockRepository()
	s.evmReader.repository = repo

	s.evmReader.scanDaveConsensusEpochsAndInputs(s.ctx, apps, 200)

	repo.AssertNumberOfCalls(s.T(), "GetLastNonOpenEpoch", 0)
	repo.AssertNumberOfCalls(s.T(), "CreateEpochsAndInputs", 0)
}

// --- setupPersistentConfig tests ---

func (s *EvmReaderSuite) TestSetupPersistentConfigFirstRun() {
	repo := newMockRepository()
	repo.On("LoadNodeConfigRaw", mock.Anything, EvmReaderConfigKey).
		Return(([]byte)(nil), time.Time{}, time.Time{}, repository.ErrNotFound)
	repo.On("SaveNodeConfigRaw", mock.Anything, EvmReaderConfigKey, mock.Anything).
		Return(nil)

	s.evmReader.repository = repo

	cfg := &config.EvmreaderConfig{
		BlockchainDefaultBlock:    DefaultBlock_Finalized,
		FeatureInputReaderEnabled: true,
		BlockchainId:              42,
	}

	result, err := s.evmReader.setupPersistentConfig(s.ctx, cfg)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(DefaultBlock_Finalized, result.DefaultBlock)
	s.Require().True(result.InputReaderEnabled)
	s.Require().Equal(uint64(42), result.ChainID)

	repo.AssertNumberOfCalls(s.T(), "SaveNodeConfigRaw", 1)
}

func (s *EvmReaderSuite) TestSetupPersistentConfigExistingConfigWins() {
	existingJSON, err := json.Marshal(PersistentConfig{
		DefaultBlock:       DefaultBlock_Safe,
		InputReaderEnabled: false,
		ChainID:            99,
	})
	s.Require().NoError(err)

	repo := newMockRepository()
	repo.On("LoadNodeConfigRaw", mock.Anything, EvmReaderConfigKey).
		Return(existingJSON, time.Now(), time.Now(), nil)

	s.evmReader.repository = repo

	// Env config has DIFFERENT values — should be ignored
	cfg := &config.EvmreaderConfig{
		BlockchainDefaultBlock:    DefaultBlock_Latest,
		FeatureInputReaderEnabled: true,
		BlockchainId:              1,
	}

	result, err := s.evmReader.setupPersistentConfig(s.ctx, cfg)
	s.Require().NoError(err)

	// Existing config wins
	s.Require().Equal(DefaultBlock_Safe, result.DefaultBlock)
	s.Require().False(result.InputReaderEnabled)
	s.Require().Equal(uint64(99), result.ChainID)

	// SaveNodeConfigRaw must NOT be called
	repo.AssertNumberOfCalls(s.T(), "SaveNodeConfigRaw", 0)
}

func (s *EvmReaderSuite) TestSetupPersistentConfigDBError() {
	repo := newMockRepository()
	repo.On("LoadNodeConfigRaw", mock.Anything, EvmReaderConfigKey).
		Return(([]byte)(nil), time.Time{}, time.Time{}, errors.New("database unreachable"))

	s.evmReader.repository = repo

	_, err := s.evmReader.setupPersistentConfig(s.ctx, &config.EvmreaderConfig{})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "database unreachable")
}

func TestReadinessBudget(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  config.EvmreaderConfig
		want time.Duration
	}{
		{"floor", config.EvmreaderConfig{}, time.Second},
		{"poll interval", config.EvmreaderConfig{EvmReaderPollingInterval: 12 * time.Second}, 36 * time.Second},
		{"requests with retries", config.EvmreaderConfig{EvmReaderPollingInterval: 12 * time.Second, BlockchainHttpRequestTimeout: 120 * time.Second, BlockchainHttpMaxRetries: 3}, 480 * time.Second},
		{"retry backoff ignored", config.EvmreaderConfig{BlockchainHttpRetryMaxWait: time.Hour}, time.Second},
		{"explicit override", config.EvmreaderConfig{EvmReaderReadyMaxStaleness: 5 * time.Second, EvmReaderPollingInterval: time.Minute}, 5 * time.Second},
		{"overflow saturates", config.EvmreaderConfig{BlockchainHttpRequestTimeout: time.Second, BlockchainHttpMaxRetries: ^uint64(0)}, time.Duration(1<<63 - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget, err := readinessBudget(&tc.cfg)
			require.NoError(t, err)
			require.Equal(t, tc.want, budget)
		})
	}
}

func TestCreateRejectsNegativeReadinessBudget(t *testing.T) {
	_, err := Create(t.Context(), &CreateInfo{Config: config.EvmreaderConfig{
		EvmReaderReadyMaxStaleness: -time.Second,
	}})
	require.ErrorContains(t, err, "CARTESI_EVM_READER_READY_MAX_STALENESS must be non-negative")
}

func (s *EvmReaderSuite) TestReadinessRefreshesAfterScan() {
	s.client.EnqueueNewHead(100).Once()
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).Unset()
	var scanFinished time.Time
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, false).
		Return([]*Application{}, uint64(0), nil).Once().Run(func(mock.Arguments) {
		s.Require().False(s.evmReader.Ready(), "fetching a header must not mark an unfinished scan ready")
		scanFinished = time.Now()
	})
	_, err := s.evmReader.Tick(s.ctx)
	s.Require().NoError(err)
	s.Require().False(s.evmReader.lastSuccessfulPoll.Load().Before(scanFinished))
	s.Require().True(s.evmReader.Ready())

	stale := time.Now().Add(-s.evmReader.readyMaxStaleness)
	s.evmReader.lastSuccessfulPoll.Store(&stale)
	s.Require().False(s.evmReader.Ready())
	s.client.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return((*types.Header)(nil), errors.New("unavailable")).Once()
	_, err = s.evmReader.Tick(s.ctx)
	s.Require().Error(err)
	s.Require().Equal(stale, *s.evmReader.lastSuccessfulPoll.Load())
	s.Require().False(s.evmReader.Ready())
}
