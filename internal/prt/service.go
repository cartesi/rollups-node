// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
)

type CreateInfo struct {
	TransactOptsFactory ethutil.TransactOptsFactory
	Config              config.PrtConfig
	Logger              *slog.Logger
	Repository          repository.Repository
	EthClient           EthClientInterface
	AdapterFactory      AdapterFactory
	DeliveryBackend     txsender.Backend
}

type Service struct {
	protectionNotices       map[int64]map[protectionNoticeKey]protectionNotice
	deployments             deploymentCache
	observationHardWarnedAt time.Time
	service.TickServiceTemplate
	delivery            *delivery
	pollInterval        time.Duration
	observerFacts       txsender.BlockRef
	hardFailureReason   atomic.Pointer[prtFailure]
	repository          prtRepository
	client              EthClientInterface
	adapterFactory      AdapterFactory
	submissionEnabled   bool
	publicationReady    atomic.Bool
	defaultBlock        DefaultBlock
	filter              ethutil.Filter
	txOptsFactory       ethutil.TransactOptsFactory // Set by Create whenever submission is enabled.
	disputeWarnings     map[common.Address]struct{}
	zeroStagingWarnings map[int64]struct{}
	observationFailures map[int64]tournamentObservationFailure // Owned only by the observer; Ready does not read it.
}

type prtFailure struct{ err error }

func (s *Service) stopPublication(err error) {
	s.hardFailureReason.CompareAndSwap(nil, &prtFailure{err: err})
}
func (s *Service) publicationStopped() bool { return s.hardFailureReason.Load() != nil }

func (s *Service) publicationFailure() error {
	if reason := s.hardFailureReason.Load(); reason != nil {
		return reason.err
	}
	return fmt.Errorf("%w: PRT protected publication prerequisite failed", txsender.ErrContent)
}

const PrtConfigKey = "prt"

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
		return nil, fmt.Errorf("repository on prt service Create is nil")
	}

	s := &Service{pollInterval: c.Config.PrtPollingInterval}
	if s.pollInterval <= 0 {
		s.pollInterval = time.Minute
	}
	tickCfg := &service.TickServiceConfigs{
		BaseConfigs: service.BaseConfigs{
			Name:     config.ServicePrt,
			Logger:   c.Logger,
			LogLevel: c.Config.LogLevel,
			LogColor: c.Config.LogColor,
		},
		PollInterval: c.Config.PrtPollingInterval,
	}
	err = service.InitTickServiceTemplate(&s.TickServiceTemplate, tickCfg, s)
	if err != nil {
		return nil, err
	}

	ethClient := c.EthClient
	if ethClient == nil {
		authOpt, err := config.HTTPAuthorizationOption()
		if err != nil {
			return nil, err
		}
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
		return nil, err
	}
	if err := config.CheckNetworkChainID(chainID, c.Config.BlockchainId); err != nil {
		return nil, err
	}

	s.repository = c.Repository

	var signer common.Address
	if c.Config.FeatureClaimSubmissionEnabled {
		if c.Config.BlockchainHttpRequestTimeout <= 0 {
			return nil, fmt.Errorf("BlockchainHttpRequestTimeout must be positive")
		}
		s.txOptsFactory = c.TransactOptsFactory
		if s.txOptsFactory == nil {
			s.txOptsFactory, err = auth.GetPrtTransactOptsFactory(ctx, chainID)
		}
		if err != nil {
			return nil, fmt.Errorf("PRT submission is enabled and requires signer configuration "+
				"(CARTESI_PRT_AUTH_*); the standalone node always starts PRT: %w", err)
		}
		signer = s.txOptsFactory.From()
		s.Logger.Info("PRT submitter identity", "address", signer)
	}
	nodeConfig, err := s.setupPersistentConfig(ctx, &c.Config, c.Repository, signer)
	if err != nil {
		return nil, err
	}

	s.client = ethClient
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.defaultBlock = nodeConfig.DefaultBlock
	if s.defaultBlock != DefaultBlock_Finalized {
		s.Logger.Warn("PRT stores provisional observations; reorg rollback is unsupported", "block_policy", s.defaultBlock)
	}
	s.filter = ethutil.Filter{
		MaxChunkSize: new(big.Int).SetUint64(c.Config.BlockchainMaxBlockRange),
		Logger:       s.Logger,
	}

	s.disputeWarnings = map[common.Address]struct{}{}
	s.zeroStagingWarnings = map[int64]struct{}{}
	s.observationFailures = map[int64]tournamentObservationFailure{}

	s.delivery, err = newDelivery(ctx, c, s, nodeConfig, chainID)
	if err != nil {
		return nil, err
	}

	s.Logger.Info("Created", "config", c.Config)

	return s, nil
}

