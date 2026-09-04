// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"
	"math/big"
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
	repository        prtRepository
	client            EthClientInterface
	adapterFactory    AdapterFactory
	submissionEnabled bool
	submissionTimeout time.Duration
	filter            ethutil.Filter
	txOptsFactory     ethutil.TransactOptsFactory
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
		s.submissionTimeout = c.Config.BlockchainHttpRequestTimeout
		if s.submissionTimeout == 0 {
			return nil, fmt.Errorf("BlockchainHttpRequestTimeout must be different from zero")
		}
		s.txOptsFactory, err = auth.GetTransactOptsFactory(ctx, chainID)
		if err != nil {
			return nil, err
		}
		s.Logger.Info("PRT submitter identity", "address", s.txOptsFactory.From())
	}

	return s, nil
}

func (s *Service) Alive() bool     { return true }
func (s *Service) Ready() bool     { return true }
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

// Tick executes the Validator main logic of producing claims and/or proofs
// for processed epochs of all running applications.
func (s *Service) Tick() []error {
	// Check for shutdown before starting work, consistent with the advancer.
	if s.IsStopping() {
		return nil
	}

	apps, _, err := getAllRunningApplications(s.Context, s.repository)
	if err != nil {
		// Only suppress context errors during shutdown; surface real DB errors.
		if s.IsStopping() && errors.Is(err, context.Canceled) {
			s.Logger.Warn("Tick interrupted by shutdown", "error", err)
			return nil
		}
		return []error{fmt.Errorf("failed to get running applications. %w", err)}
	}

	// validate each application
	errs := []error{}
	for idx := range apps {
		if s.Context.Err() != nil {
			return errs
		}
		app := apps[idx]
		// Foreclosed apps: run the drain path (reconcile accepted epochs,
		// foreclose the rest) instead of normal tournament work. EVM reader is
		// the sole writer of ForecloseBlock; the app keeps health status OK and
		// remains enabled for L1 observation.
		if app.ForecloseBlock != 0 {
			if ferr := s.handleForeclosedApp(s.Context, app); ferr != nil {
				if s.IsStopping() && errors.Is(ferr, context.Canceled) {
					continue
				}
				errs = append(errs, ferr)
			}
			continue
		}
		if err := s.validateApplication(s.Context, app); err != nil {
			// During shutdown, in-flight L1 requests see context cancellation.
			// Suppress these to avoid spurious ERR log entries.
			if s.IsStopping() && errors.Is(err, context.Canceled) {
				s.Logger.Warn("Tick interrupted by shutdown",
					"application", app.IApplicationAddress, "error", err)
				continue
			}
			errs = append(errs, err)
		}
	}
	return errs
}

// handleForeclosedApp drains a foreclosed DaveConsensus application's epochs to
// a terminal state. Foreclosure is a lifecycle fact (foreclose_block); the
// application keeps health status OK and stays enabled for L1 observation.
//
// Once the app has ingested its pre-foreclosure sealed epochs and the advancer
// has processed their inputs, each pre-foreclosure epoch is reconciled read-only
// against the chain: an epoch whose root tournament settled with our commitment
// becomes CLAIM_ACCEPTED, and a mismatch marks the app DIVERGED (reproducing the
// on-chain divergence). Every remaining epoch can no longer be accepted once the
// app is foreclosed, so it is terminalized to CLAIM_FORECLOSED. No Settle/Join
// transactions are sent. A freshly bootstrapped node therefore reaches the same
// epoch states a node that ran in real time would have.
func (s *Service) handleForeclosedApp(ctx context.Context, app *Application) error {
	if app.ForecloseBlock == 0 {
		return nil
	}
	// Bootstrap-readiness guard. The drain gate below answers "given the
	// rows currently in the local input table, is there any pre-foreclosure
	// input still status=NONE?". For a freshly registered PRT app against
	// an already-foreclosed contract, evmreader's checkForForeclosure writes
	// foreclose_block before checkForEpochsAndInputs has had a chance to
	// ingest the historical sealed epochs (and their inputs) — so the gate
	// would see an empty table and return false. PRT's input ingestion is
	// driven by EpochSealed scans, so the relevant scanner cursor is
	// last_epoch_check_block (not last_input_check_block, which the Dave
	// path never writes) — ForeclosureScanCaughtUp branches on consensus
	// type to consult it.
	if !app.ForeclosureScanCaughtUp() {
		s.Logger.Info(
			"Foreclosed PRT application still ingesting pre-foreclosure sealed epochs",
			"application", app.Name,
			"address", app.IApplicationAddress,
			"last_epoch_check_block", app.LastEpochCheckBlock,
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

	// Read-only reconciliation: accept epochs whose root tournament settled with
	// our commitment, and surface any divergence. This sends no transactions.
	mostRecentBlock, err := s.client.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("fetching latest block for foreclosed app %s: %w",
			app.IApplicationAddress, err)
	}
	if err := s.checkEpochs(ctx, app, mostRecentBlock); err != nil {
		// A divergence detected here marks the app DIVERGED and returns the
		// reason; propagate it like the normal validation path does.
		return err
	}

	// Claim-computed epochs without an on-chain claim transaction can never be
	// accepted now that the app is foreclosed: terminalize them to CLAIM_FORECLOSED.
	return s.forecloseComputedEpochs(ctx, app)
}

// forecloseComputedEpochs transitions every unaccepted CLAIM_COMPUTED epoch of a
// foreclosed application to CLAIM_FORECLOSED. Epochs that already have a
// ClaimTransactionHash have an on-chain EpochSealed event to reconcile; leave
// them CLAIM_COMPUTED so the next checkEpochs pass can accept or reject them.
func (s *Service) forecloseComputedEpochs(ctx context.Context, app *Application) error {
	epochs, _, err := getAllClaimComputedEpochs(ctx, s.repository, app.Name)
	if err != nil {
		return fmt.Errorf("listing computed epochs for foreclosed app %s: %w",
			app.IApplicationAddress, err)
	}
	for _, epoch := range epochs {
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
