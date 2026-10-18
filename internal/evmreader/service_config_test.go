// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
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

	requested := PersistentConfig{DefaultBlock: DefaultBlock_Finalized, ChainID: chainID}
	repo := newMockRepository()
	repo.On("RegisterObserver", mock.Anything, EvmReaderConfigKey, requested).
		Return(requested, nil).Once()

	_, err = Create(t.Context(), &CreateInfo{
		Config: config.EvmreaderConfig{
			LogLevel:                     logLevel,
			BlockchainDefaultBlock:       DefaultBlock_Finalized,
			BlockchainHttpRequestTimeout: requestTimeout,
			BlockchainId:                 chainID,
			EvmReaderPollingInterval:     pollInterval,
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

// --- setupPersistentConfig tests ---

func (s *EvmReaderSuite) TestSetupPersistentConfigFirstRun() {
	requested := PersistentConfig{DefaultBlock: DefaultBlock_Finalized, ChainID: 42}
	repo := newMockRepository()
	repo.On("RegisterObserver", mock.Anything, EvmReaderConfigKey, requested).
		Return(requested, nil).Once()
	s.evmReader.repository = repo
	result, err := s.evmReader.setupPersistentConfig(s.ctx, &config.EvmreaderConfig{
		BlockchainDefaultBlock: DefaultBlock_Finalized, BlockchainId: 42,
	})
	s.Require().NoError(err)
	s.Require().Equal(requested, *result)
	repo.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestSetupPersistentConfigRejectsConcurrentInitializer() {
	// The repository serializes registration and detects a different winner.
	errConflict := errors.New("observation policy mismatch: database=LATEST, configured=FINALIZED")
	repo := newMockRepository()
	repo.On("RegisterObserver", mock.Anything, EvmReaderConfigKey, mock.Anything).
		Return(PersistentConfig{}, errConflict).Once()
	s.evmReader.repository = repo
	result, err := s.evmReader.setupPersistentConfig(s.ctx, &config.EvmreaderConfig{
		BlockchainId: 42, BlockchainDefaultBlock: DefaultBlock_Finalized,
	})
	s.Require().Nil(result)
	s.Require().ErrorIs(err, errConflict)
	repo.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestSetupPersistentConfigPropagatesSavedFieldErrors() {
	// Decoding and persistent validation live inside the coordinated repository
	// operation. Its concrete behavior has PostgreSQL registration fixtures.
	for _, message := range []string{
		"persistent config requires non-null DefaultBlock and ChainID",
		"ChainID must be greater than zero",
		"invalid DefaultBlock",
		"observation policy mismatch: database=LATEST, configured=FINALIZED",
	} {
		s.Run(message, func() {
			errSaved := errors.New(message)
			repo := newMockRepository()
			repo.On("RegisterObserver", mock.Anything, EvmReaderConfigKey, mock.Anything).
				Return(PersistentConfig{}, errSaved).Once()
			s.evmReader.repository = repo
			result, err := s.evmReader.setupPersistentConfig(s.ctx, &config.EvmreaderConfig{
				BlockchainId: 42, BlockchainDefaultBlock: DefaultBlock_Finalized,
			})
			s.Require().Nil(result)
			s.Require().ErrorIs(err, errSaved)
			repo.AssertExpectations(s.T())
		})
	}
}

func (s *EvmReaderSuite) TestSetupPersistentConfigRejectsDifferentConfig() {
	errMismatch := errors.New("chain ID mismatch: database=99, configured=1")
	repo := newMockRepository()
	repo.On("RegisterObserver", mock.Anything, EvmReaderConfigKey, mock.Anything).
		Return(PersistentConfig{}, errMismatch).Once()
	s.evmReader.repository = repo
	result, err := s.evmReader.setupPersistentConfig(s.ctx, &config.EvmreaderConfig{
		BlockchainDefaultBlock: DefaultBlock_Latest, BlockchainId: 1,
	})
	s.Require().Nil(result)
	s.Require().ErrorIs(err, errMismatch)
	repo.AssertExpectations(s.T())
}

func (s *EvmReaderSuite) TestSetupPersistentConfigDBError() {
	errDB := errors.New("database unreachable")
	repo := newMockRepository()
	repo.On("RegisterObserver", mock.Anything, EvmReaderConfigKey, mock.Anything).
		Return(PersistentConfig{}, errDB).Once()
	s.evmReader.repository = repo
	_, err := s.evmReader.setupPersistentConfig(s.ctx, &config.EvmreaderConfig{
		BlockchainId: 42, BlockchainDefaultBlock: DefaultBlock_Finalized,
	})
	s.Require().ErrorIs(err, errDB)
	repo.AssertExpectations(s.T())
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
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything).Unset()
	var scanFinished time.Time
	s.repository.On("ListApplications", mock.Anything, mock.Anything, mock.Anything).
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
