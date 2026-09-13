// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type CreateInfo struct {
	service.CreateInfo
	Config         config.PrtConfig
	Repository     repository.Repository
	EthClient      EthClientInterface
	AdapterFactory AdapterFactory
}

type Service struct {
	service.Service
	repository          prtRepository
	client              EthClientInterface
	adapterFactory      AdapterFactory
	submissionEnabled   bool
	defaultBlock        DefaultBlock
	submissionTimeout   time.Duration
	filter              ethutil.Filter
	txOptsFactory       ethutil.TransactOptsFactory            // Set by Create whenever submission is enabled.
	pendingTransactions map[int64]pendingTournamentTransaction // application.ID -> pending action
	disputeWarnings     map[common.Address]struct{}
	zeroStagingWarnings map[int64]struct{}
	rootBondRecoveries  map[int64][]*rootBondRecovery
	observationHealthMu sync.RWMutex // Ready runs concurrently with the observation loop.
	observationFailures map[int64]tournamentObservationFailure
}

const PrtConfigKey = "prt"

type PersistentConfig = config.PersistentSubmitterConfig

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
	if err := config.CheckNetworkChainID(chainID, c.Config.BlockchainId); err != nil {
		return nil, err
	}

	s.repository = c.Repository
	if s.repository == nil {
		return nil, fmt.Errorf("repository on prt service Create is nil")
	}

	nodeConfig, err := s.setupPersistentConfig(ctx, &c.Config)
	if err != nil {
		return nil, err
	}

	s.client = c.EthClient
	s.submissionEnabled = nodeConfig.ClaimSubmissionEnabled
	s.defaultBlock = nodeConfig.DefaultBlock
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

	s.pendingTransactions = map[int64]pendingTournamentTransaction{}
	s.disputeWarnings = map[common.Address]struct{}{}
	s.zeroStagingWarnings = map[int64]struct{}{}
	s.rootBondRecoveries = map[int64][]*rootBondRecovery{}
	s.observationFailures = map[int64]tournamentObservationFailure{}

	if s.submissionEnabled {
		s.submissionTimeout = c.Config.BlockchainHttpRequestTimeout
		if s.submissionTimeout == 0 {
			return nil, fmt.Errorf("BlockchainHttpRequestTimeout must be different from zero")
		}
		s.txOptsFactory, err = auth.GetPrtTransactOptsFactory(ctx, chainID)
		if err != nil {
			return nil, err
		}
		if c.Config.BlockchainLegacyEnabled {
			s.txOptsFactory = ethutil.WithLegacyFees(s.txOptsFactory, c.EthClient)
		}
		s.Logger.Info("PRT submitter identity", "address", s.txOptsFactory.From())
	}

	return s, nil
}

func (s *Service) Alive() bool     { return true }
func (s *Service) Reload() []error { return nil }

// logErrorUnlessShutdown keeps an in-flight shutdown cancellation from being
// reported as an operational failure. DeadlineExceeded and cancellations while
// the service is running remain errors.
func (s *Service) logErrorUnlessShutdown(message string, err error, args ...any) {
	if s.IsStopping() && errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return
	}
	args = append(args, "error", err)
	s.Logger.Error(message, args...)
}

// Tick observes PRT tournaments and maintains their pending actions.
func (s *Service) Tick() []error {
	// Check for shutdown before starting work, consistent with the advancer.
	if s.IsStopping() {
		return nil
	}

	apps, _, err := getObservableApplications(s.Context, s.repository)
	if err != nil {
		// Only suppress context errors during shutdown; surface real DB errors.
		if s.IsStopping() && errors.Is(err, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "error", err)
			return nil
		}
		return []error{fmt.Errorf("failed to get observable applications: %w", err)}
	}
	s.pruneTournamentObservationFailures(apps)

	// Resolve observation policy once, but only when a path needs it. In
	// particular, queued foreclosed bond recovery does not require finality.
	observationBlock := sync.OnceValues(func() (uint64, error) {
		return s.getDefaultBlockNumber(s.Context)
	})
	// validate each application
	errs := []error{}
	for idx := range apps {
		if s.Context.Err() != nil {
			return errs
		}
		app := apps[idx]
		// Foreclosed apps still observe tournaments. Only healthy apps also
		// drain local claims and maintain already-queued bond recoveries.
		// EVM reader is the sole writer of ForecloseBlock.
		if app.ForecloseBlock != 0 {
			if ferr := s.handleForeclosedApp(s.Context, app, observationBlock); ferr != nil {
				if s.IsStopping() && errors.Is(ferr, context.Canceled) {
					continue
				}
				errs = append(errs, fmt.Errorf("draining foreclosed PRT application %s: %w", app.IApplicationAddress, ferr))
			}
			continue
		}
		confirmedBlock, err := observationBlock()
		if err != nil {
			if s.IsStopping() && errors.Is(err, context.Canceled) {
				return errs
			}
			errs = append(errs, fmt.Errorf("fetching configured block for application %s: %w", app.IApplicationAddress, err))
			continue
		}
		if err := s.validateApplication(s.Context, app, confirmedBlock); err != nil {
			// During shutdown, in-flight L1 requests see context cancellation.
			// Suppress these to avoid spurious ERR log entries.
			if s.IsStopping() && errors.Is(err, context.Canceled) {
				s.Logger.Warn("Tick interrupted by shutdown",
					"application", app.IApplicationAddress, "error", err)
				continue
			}
			errs = append(errs, fmt.Errorf("validating PRT application %s: %w", app.IApplicationAddress, err))
		}
	}
	return errs
}

