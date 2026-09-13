// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
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
	service.CreateInfo

	Config config.EvmreaderConfig

	Repository EvmReaderRepository

	EthClient *ethclient.Client
}

type Service struct {
	service.Service

	client          EthClientInterface
	adapterFactory  AdapterFactory
	resolver        *applicationAdapterResolver
	repository      EvmReaderRepository
	chainID         uint64
	defaultBlock    DefaultBlock
	hasEnabledApps  bool
	lastBlockNumber atomic.Uint64
	alive           atomic.Bool
	ready           atomic.Bool
}

const EvmReaderConfigKey = "evm-reader"

type PersistentConfig = config.PersistentChainConfig

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
	if err := config.CheckNetworkChainID(chainId, c.Config.BlockchainId); err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on evmreader service Create is nil")
	}

	nodeConfig, err := s.setupPersistentConfig(ctx, &c.Config)
	if err != nil {
		return nil, err
	}

	s.client = c.EthClient

	s.chainID = nodeConfig.ChainID
	s.defaultBlock = nodeConfig.DefaultBlock
	s.hasEnabledApps = true
	s.adapterFactory = &DefaultAdapterFactory{
		Client: c.EthClient,
		Filter: ethutil.Filter{
			MinChunkSize: ethutil.DefaultMinChunkSize,
			MaxChunkSize: new(big.Int).SetUint64(c.Config.BlockchainMaxBlockRange),
			Logger:       s.Logger,
		},
	}
	s.resolver = newApplicationAdapterResolver(s.Logger, s.adapterFactory)

	return s, nil
}

func (s *Service) Alive() bool {
	return s.alive.Load()
}

func (s *Service) Ready() bool {
	return s.ready.Load()
}

func (s *Service) Reload() []error {
	return nil
}

func (s *Service) Stop(bool) []error {
	s.SetStopping()
	return nil
}

func (s *Service) Serve() error {
	s.alive.Store(true)
	s.ready.Store(true)
	defer s.alive.Store(false)
	defer s.ready.Store(false)
	return s.Service.Serve()
}

func (s *Service) String() string {
	return s.Name
}

func (s *Service) setupPersistentConfig(
	ctx context.Context,
	c *config.EvmreaderConfig,
) (*PersistentConfig, error) {
	requested := PersistentConfig{DefaultBlock: c.BlockchainDefaultBlock, ChainID: c.BlockchainId}
	if err := requested.Validate(); err != nil {
		return nil, fmt.Errorf("invalid evm-reader config: %w", err)
	}
	config, err := repository.LoadNodeConfig[PersistentConfig](ctx, s.repository, EvmReaderConfigKey)
	if config == nil && errors.Is(err, repository.ErrNotFound) {
		nc := NodeConfig[PersistentConfig]{
			Key:   EvmReaderConfigKey,
			Value: requested,
		}
		s.Logger.Info("Initializing evm-reader persistent config", "config", nc.Value)
		config, err = repository.InitializeNodeConfig(ctx, s.repository, &nc)
	}
	if err == nil {
		if err := config.Value.CheckRequested(requested); err != nil {
			return nil, fmt.Errorf("evm-reader persistent config: %w", err)
		}
		s.Logger.Info("Evm-reader persistent config matches requested config", "config", config.Value)
		return &config.Value, nil
	}

	s.Logger.Error("Could not retrieve persistent config from database", "error", err)
	return nil, err
}
