// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
)

type CreateInfo struct {
	service.CreateInfo

	Config config.EvmreaderConfig

	Repository repository.Repository

	EthClient   EthClientInterface
	EthWsClient EthClientInterface
}

type Service struct {
	service.Service

	client                              EthClientInterface
	wsClient                            EthClientInterface
	adapterFactory                      AdapterFactory
	repository                          EvmReaderRepository
	chainId                             uint64
	defaultBlock                        DefaultBlock
	hasEnabledApps                      bool
	inputReaderEnabled                  bool
	blockchainMaxRetries                uint64
	blockchainSubscriptionRetryInterval time.Duration
	wsLivenessTimeout                   time.Duration
}

const EvmReaderConfigKey = "evm-reader"

type PersistentConfig struct {
	DefaultBlock       DefaultBlock
	InputReaderEnabled bool
	ChainID            uint64
}

func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, err
	}

	if c.EthClient == nil {
		return nil, fmt.Errorf("EthClient on evmreader service Create is nil")
	}
	chainId, err := c.EthClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("EthClient chainId mismatch: network %d != provided %d",
			chainId.Uint64(), c.Config.BlockchainId)
	}

	if c.EthWsClient == nil {
		return nil, fmt.Errorf("EthWsClient on evmreader service Create is nil")
	}
	chainId, err = c.EthWsClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("EthWsClient chainId mismatch: network %d != provided %d",
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
	s.blockchainMaxRetries = c.Config.BlockchainHttpMaxRetries
	s.blockchainSubscriptionRetryInterval = c.Config.BlockchainHttpRetryMinWait
	s.wsLivenessTimeout = c.Config.BlockchainWsLivenessTimeout

	s.client = c.EthClient
	s.wsClient = c.EthWsClient

	s.chainId = nodeConfig.ChainID
	s.defaultBlock = nodeConfig.DefaultBlock
	s.inputReaderEnabled = nodeConfig.InputReaderEnabled
	s.hasEnabledApps = true
	s.adapterFactory = &DefaultAdapterFactory{
		Filter: ethutil.Filter{
			MinChunkSize: ethutil.DefaultMinChunkSize,
			MaxChunkSize: new(big.Int).SetUint64(c.Config.BlockchainMaxBlockRange),
			Logger:       s.Logger,
		},
	}

	return s, nil
}

func (s *Service) Alive() bool {
	return true
}

func (s *Service) Ready() bool {
	return true
}

func (s *Service) Reload() []error {
	return nil
}

func (s *Service) Stop(bool) []error {
	return nil
}

func (s *Service) Tick() []error {
	return []error{}
}

func (s *Service) Serve() error {
	ready := make(chan struct{}, 1)
	go func() {
		s.Run(s.Context, ready)
		s.Service.Stop(false)
	}()
	return s.Service.Serve()
}

func (s *Service) String() string {
	return s.Name
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
