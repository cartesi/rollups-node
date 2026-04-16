// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync/atomic"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	Config     config.EvmreaderConfig
	Logger     *slog.Logger
	EthClient  *ethclient.Client
	Repository EvmReaderRepository
}

type Service struct {
	service.TickServiceTemplate

	client             EthClientInterface
	adapterFactory     AdapterFactory
	resolver           *applicationAdapterResolver
	repository         EvmReaderRepository
	chainID            uint64
	defaultBlock       DefaultBlock
	hasEnabledApps     bool
	inputReaderEnabled bool
	lastBlockNumber    atomic.Uint64
}

const EvmReaderConfigKey = "evm-reader"

type PersistentConfig struct {
	DefaultBlock       DefaultBlock
	InputReaderEnabled bool
	ChainID            uint64
}

func Create(ctx context.Context, c *CreateInfo) (service.SupervisedService, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	tickCfg := &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     config.ServiceEvmReader,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		PollInterval: c.Config.EvmReaderPollingInterval,
	}
	err = service.InitTickServiceTemplate(&s.TickServiceTemplate, tickCfg, s)
	if err != nil {
		return nil, err
	}

	authOpt, err := config.HTTPAuthorizationOption()
	if err != nil {
		return nil, err
	}

	ethClient := c.EthClient
	if ethClient == nil {
		ethClient, err = ethutil.NewEthClient(ctx, c.Config.BlockchainHttpEndpoint.Raw(), s.Logger,
			ethutil.RetryConfig{
				MaxRetries:     c.Config.BlockchainHttpMaxRetries,
				RetryMinWait:   c.Config.BlockchainHttpRetryMinWait,
				RetryMaxWait:   c.Config.BlockchainHttpRetryMaxWait,
				RequestTimeout: c.Config.BlockchainHttpRequestTimeout,
			}, authOpt)
		if err != nil {
			return nil, err
		}
	}

	chainId, err := ethClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("EthClient chainId mismatch: network %d != provided %d",
			chainId.Uint64(), c.Config.BlockchainId)
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on evmreader service Create is nil")
	}

	nodeConfig, err := s.setupPersistentConfig(ctx, &c.Config)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != nodeConfig.ChainID {
		return nil, fmt.Errorf("NodeConfig chainId mismatch: network %d != config %d",
			chainId.Uint64(), nodeConfig.ChainID)
	}

	s.client = ethClient
	s.chainID = nodeConfig.ChainID
	s.defaultBlock = nodeConfig.DefaultBlock
	s.inputReaderEnabled = nodeConfig.InputReaderEnabled
	s.hasEnabledApps = true
	s.adapterFactory = &DefaultAdapterFactory{
		Client: ethClient,
		Filter: ethutil.Filter{
			MinChunkSize: ethutil.DefaultMinChunkSize,
			MaxChunkSize: new(big.Int).SetUint64(c.Config.BlockchainMaxBlockRange),
			Logger:       s.Logger,
		},
	}
	s.resolver = newApplicationAdapterResolver(s.Logger, s.adapterFactory)

	s.Logger.Info("Created", "config", c.Config)

	return s, nil
}

func (s *Service) setupPersistentConfig(
	ctx context.Context,
	c *config.EvmreaderConfig,
) (*PersistentConfig, error) {
	config, err := repository.LoadNodeConfig[PersistentConfig](ctx, s.repository, EvmReaderConfigKey)
	if config == nil && errors.Is(err, repository.ErrNotFound) {
		nc := NodeConfig[PersistentConfig]{
			Key: EvmReaderConfigKey,
			Value: PersistentConfig{
				DefaultBlock:       c.BlockchainDefaultBlock,
				InputReaderEnabled: c.FeatureInputReaderEnabled,
				ChainID:            c.BlockchainId,
			},
		}
		s.Logger.Info("Initializing evm-reader persistent config", "config", nc.Value)
		err = repository.SaveNodeConfig(ctx, s.repository, &nc)
		if err != nil {
			return nil, err
		}
		return &nc.Value, nil
	} else if err == nil {
		s.Logger.Info("Evm-reader was already configured. Using previous persistent config", "config", config.Value)
		return &config.Value, nil
	}

	s.Logger.Error("Could not retrieve persistent config from database", "error", err)
	return nil, err
}
