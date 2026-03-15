// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/internal/events"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo
	Config         config.PrtConfig
	Repository     repository.Repository
	EthClient      EthClientInterface
	AdapterFactory AdapterFactory

	// Publisher sends advisory event notifications after DB writes.
	// Defaults to events.NopPublisher{} if nil.
	Publisher events.Publisher
}

type Service struct {
	service.Service
	repository        prtRepository
	publisher         events.Publisher
	client            EthClientInterface
	adapterFactory    AdapterFactory
	submissionEnabled bool
	filter            ethutil.Filter
	txOpts            *bind.TransactOpts
	currentEpochIndex map[int64]uint64       // application.ID -> epochIndex
	settleInFlight    map[int64]*common.Hash // application.ID -> txHash
	joinInFlight      map[int64]*common.Hash // application.ID -> txHash
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
	chainID, err := c.EthClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if chainID.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("EthClient chainId mismatch: network %d != provided %d",
			chainID.Uint64(), c.Config.BlockchainId)
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on prt service Create is nil")
	}

	s.publisher = c.Publisher
	if s.publisher == nil {
		s.publisher = events.NopPublisher{}
	}

	nodeConfig, err := s.setupPersistentConfig(ctx, &c.Config)
	if err != nil {
		return nil, err
	}
	if chainID.Uint64() != nodeConfig.ChainID {
		return nil, fmt.Errorf("NodeConfig chainId mismatch: network %d != config %d",
			chainID.Uint64(), nodeConfig.ChainID)
	}

	s.client = c.EthClient
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.filter = ethutil.Filter{
		MinChunkSize: ethutil.DefaultMinChunkSize,
		MaxChunkSize: new(big.Int).SetUint64(c.Config.BlockchainMaxBlockRange),
		Logger:       s.Logger,
	}

	if c.AdapterFactory != nil {
		s.adapterFactory = c.AdapterFactory
	} else {
		ethClient, ok := c.EthClient.(*ethclient.Client)
		if !ok {
			return nil, fmt.Errorf("EthClient must be *ethclient.Client when AdapterFactory is not provided")
		}
		s.adapterFactory = NewDefaultAdapterFactory(ethClient, s.filter)
	}

	s.currentEpochIndex = map[int64]uint64{}
	s.settleInFlight = map[int64]*common.Hash{}
	s.joinInFlight = map[int64]*common.Hash{}

	if s.submissionEnabled {
		s.txOpts, err = auth.GetTransactOpts(ctx, chainID)
		if err != nil {
			return nil, err
		}
	}

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

	// Clean up per-app state for apps no longer in the active set.
	activeIDs := make(map[int64]struct{}, len(apps))
	for _, app := range apps {
		activeIDs[app.ID] = struct{}{}
	}
	for appID := range s.currentEpochIndex {
		if _, ok := activeIDs[appID]; !ok {
			s.Logger.Info("Cleaning PRT state for inactive app",
				"app_id", appID)
			delete(s.currentEpochIndex, appID)
			delete(s.settleInFlight, appID)
			delete(s.joinInFlight, appID)
		}
	}
	// Also clean settle/join maps for IDs not in currentEpochIndex
	// (defensive: covers keys present only in these maps).
	for appID := range s.settleInFlight {
		if _, ok := activeIDs[appID]; !ok {
			delete(s.settleInFlight, appID)
		}
	}
	for appID := range s.joinInFlight {
		if _, ok := activeIDs[appID]; !ok {
			delete(s.joinInFlight, appID)
		}
	}

	// validate each application
	errs := []error{}
	for idx := range apps {
		if s.Context.Err() != nil {
			return errs
		}
		if err := s.validateApplication(s.Context, apps[idx]); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (s *Service) Stop(_ bool) []error {
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

	s.Logger.Error("could not retrieve persistent config from database", "error", err)
	return nil, err
}
