// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
)

type CreateInfo struct {
	service.CreateInfo
	Config     config.PrtConfig
	Repository repository.Repository
	EthClient  EthClientInterface
}

type Service struct {
	service.Service
	repository        PrtRepository
	client            EthClientInterface
	submissionEnabled bool
}

const PrtConfigKey = "prt"

type PersistentConfig struct {
	DefaultBlock           DefaultBlock
	ClaimSubmissionEnabled bool
	ChainID                uint64
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
		return nil, fmt.Errorf("EthClient on prt service Create is nil")
	}
	chainId, err := c.EthClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("EthClient chainId mismatch: network %d != provided %d",
			chainId.Uint64(), c.Config.BlockchainId)
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on prt service Create is nil")
	}

	nodeConfig, err := s.setupPersistentConfig(ctx, &c.Config)
	if err != nil {
		return nil, err
	}
	if chainId.Uint64() != nodeConfig.ChainID {
		return nil, fmt.Errorf("NodeConfig chainId mismatch: network %d != config %d",
			chainId.Uint64(), nodeConfig.ChainID)
	}

	s.client = c.EthClient
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled

	return s, nil
}

func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
func (s *Service) Reload() []error { return nil }

// Tick executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all running applications.
func (s *Service) Tick() []error {
	apps, _, err := getAllRunningApplications(s.Context, s.repository)
	if err != nil {
		return []error{fmt.Errorf("failed to get running applications. %w", err)}
	}

	// validate each application
	errs := []error{}
	for idx := range apps {
		if err := s.validateApplication(s.Context, apps[idx]); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
func (s *Service) Stop(b bool) []error {
	return nil
}

func (s *Service) String() string {
	return s.Name
}

func (s *Service) setupPersistentConfig(
	ctx context.Context,
	c *config.PrtConfig,
) (*PersistentConfig, error) {
	config, err := repository.LoadNodeConfig[PersistentConfig](ctx, s.repository, PrtConfigKey)
	if config == nil && errors.Is(err, repository.ErrNotFound) {
		nc := NodeConfig[PersistentConfig]{
			Key: PrtConfigKey,
			Value: PersistentConfig{
				DefaultBlock:           c.BlockchainDefaultBlock,
				ClaimSubmissionEnabled: c.FeatureClaimSubmissionEnabled,
				ChainID:                c.BlockchainId,
			},
		}
		s.Logger.Info("Initializing PRT persistent config", "config", nc.Value)
		err = repository.SaveNodeConfig(ctx, s.repository, &nc)
		if err != nil {
			return nil, err
		}
		return &nc.Value, nil
	} else if err == nil {
		s.Logger.Info("PRT service was already configured. Using previous persistent config", "config", config.Value)
		return &config.Value, nil
	}

	s.Logger.Error("Could not retrieve persistent config from Database. %w", "error", err)
	return nil, err
}
