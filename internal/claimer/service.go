// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	Config              config.ClaimerConfig
	Logger              *slog.Logger
	EthConn             *ethclient.Client
	Repository          repository.Repository
	TransactOptsFactory ethutil.TransactOptsFactory
}

type Service struct {
	service.TickServiceTemplate

	repository iclaimerRepository
	blockchain iclaimerBlockchain

	// publisher owns all volatile transaction state. PostgreSQL is the only
	// handoff to the observer's independent domain traversal.
	publisher           *claimPublisher
	pageCursors         [3]repository.ClaimCursor
	foreclosePageCursor int64

	// consensusAddressChecks memoizes consensus-address drift checks during one
	// Tick. An app can move through multiple claim stages in a single tick, and
	// each stage guards against consensus replacement. The underlying eth_call is
	// block-pinned, so one result per (app, block) is enough.
	consensusAddressChecks map[consensusAddressCheckKey]error

	submissionEnabled       bool
	defaultBlock            model.DefaultBlock
	observationHardWarnedAt time.Time
	failure                 atomic.Pointer[error]
	publicationReady        atomic.Bool
	deliveryRepository      repository.DeliveryRepository
	signer                  common.Address
}

const ClaimerConfigKey = "claimer"

type PersistentConfig = config.PersistentSubmitterConfig

func Create(ctx context.Context, c *CreateInfo) (service.SupervisedService, error) {
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

	s := &Service{}
	tickCfg := &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     config.ServiceClaimer,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		PollInterval: c.Config.ClaimerPollingInterval,
	}
	err = service.InitTickServiceTemplate(&s.TickServiceTemplate, tickCfg, s)
	if err != nil {
		return nil, err
	}

	authOpt, err := config.HTTPAuthorizationOption()
	if err != nil {
		return nil, err
	}

	ethClient := c.EthConn
	if ethClient == nil {
		//nolint:forbidigo // Protected Ethereum connection.
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

	chainID, err := ethClient.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying chain ID: %w", err)
	}
	if err := config.CheckNetworkChainID(chainID, c.Config.BlockchainId); err != nil {
		return nil, err
	}
	var txOptsFactory ethutil.TransactOptsFactory
	if c.Config.FeatureClaimSubmissionEnabled {
		if c.Config.BlockchainHttpRequestTimeout <= 0 {
			return nil, fmt.Errorf("BlockchainHttpRequestTimeout must be positive")
		}
		txOptsFactory = c.TransactOptsFactory
		if txOptsFactory == nil {
			txOptsFactory, err = auth.GetTransactOptsFactory(ctx, chainID)
			if err != nil {
				return nil, fmt.Errorf("getting transaction options: %w", err)
			}
		}
		s.signer = txOptsFactory.From()
		s.Logger.Info("Claim submitter identity", "address", s.signer)
	}
	nodeConfig, err := setupPersistentConfig(ctx, s.Logger, c.Repository, &c.Config, s.signer)
	if err != nil {
		return nil, fmt.Errorf("setting up persistent config: %w", err)
	}
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.defaultBlock = nodeConfig.DefaultBlock
	if nodeConfig.DefaultBlock == model.DefaultBlock_Latest || nodeConfig.DefaultBlock == model.DefaultBlock_Safe {
		s.Logger.Warn("Claim observation stores provisional chain facts; reorg rollback is unsupported",
			"default_block", nodeConfig.DefaultBlock)
	}
	s.deliveryRepository = c.Repository
	backend := txsender.ProtectedClient{Client: ethClient}
	if s.submissionEnabled {
		policy, policyErr := txsender.NewPolicy(txsender.FinancialConfig{
			MaxFeePerGas:   c.Config.BlockchainTxMaxFeePerGas,
			Legacy:         c.Config.BlockchainLegacyEnabled,
			SigningTimeout: c.Config.BlockchainHttpRequestTimeout,
		})
		if policyErr != nil {
			return nil, policyErr
		}
		publisher := &claimPublisher{
			repository: c.Repository, lookup: c.Repository, backend: backend,
			signer: s.signer, Logger: s.Logger, interval: c.Config.ClaimerPollingInterval,
			failure: &s.failure, cooldowns: map[claimAction]time.Time{},
		}
		publisher.sender, err = txsender.New[claimAction](backend, txOptsFactory, chainID.Uint64(), policy,
			s.guardClaimPublication, s.Logger)
		if err != nil {
			return nil, err
		}
		s.publisher = publisher
	}

	s.repository = c.Repository
	s.blockchain = &claimerBlockchain{
		logger:        s.Logger,
		client:        ethClient,
		txOptsFactory: txOptsFactory,
		defaultBlock:  nodeConfig.DefaultBlock,
		backend:       backend,
	}

	s.Logger.Info("Created", "config", c.Config)

	return s, nil
}

func setupPersistentConfig(
	ctx context.Context,
	logger *slog.Logger,
	repo repository.DeliveryRepository,
	c *config.ClaimerConfig,
	signer common.Address,
) (*PersistentConfig, error) {
	requested := PersistentConfig{
		DefaultBlock: c.BlockchainDefaultBlock, ChainID: c.BlockchainId,
		ClaimSubmissionEnabled: c.FeatureClaimSubmissionEnabled,
		ResolvedSignerAddress:  signer,
	}
	saved, err := repo.RegisterSubmitter(ctx, ClaimerConfigKey, requested)
	if err != nil {
		return nil, err
	}
	logger.Info("Claimer persistent config matches requested config", "config", saved)
	return &saved, nil
}
