// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/common"
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

	repository iclaimerRepository
	blockchain iclaimerBlockchain

	// submitClaim transactions waiting for confirmation from the blockchain.
	// Tick is the only caller, so this map does not need a lock.
	// Key: application ID. There is at most one entry per app.
	claimsInFlight map[int64]inFlightTx

	// acceptClaim transactions waiting for confirmation. This has the same map
	// shape as claimsInFlight. It is separate because one app can have a submit
	// transaction for a newer epoch and an accept transaction for an older one.
	acceptsInFlight map[int64]inFlightTx

	// acceptAttempts counts repeated acceptClaim attempts for one app and epoch.
	// When the count is greater than maxAcceptAttempts, the app is marked
	// FAILED. This prevents the node from spending gas forever on the same
	// failing claim. The map is in memory only; restart clears it.
	acceptAttempts map[acceptAttemptKey]uint64

	// maxAcceptAttempts limits the counter above. It comes from
	// CARTESI_CLAIMER_MAX_ACCEPT_ATTEMPTS. The default is 5.
	maxAcceptAttempts uint64

	// consensusAddressChecks memoizes consensus-address drift checks during one
	// Tick. An app can move through multiple claim stages in a single tick, and
	// each stage guards against consensus replacement. The underlying eth_call is
	// block-pinned, so one result per (app, block) is enough.
	consensusAddressChecks map[consensusAddressCheckKey]error

	submissionEnabled bool
	submissionTimeout time.Duration
}

// defaultMaxAcceptAttempts is used only when config is not supplied, mainly in
// tests. The real env var also defaults to 5.
const defaultMaxAcceptAttempts uint64 = 5

const ClaimerConfigKey = "claimer"

type PersistentConfig = config.PersistentSubmitterConfig

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
	c.EnableReschedule = true

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, fmt.Errorf("creating base service: %w", err)
	}

	chainID, err := c.EthConn.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying chain ID: %w", err)
	}
	if err := config.CheckNetworkChainID(chainID, c.Config.BlockchainId); err != nil {
		return nil, err
	}
	nodeConfig, err := setupPersistentConfig(ctx, s.Logger, c.Repository, &c.Config)
	if err != nil {
		return nil, fmt.Errorf("setting up persistent config: %w", err)
	}
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.claimsInFlight = map[int64]inFlightTx{}
	s.acceptsInFlight = map[int64]inFlightTx{}
	s.acceptAttempts = map[acceptAttemptKey]uint64{}
	s.maxAcceptAttempts = c.Config.ClaimerMaxAcceptAttempts
	if s.maxAcceptAttempts == 0 {
		s.maxAcceptAttempts = defaultMaxAcceptAttempts
	}

	var txOptsFactory ethutil.TransactOptsFactory
	if s.submissionEnabled {
		s.submissionTimeout = c.Config.BlockchainHttpRequestTimeout
		if s.submissionTimeout == 0 {
			return nil, fmt.Errorf("BlockchainHttpRequestTimeout must be different from zero")
		}
		txOptsFactory, err = auth.GetTransactOptsFactory(ctx, chainID)
		if err != nil {
			return nil, fmt.Errorf("getting transaction options: %w", err)
		}
		if c.Config.BlockchainLegacyEnabled {
			txOptsFactory = ethutil.WithLegacyFees(txOptsFactory, c.EthConn)
		}
		s.Logger.Info("Claim submitter identity", "address", txOptsFactory.From())
	}

	s.repository = c.Repository
	s.blockchain = &claimerBlockchain{
		logger:        s.Logger,
		client:        c.EthConn,
		txOptsFactory: txOptsFactory,
		defaultBlock:  nodeConfig.DefaultBlock,
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
	s.SetStopping()
	return nil
}

func (s *Service) String() string {
	return s.Name
}

// SubmitterAddress returns the configured claim submitter address.
func (s *Service) SubmitterAddress() (common.Address, bool) {
	if s.blockchain == nil {
		return common.Address{}, false
	}
	return s.blockchain.claimSubmitterAddress()
}

func setupPersistentConfig(
	ctx context.Context,
	logger *slog.Logger,
	repo iclaimerRepository,
	c *config.ClaimerConfig,
) (*PersistentConfig, error) {
	requested := PersistentConfig{
		DefaultBlock: c.BlockchainDefaultBlock, ChainID: c.BlockchainId,
		ClaimSubmissionEnabled: c.FeatureClaimSubmissionEnabled,
	}
	if err := requested.Validate(); err != nil {
		return nil, fmt.Errorf("invalid claimer config: %w", err)
	}
	config, err := repository.LoadNodeConfig[PersistentConfig](ctx, repo, ClaimerConfigKey)
	if config == nil && errors.Is(err, repository.ErrNotFound) {
		nc := model.NodeConfig[PersistentConfig]{
			Key:   ClaimerConfigKey,
			Value: requested,
		}
		logger.Info("Initializing claimer persistent config", "config", nc.Value)
		config, err = repository.InitializeNodeConfig(ctx, repo, &nc)
	}
	if err == nil {
		if err := config.Value.CheckRequested(requested); err != nil {
			return nil, fmt.Errorf("claimer persistent config: %w", err)
		}
		logger.Info("Claimer persistent config matches requested config", "config", config.Value)
		return &config.Value, nil
	}

	logger.Error("Could not retrieve persistent config from Database.", "error", err)
	return nil, err
}
