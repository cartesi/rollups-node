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
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	Config         config.EvmreaderConfig
	Logger         *slog.Logger
	Repository     EvmReaderRepository
	EthClient      EthClientInterface
	AdapterFactory AdapterFactory
}

type Service struct {
	service.TickServiceTemplate

	client                  EthClientInterface
	adapterFactory          AdapterFactory
	resolver                *applicationAdapterResolver
	repository              EvmReaderRepository
	chainID                 uint64
	defaultBlock            DefaultBlock
	hasEnabledApps          bool
	inputReaderEnabled      bool
	lastBlockNumber         atomic.Uint64
	lastSuccessfulPoll      atomic.Pointer[time.Time]
	consecutiveScanFailures atomic.Uint32
	readyMaxStaleness       time.Duration
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

	readyMaxStaleness, err := readinessBudget(&c.Config)
	if err != nil {
		return nil, err
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

	if c.AdapterFactory != nil {
		s.adapterFactory = c.AdapterFactory
	} else {
		fullEthClient, ok := ethClient.(*ethclient.Client)
		if !ok {
			return nil, fmt.Errorf("EthClient must be *ethclient.Client when AdapterFactory is not provided")
		}
		s.adapterFactory = &DefaultAdapterFactory{
			Client: fullEthClient,
			Filter: ethutil.Filter{
				MinChunkSize: ethutil.DefaultMinChunkSize,
				MaxChunkSize: new(big.Int).SetUint64(c.Config.BlockchainMaxBlockRange),
				Logger:       s.Logger,
			},
		}
	}

	s.resolver = newApplicationAdapterResolver(s.Logger, s.adapterFactory)
	s.readyMaxStaleness = readyMaxStaleness
	s.lastSuccessfulPoll.Store(&time.Time{})

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

// readinessBudget is independent of the HTTP retry-backoff cap.
func readinessBudget(c *config.EvmreaderConfig) (time.Duration, error) {
	if c.EvmReaderReadyMaxStaleness < 0 {
		return 0, fmt.Errorf("CARTESI_EVM_READER_READY_MAX_STALENESS must be non-negative")
	}
	if c.EvmReaderReadyMaxStaleness > 0 {
		return c.EvmReaderReadyMaxStaleness, nil
	}

	// Saturate large configured durations instead of overflowing to a small budget.
	multiply := func(d time.Duration, n uint64) time.Duration {
		const limit = time.Duration(1<<63 - 1)
		if d <= 0 {
			return 0
		}
		if n > uint64(limit/d) {
			return limit
		}
		return d * time.Duration(n)
	}
	requestBudget := multiply(c.BlockchainHttpRequestTimeout, c.BlockchainHttpMaxRetries)
	const limit = time.Duration(1<<63 - 1)
	if c.BlockchainHttpRequestTimeout > 0 {
		requestBudget += min(c.BlockchainHttpRequestTimeout, limit-requestBudget)
	}
	return max(time.Second, multiply(c.EvmReaderPollingInterval, 3), requestBudget), nil
}
