// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo

	Config config.ClaimerConfig

	EthConn    *ethclient.Client
	Repository repository.Repository
}

type Service struct {
	service.Service

	repository        iRepository
	blockchain        iBlockchain
	submissionEnabled bool
	txOpts            *bind.TransactOpts
	defaultBlock      model.DefaultBlock
}

const ClaimerConfigKey = "claimer"

type PersistentConfig struct {
	DefaultBlock           model.DefaultBlock
	ClaimSubmissionEnabled bool
	ChainID                uint64
}

func Create(ctx context.Context, c *CreateInfo) (*Service, error) {
	var err error

	if c == nil {
		return nil, errors.New("invalid CreateInfo is nil")
	}
	if err = ctx.Err(); err != nil {
		return nil, err // This returns context.Canceled or context.DeadlineExceeded.
	}
	if c.Repository == nil {
		return nil, fmt.Errorf("repository on claimer service Create is nil")
	}
	if c.EthConn == nil {
		return nil, fmt.Errorf("ethclient on claimer service Create is nil")
	}

	s := &Service{}
	c.Impl = s

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, err
	}

	nodeConfig, err := setupPersistentConfig(ctx, s.Logger, c.Repository, &c.Config)
	if err != nil {
		return nil, err
	}

	chainId, err := c.EthConn.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("chainId mismatch: network %d != provided %d", chainId.Uint64(), c.Config.BlockchainId)
	}

	if chainId.Uint64() != nodeConfig.ChainID {
		return nil, fmt.Errorf("NodeConfig chainId mismatch: network %d != config %d",
			chainId.Uint64(), nodeConfig.ChainID)
	}
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.defaultBlock = nodeConfig.DefaultBlock

	var txOpts *bind.TransactOpts = nil
	if s.submissionEnabled {
		txOpts, err = auth.GetTransactOpts(ctx, chainId)
		if err != nil {
			return nil, err
		}
	}

	s.repository = c.Repository
	s.blockchain = c.EthConn
	s.txOpts = txOpts

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

// NOTE: tick is not re-entrant!
func (s *Service) Tick() []error {
	nr, err := getDefaultBlockNumber(s.Context, s.blockchain, s.defaultBlock)
	if err != nil {
		return []error{err}
	}
	return update(s.Context, s.Logger, s.txOpts, s.repository, s.blockchain, nr)
}

func setupPersistentConfig(
	ctx context.Context,
	logger *slog.Logger,
	repo iRepository,
	c *config.ClaimerConfig,
) (*PersistentConfig, error) {
	config, err := repository.LoadNodeConfig[PersistentConfig](ctx, repo, ClaimerConfigKey)
	if config == nil && errors.Is(err, repository.ErrNotFound) {
		nc := model.NodeConfig[PersistentConfig]{
			Key: ClaimerConfigKey,
			Value: PersistentConfig{
				DefaultBlock:           c.BlockchainDefaultBlock,
				ClaimSubmissionEnabled: c.FeatureClaimSubmissionEnabled,
				ChainID:                c.BlockchainId,
			},
		}
		logger.Info("Initializing claimer persistent config", "config", nc.Value)
		err = repository.SaveNodeConfig(ctx, repo, &nc)
		if err != nil {
			return nil, err
		}
		return &nc.Value, nil
	} else if err == nil {
		logger.Info("Claimer was already configured. Using previous persistent config", "config", config.Value)
		return &config.Value, nil
	}

	logger.Error("Could not retrieve persistent config from Database.", "error", err)
	return nil, err
}
