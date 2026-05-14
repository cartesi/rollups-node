// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/core/types"
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

func TestCreateWithNilEthClient(t *testing.T) {
	config.SetDefaults()
	logLevel, err := config.GetLogLevel()
	require.NoError(t, err)

	_, err = Create(context.Background(), &CreateInfo{
		CreateInfo: service.CreateInfo{Name: "evm-reader", LogLevel: logLevel},
	})
	require.ErrorContains(t, err, "EthClient on evmreader service Create is nil")
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
	s.Require().Equal(expected, header)
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