// handleForeclosedApp observes a foreclosed application's tournaments and,
// while local processing is healthy, drains its epochs to a terminal state.
//
// Once the app has ingested its pre-foreclosure sealed epochs and the advancer
// has processed their inputs, each pre-foreclosure epoch is reconciled read-only
// against the chain: an epoch whose accepted root result has our commitment
// becomes CLAIM_ACCEPTED, and a mismatch marks the app DIVERGED (reproducing the
// on-chain divergence). Every remaining epoch can no longer be accepted once the
// app is foreclosed, so it is terminalized to CLAIM_FORECLOSED. Join, stage,
// and accept transactions are not sent. An already-queued root bond recovery
// can continue because it is independent of consensus foreclosure. A freshly
// bootstrapped node therefore reaches the same epoch states a node that ran in
// real time would have.
func (s *Service) handleForeclosedApp(ctx context.Context, app *Application, observationBlock func() (uint64, error)) error {
	if app.ForecloseBlock == 0 {
		return nil
	}
	mostRecentBlock, observationErr := observationBlock()
	var epochs []*Epoch
	var consensus DaveConsensusAdapter
	if observationErr == nil {
		epochs, consensus, observationErr = s.observeApplicationTournaments(ctx, app, mostRecentBlock)
	}
	if app.Status != ApplicationStatus_OK {
		return observationErr
	}
	// Queued payment maintenance does not depend on a finalized-head read.
	// Its wait or failure must not prevent the passive observation above.
	blocked, recoveryErr := s.recoverForeclosedRootBonds(ctx, app)
	if observationErr != nil || recoveryErr != nil {
		return errors.Join(observationErr, recoveryErr)
	}
	if blocked {
		return nil
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

func (s *Service) recoverForeclosedRootBonds(ctx context.Context, app *Application) (bool, error) {
	if !s.submissionEnabled || len(s.rootBondRecoveries[app.ID]) == 0 {
		return false, nil
	}
	mostRecentBlock, err := s.client.BlockNumber(ctx)
	if err != nil {
		return true, fmt.Errorf("fetching latest block for foreclosed app root bond recovery %s: %w",
			app.IApplicationAddress, err)
	}
	if _, err := s.waitForTournamentTransaction(ctx, app); err != nil {
		return true, fmt.Errorf("checking tournament transaction for foreclosed app %s: %w", app.IApplicationAddress, err)
	}
	if s.hasNonRecoveryMutationInFlight(app.ID) {
		return true, nil
	}
	if err := s.recoverRootBonds(ctx, app, mostRecentBlock); err != nil {
		return true, fmt.Errorf("recovering queued root bonds for foreclosed app %s: %w", app.IApplicationAddress, err)
	}
	return false, nil
}

// foreclosePendingClaimEpochs terminalizes remaining pre-foreclosure epochs.
// The caller must first confirm ingestion and input processing are complete.
// Epochs with an acceptance transaction stay pending for claim reconciliation.
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

func (s *Service) Stop(_ bool) []error {
	s.SetStopping()
	return nil
}

func (s *Service) String() string {
	return s.Name
}

// SubmitterAddress returns the configured PRT submitter address.
func (s *Service) SubmitterAddress() (common.Address, bool) {
	if s.txOptsFactory == nil {
		return common.Address{}, false
	}
	return s.txOptsFactory.From(), true
}

func (s *Service) setupPersistentConfig(
	ctx context.Context,
	c *config.PrtConfig,
) (*PersistentConfig, error) {
	requested := PersistentConfig{
		DefaultBlock: c.BlockchainDefaultBlock, ChainID: c.BlockchainId,
		ClaimSubmissionEnabled: c.FeatureClaimSubmissionEnabled,
	}
	if err := requested.Validate(); err != nil {
		return nil, fmt.Errorf("invalid prt config: %w", err)
	}
	config, err := repository.LoadNodeConfig[PersistentConfig](ctx, s.repository, PrtConfigKey)
	if config == nil && errors.Is(err, repository.ErrNotFound) {
		nc := NodeConfig[PersistentConfig]{
			Key:   PrtConfigKey,
			Value: requested,
		}
		s.Logger.Info("Initializing PRT persistent config", "config", nc.Value)
		config, err = repository.InitializeNodeConfig(ctx, s.repository, &nc)
	}
	if err == nil {
		if err := config.Value.CheckRequested(requested); err != nil {
			return nil, fmt.Errorf("prt persistent config: %w", err)
		}
		s.Logger.Info("PRT persistent config matches requested config", "config", config.Value)
		return &config.Value, nil
	}

	s.Logger.Error("could not retrieve persistent config from database", "error", err)
	return nil, err
}
