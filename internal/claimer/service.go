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
	"github.com/cartesi/rollups-node/internal/events"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo

	Config config.ClaimerConfig

	EthConn    *ethclient.Client
	Repository repository.Repository

	// Publisher sends advisory event notifications after DB writes.
	// Defaults to events.NopPublisher{} if nil.
	Publisher events.Publisher
}

type Service struct {
	service.Service

	repository iclaimerRepository
	blockchain iclaimerBlockchain
	publisher  events.Publisher

	// submitted claims waiting for confirmation from the blockchain.
	// only accessed from tick, so no need for a lock
	// contains: application ID -> transaction hash, with a maximum of one
	// key per application due to the epoch advancement logic.
	claimsInFlight    map[int64]common.Hash
	submissionEnabled bool
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
	c.EnableReschedule = true

	err = service.Create(ctx, &c.CreateInfo, &s.Service)
	if err != nil {
		return nil, fmt.Errorf("creating base service: %w", err)
	}

	nodeConfig, err := setupPersistentConfig(ctx, s.Logger, c.Repository, &c.Config)
	if err != nil {
		return nil, fmt.Errorf("setting up persistent config: %w", err)
	}

	chainId, err := c.EthConn.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying chain ID: %w", err)
	}
	if chainId.Uint64() != c.Config.BlockchainId {
		return nil, fmt.Errorf("chainId mismatch: network %d != provided %d", chainId.Uint64(), c.Config.BlockchainId)
	}

	if chainId.Uint64() != nodeConfig.ChainID {
		return nil, fmt.Errorf("NodeConfig chainId mismatch: network %d != config %d",
			chainId.Uint64(), nodeConfig.ChainID)
	}
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.claimsInFlight = map[int64]common.Hash{}

	var txOpts *bind.TransactOpts = nil
	if s.submissionEnabled {
		txOpts, err = auth.GetTransactOpts(ctx, chainId)
		if err != nil {
			return nil, fmt.Errorf("getting transaction options: %w", err)
		}
	}

	s.publisher = c.Publisher
	if s.publisher == nil {
		s.publisher = events.NopPublisher{}
	}

	s.repository = c.Repository
	s.blockchain = &claimerBlockchain{
		logger:       s.Logger,
		client:       c.EthConn,
		txOpts:       txOpts,
		defaultBlock: c.Config.BlockchainDefaultBlock,
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

// NOTE: tick is not re-entrant!
func (s *Service) Tick() []error {
	if s.IsStopping() {
		return nil
	}

	errs := []error{}

	// gather epochs pairs with open claims, either:
	// - computed but not yet submitted
	acceptedOrSubmittedEpochs, computedEpochs, computedApps, errSubmitted := s.repository.SelectSubmittedClaimPairsPerApp(s.Context)
	if errSubmitted != nil {
		if s.IsStopping() && errors.Is(errSubmitted, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "error", errSubmitted)
			return nil
		}
		errs = append(errs, errSubmitted)
		return errs
	}

	// - submitted but not yet accepted.
	acceptedEpochs, submittedEpochs, submittedApps, errAccepted := s.repository.SelectAcceptedClaimPairsPerApp(s.Context)
	if errAccepted != nil {
		if s.IsStopping() && errors.Is(errAccepted, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "error", errAccepted)
			return nil
		}
		errs = append(errs, errAccepted)
		return errs
	}

	needAck, ackErr := s.repository.GetAppsNeedingAck(s.Context, repository.ServiceClaimer,
		repository.ConsensusTypesForService(repository.ServiceClaimer))
	if ackErr != nil {
		s.Logger.Warn("Failed to query apps needing drain ack", "error", ackErr)
	}
	drainingApps := make(map[int64]struct{}, len(needAck))
	for _, appID := range needAck {
		drainingApps[appID] = struct{}{}
	}

	// Clean up in-flight claims for apps no longer in the active set.
	for appID := range s.claimsInFlight {
		if _, draining := drainingApps[appID]; draining {
			continue
		}
		_, inComputed := computedEpochs[appID]
		_, inSubmitted := submittedEpochs[appID]
		if !inComputed && !inSubmitted {
			s.Logger.Info("Cleaning in-flight claim for inactive app",
				"app_id", appID)
			delete(s.claimsInFlight, appID)
		}
	}

	// Scan-based drain ack for soft-deleted apps.
	// Runs after in-flight cleanup so the check sees current state.
	if ackErr == nil {
		for _, appID := range needAck {
			if _, inFlight := s.claimsInFlight[appID]; inFlight {
				continue // defer until L1 tx clears
			}
			if err := s.repository.AcknowledgeAppStopped(s.Context, appID, repository.ServiceClaimer); err != nil {
				s.Logger.Warn("Failed to write claimer drain ack",
					"app_id", appID, "error", err)
			}
		}
	}

	s.Logger.Debug("Processing claims for epochs",
		"computed", len(computedEpochs),
		"submitted", len(submittedEpochs),
	)

	// return early if there is nothing to do
	if len(computedEpochs) == 0 && len(submittedEpochs) == 0 && len(s.claimsInFlight) == 0 {
		return nil
	}

	// we have claims to check. Get the latest/safe/finalized, etc. block
	defaultBlockNumber, err := s.blockchain.getDefaultBlockNumber(s.Context)
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	submitted, submitErrs := s.submitClaimsAndUpdateDatabase(
		acceptedOrSubmittedEpochs,
		computedEpochs,
		computedApps,
		drainingApps,
		defaultBlockNumber,
	)
	accepted, acceptErrs := s.acceptClaimsAndUpdateDatabase(acceptedEpochs, submittedEpochs, submittedApps, defaultBlockNumber)
	errs = append(errs, submitErrs...)
	errs = append(errs, acceptErrs...)

	// Signal reschedule whenever pipeline progress was made, even with errors.
	// Accepting a claim frees the pipeline slot for the next epoch's submission.
	// Confirming a submission enables the acceptance scan on the next tick.
	// Erring apps are retried on the next tick regardless; suppressing
	// reschedule would delay healthy apps by a full poll interval.
	if submitted > 0 || accepted > 0 {
		s.SignalReschedule()
	}
	return errs
}

func setupPersistentConfig(
	ctx context.Context,
	logger *slog.Logger,
	repo iclaimerRepository,
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
			return nil, fmt.Errorf("saving claimer persistent config: %w", err)
		}
		return &nc.Value, nil
	} else if err == nil {
		logger.Info("Claimer was already configured. Using previous persistent config", "config", config.Value)
		return &config.Value, nil
	}

	logger.Error("Could not retrieve persistent config from Database.", "error", err)
	return nil, err
}