// logErrorUnlessShutdown suppresses shutdown cancellations and ordinary waits.
// Any real failure or expired operation deadline remains visible.
func (s *Service) logErrorUnlessShutdown(ctx context.Context, message string, err error, args ...any) {
	if txsender.ShutdownOnly(ctx, err, repository.ErrTournamentIntervalStale, txsender.ErrLimit) {
		return
	}
	args = append(args, "error", err)
	s.Logger.Error(message, args...)
}

// Serve runs the PRT observer and publisher independently of Claimer.
// Temporary publication failures retain the current transaction for the next pass.
func (s *Service) Serve(ctx context.Context) error {
	if !s.submissionEnabled {
		return s.TickServiceTemplate.Serve(ctx)
	}
	if s.delivery == nil {
		return errors.New("PRT delivery is not initialized")
	}
	if err := s.waitForObserver(ctx); err != nil {
		return err
	}
	s.publicationReady.Store(true)
	defer s.publicationReady.Store(false)
	return s.serveLoops(ctx)
}

// Tick is the observer loop. The publisher runs independently in Serve.
func (s *Service) Tick(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return false, nil
		}
		return false, err
	}
	if s.delivery == nil {
		return false, errors.New("PRT delivery is not initialized")
	}
	err := s.observe(ctx)
	if txsender.ShutdownOnly(ctx, err, repository.ErrTournamentIntervalStale, txsender.ErrLimit) {
		return false, nil
	}
	if errors.Is(err, txsender.ErrFinality) || errors.Is(err, txsender.ErrContent) {
		if !warningDue(s.observationHardWarnedAt) {
			return false, nil
		}
		s.observationHardWarnedAt = time.Now()
	}
	if deliveryWaitOnly(err) {
		s.logDeliveryWait(err)
		return false, nil
	}
	return false, err
}

// serveLoops waits for both workers before returning from service shutdown.
func (s *Service) serveLoops(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	observerDone := make(chan error, 1)
	go func() { observerDone <- s.TickServiceTemplate.Serve(runCtx) }()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		err := s.delivery.tick(runCtx)
		if deliveryWaitOnly(err) {
			s.logDeliveryWait(err)
		} else if err != nil && !s.publicationStopped() {
			s.delivery.reportPublicationError(runCtx, err)
		}
		select {
		case <-ctx.Done():
			cancel()
			return errors.Join(ctx.Err(), <-observerDone)
		case err := <-observerDone:
			cancel()
			return err
		case <-ticker.C:
		}
	}
}

func (s *Service) drainForeclosedClaims(
	ctx context.Context, app *Application, epochs []*Epoch, consensus DaveConsensusAdapter, mostRecentBlock uint64,
) error {
	state, generation, gateErr := s.deploymentVerdictState(ctx, app)
	if gateErr != nil {
		return gateErr
	}
	if state == deploymentNone || state == deploymentPending {
		return nil
	}
	// Another observer can have committed a newer foreclosure. Absence at our
	// protected head cannot prove that a claim is no longer acceptable there.
	if app.ForecloseBlock > mostRecentBlock {
		return txsender.ErrInconclusive
	}
	// Bootstrap-readiness guard. The drain gate below answers "given the
	// rows currently in the local input table, is there any pre-foreclosure
	// input still status=NONE?". For a freshly registered PRT app against
	// an already-foreclosed contract, evmreader's checkForForeclosure writes
	// foreclose_block before checkForEpochsAndInputs has had a chance to
	// ingest the historical sealed epochs (and their inputs) — so the gate
	// would see an empty table and return false. Dave writes both scanner
	// cursors. ForeclosureScanCaughtUp requires last_epoch_check_block AND
	// last_input_check_block to reach foreclose_block: sealed-epoch coverage
	// alone does not prove that open-epoch inputs have been ingested.
	if !app.ForeclosureScanCaughtUp() {
		s.Logger.Info(
			"Foreclosed PRT application still ingesting pre-foreclosure sealed epochs and inputs",
			"application", app.Name,
			"address", app.IApplicationAddress,
			"last_epoch_check_block", app.LastEpochCheckBlock,
			"last_input_check_block", app.LastInputCheckBlock,
			"foreclose_block", app.ForecloseBlock,
		)
		return nil
	}
	undrained, err := s.repository.HasUndrainedEpochsBeforeBlock(ctx, app.ID, app.ForecloseBlock)
	if err != nil {
		return fmt.Errorf("foreclosed app drain check (%s): %w",
			app.IApplicationAddress, err)
	}
	if undrained {
		s.Logger.Info(
			"Foreclosed PRT application still draining pre-foreclosure inputs",
			"application", app.Name,
			"address", app.IApplicationAddress,
			"foreclose_block", app.ForecloseBlock,
		)
		return nil
	}
	// Epoch-level completion gate. Once every pre-foreclosure epoch is terminal
	// (CLAIM_ACCEPTED or CLAIM_FORECLOSED), the drain is done and evmreader
	// continues the post-foreclosure observation (drive-prove, withdrawals).
	unreconciled, err := s.repository.HasUnreconciledClaimsBeforeBlock(ctx, app.ID, app.ForecloseBlock)
	if err != nil {
		return fmt.Errorf("foreclosed app claim-reconciliation check (%s): %w",
			app.IApplicationAddress, err)
	}
	if !unreconciled {
		return nil
	}
	if state == deploymentGeneration {
		return s.forecloseGenerationClaims(ctx, app, generation)
	}

	// Read-only reconciliation: accept epochs whose root result has
	// our commitment, and surface any divergence. This sends no transactions.
	deferActions, err := s.reconcileAcceptedEpochs(ctx, app, epochs, consensus, mostRecentBlock)
	if err != nil {
		// A divergence detected here marks the app DIVERGED and returns the
		// reason; propagate it like the normal validation path does.
		return err
	}
	if deferActions {
		return nil
	}

	// Pending claim epochs without an acceptance transaction can never be
	// accepted now that the app is foreclosed. Make them CLAIM_FORECLOSED.
	return s.foreclosePendingClaimEpochs(ctx, app)
}

func (s *Service) foreclosePendingClaimEpochs(ctx context.Context, app *Application) error {
	filter := repository.EpochFilter{Status: NonTerminalEpochStatuses()}
	epochs, _, err := s.repository.ListEpochs(ctx, app.Name, filter, repository.Pagination{}, false)
	if err != nil {
		return fmt.Errorf("listing pending claim epochs for foreclosed app %s: %w",
			app.IApplicationAddress, err)
	}
	for _, epoch := range epochs {
		if epoch.FirstBlock > app.ForecloseBlock {
			continue
		}
		// A complete valid prefix epoch still needs the validator before its
		// accepted result can be reconciled. Foreclosure must not erase it.
		if epoch.Status == EpochStatus_InputsProcessed && epoch.LastBlock < app.ForecloseBlock {
			pending, err := s.observerCompletionEpoch(ctx, app, epoch)
			if err != nil {
				return err
			}
			if pending != nil {
				continue
			}
		}
		if epoch.ClaimTransactionHash != nil {
			s.Logger.Debug("Skipping foreclose terminalization for epoch with on-chain claim transaction",
				"application", app.Name,
				"address", app.IApplicationAddress,
				"epoch_index", epoch.Index,
				"tx", epoch.ClaimTransactionHash,
			)
			continue
		}
		if err := s.repository.UpdateEpochWithForeclosedClaim(ctx, app.ID, epoch.Index); err != nil {
			// The guarded transition can lose eligibility after selection. Only
			// the pure no-write result is deferred; dependency errors stay visible.
			if err == repository.ErrNoUpdate {
				continue
			}
			return fmt.Errorf("foreclosing epoch %d of app %s: %w",
				epoch.Index, app.IApplicationAddress, err)
		}
		s.Logger.Info("Terminalized unaccepted epoch of foreclosed application",
			"application", app.Name,
			"address", app.IApplicationAddress,
			"epoch_index", epoch.Index,
			"foreclose_block", app.ForecloseBlock,
		)
	}
	return nil
}

func (s *Service) setupPersistentConfig(
	ctx context.Context,
	c *config.PrtConfig,
	repo repository.DeliveryRepository,
	signer common.Address,
) (*PersistentConfig, error) {
	requested := PersistentConfig{
		DefaultBlock: c.BlockchainDefaultBlock, ChainID: c.BlockchainId,
		ClaimSubmissionEnabled: c.FeatureClaimSubmissionEnabled,
		ResolvedSignerAddress:  signer,
	}
	saved, err := repo.RegisterSubmitter(ctx, PrtConfigKey, requested)
	if err != nil {
		return nil, err
	}
	s.Logger.Info("PRT persistent config matches requested config", "config", saved)
	return &saved, nil
}
