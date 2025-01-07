// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	appcontract "github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/service"
)

type CreateInfo struct {
	service.CreateInfo
	model.NodeConfig[model.NodeConfigValue]

	PostgresEndpoint       config.Redacted[string]
	BlockchainHttpEndpoint config.Redacted[string]
	BlockchainWsEndpoint   config.Redacted[string]
	Repository             repository.Repository
	EnableInputReader      bool
	MaxRetries             uint64
	MaxDelay               time.Duration
	MaxStartupTime         time.Duration
}

type Service struct {
	service.Service

	client                  EthClient
	wsClient                EthWsClient
	inputSource             InputSource
	repository              EvmReaderRepository
	contractFactory         ContractFactory
	inputBoxDeploymentBlock uint64
	defaultBlock            DefaultBlock
	hasEnabledApps          bool
	inputReaderEnabled      bool
}

func (c *CreateInfo) LoadEnv() {
	c.BlockchainHttpEndpoint.Value = config.GetBlockchainHttpEndpoint()
	c.BlockchainWsEndpoint.Value = config.GetBlockchainWsEndpoint()
	c.MaxDelay = config.GetEvmReaderRetryPolicyMaxDelay()
	c.MaxRetries = config.GetEvmReaderRetryPolicyMaxRetries()
	c.PostgresEndpoint.Value = config.GetPostgresEndpoint()
	c.LogLevel = service.LogLevel(config.GetLogLevel())
	c.LogPretty = config.GetLogPrettyEnabled()
	c.MaxStartupTime = config.GetMaxStartupTime()
	c.EnableInputReader = config.GetFeatureInputReaderEnabled()

	// persistent
	c.Key = BaseConfigKey
	c.Value.DefaultBlock = config.GetEvmReaderDefaultBlock()
	c.Value.InputBoxDeploymentBlock = uint64(config.GetContractsInputBoxDeploymentBlockNumber())
	c.Value.InputBoxAddress = common.HexToAddress(config.GetContractsInputBoxAddress()).String()
	c.Value.ChainID = config.GetBlockchainId()
}

func Create(c *CreateInfo, s *Service) error {
	var err error

	err = service.Create(&c.CreateInfo, &s.Service)
	if err != nil {
		return err
	}

	return service.WithTimeout(c.MaxStartupTime, func() error {
		client, err := ethclient.DialContext(s.Context, c.BlockchainHttpEndpoint.Value)
		if err != nil {
			return err
		}

		wsClient, err := ethclient.DialContext(s.Context, c.BlockchainWsEndpoint.Value)
		if err != nil {
			return err
		}

		if c.Repository == nil {
			c.Repository, err = factory.NewRepositoryFromConnectionString(s.Context, c.PostgresEndpoint.Value)
			if err != nil {
				return err
			}
		}

		err = s.SetupNodeConfig(s.Context, c.Repository, &c.NodeConfig)
		if err != nil {
			return err
		}

		inputSource, err := NewInputSourceAdapter(common.HexToAddress(c.Value.InputBoxAddress), client)
		if err != nil {
			return err
		}

		contractFactory := NewEvmReaderContractFactory(client, c.MaxRetries, c.MaxDelay)

		s.client = NewEhtClientWithRetryPolicy(client, c.MaxRetries, c.MaxDelay, s.Logger)
		s.wsClient = NewEthWsClientWithRetryPolicy(wsClient, c.MaxRetries, c.MaxDelay, s.Logger)
		s.inputSource = NewInputSourceWithRetryPolicy(inputSource, c.MaxRetries, c.MaxDelay, s.Logger)
		s.repository = c.Repository
		s.inputBoxDeploymentBlock = c.Value.InputBoxDeploymentBlock
		s.defaultBlock = c.Value.DefaultBlock
		s.contractFactory = contractFactory
		s.hasEnabledApps = true
		s.inputReaderEnabled = c.EnableInputReader

		return nil
	})
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

func (s *Service) Tick() []error {
	return []error{}
}

func (s *Service) Serve() error {
	ready := make(chan struct{}, 1)
	go s.Run(s.Context, ready)
	return s.Service.Serve()
}

func (s *Service) String() string {
	return s.Name
}

func (s *Service) SetupNodeConfig(
	ctx context.Context,
	r repository.Repository,
	c *model.NodeConfig[model.NodeConfigValue],
) error {
	_, err := repository.LoadNodeConfig[model.NodeConfigValue](ctx, r, BaseConfigKey)
	if err == pgx.ErrNoRows {
		s.Logger.Debug("Initializing node config", "config", c)
		err = repository.SaveNodeConfig(ctx, r, c)
		if err != nil {
			return err
		}
	} else if err == nil {
		s.Logger.Warn("Node was already configured. Using previous persistent config", "config", c.Value)
	} else {
		s.Logger.Error("Could not retrieve persistent config from Database. %w", "error", err)
	}
	return err
}

// Interface for Input reading
type InputSource interface {
	// Wrapper for FilterInputAdded(), which is automatically generated
	// by go-ethereum and cannot be used for testing
	RetrieveInputs(opts *bind.FilterOpts, appAddresses []common.Address, index []*big.Int,
	) ([]iinputbox.IInputBoxInputAdded, error)
}

// Interface for the node repository
type EvmReaderRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination) ([]*Application, error)

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)

	// Input monitor
	CreateEpochsAndInputs(
		ctx context.Context, nameOrAddress string,
		epochInputMap map[*Epoch][]*Input, blockNumber uint64,
	) error
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination) ([]*Epoch, error)

	// Claim acceptance monitor
	UpdateEpochsClaimAccepted(ctx context.Context, nameOrAddress string, epochs []*Epoch, lastClaimCheckBlock uint64) error

	// Output execution monitor
	GetOutput(ctx context.Context, nameOrAddress string, indexKey uint64) (*Output, error)
	UpdateOutputsExecution(ctx context.Context, nameOrAddress string, executedOutputs []*Output, blockNumber uint64) error
}

// EthClient mimics part of ethclient.Client functions to narrow down the
// interface needed by the EvmReader. It must be bound to an HTTP endpoint
type EthClient interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// EthWsClient mimics part of ethclient.Client functions to narrow down the
// interface needed by the EvmReader. It must be bound to a WS endpoint
type EthWsClient interface {
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
}

type ConsensusContract interface {
	GetEpochLength(opts *bind.CallOpts) (*big.Int, error)
	RetrieveClaimAcceptanceEvents(
		opts *bind.FilterOpts,
		appAddresses []common.Address,
	) ([]*iconsensus.IConsensusClaimAcceptance, error)
}

type ApplicationContract interface {
	GetConsensus(opts *bind.CallOpts) (common.Address, error)
	RetrieveOutputExecutionEvents(
		opts *bind.FilterOpts,
	) ([]*appcontract.IApplicationOutputExecuted, error)
}

type ContractFactory interface {
	NewApplication(address common.Address) (ApplicationContract, error)
	NewIConsensus(address common.Address) (ConsensusContract, error)
}

type SubscriptionError struct {
	Cause error
}

func (e *SubscriptionError) Error() string {
	return fmt.Sprintf("Subscription error : %v", e.Cause)
}

// Internal struct to hold application and it's contracts together
type appContracts struct {
	application         *Application
	applicationContract ApplicationContract
	consensusContract   ConsensusContract
}

func (r *Service) Run(ctx context.Context, ready chan struct{}) error {
	for {
		err := r.watchForNewBlocks(ctx, ready)
		// If the error is a SubscriptionError, re run watchForNewBlocks
		// that it will restart the websocket subscription
		if _, ok := err.(*SubscriptionError); !ok {
			return err
		}
		r.Logger.Error(err.Error())
		r.Logger.Info("Restarting subscription")
	}
}

func getAllRunningApplications(ctx context.Context, er EvmReaderRepository) ([]*Application, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled)}
	return er.ListApplications(ctx, f, repository.Pagination{})
}

// watchForNewBlocks watches for new blocks and reads new inputs based on the
// default block configuration, which have not been processed yet.
func (r *Service) watchForNewBlocks(ctx context.Context, ready chan<- struct{}) error {
	headers := make(chan *types.Header)
	sub, err := r.wsClient.SubscribeNewHead(ctx, headers)
	if err != nil {
		return fmt.Errorf("could not start subscription: %v", err)
	}
	r.Logger.Info("Subscribed to new block events")
	ready <- struct{}{}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sub.Err():
			return &SubscriptionError{Cause: err}
		case header := <-headers:

			// Every time a new block arrives
			r.Logger.Debug("New block header received", "blockNumber", header.Number, "blockHash", header.Hash())

			r.Logger.Debug("Retrieving enabled applications")
			runningApps, err := getAllRunningApplications(ctx, r.repository)
			if err != nil {
				r.Logger.Error("Error retrieving running applications",
					"error",
					err,
				)
				continue
			}

			if len(runningApps) == 0 {
				if r.hasEnabledApps {
					r.Logger.Info("No registered applications enabled")
				}
				r.hasEnabledApps = false
				continue
			}
			if !r.hasEnabledApps {
				r.Logger.Info("Found enabled applications")
			}
			r.hasEnabledApps = true

			// Build Contracts
			var apps []appContracts
			for _, app := range runningApps {
				applicationContract, consensusContract, err := r.getAppContracts(app)
				if err != nil {
					r.Logger.Error("Error retrieving application contracts", "app", app, "error", err)
					continue
				}
				aContracts := appContracts{
					application:         app,
					applicationContract: applicationContract,
					consensusContract:   consensusContract,
				}

				apps = append(apps, aContracts)
			}

			if len(apps) == 0 {
				r.Logger.Info("No correctly configured applications running")
				continue
			}

			blockNumber := header.Number.Uint64()
			if r.defaultBlock != DefaultBlock_Latest {
				mostRecentHeader, err := r.fetchMostRecentHeader(
					ctx,
					r.defaultBlock,
				)
				if err != nil {
					r.Logger.Error("Error fetching most recent block",
						"default block", r.defaultBlock,
						"error", err)
					continue
				}
				blockNumber = mostRecentHeader.Number.Uint64()

				r.Logger.Debug(fmt.Sprintf("Using block %d and not %d because of commitment policy: %s",
					mostRecentHeader.Number.Uint64(), header.Number.Uint64(), r.defaultBlock))
			}

			r.checkForNewInputs(ctx, apps, blockNumber)

			r.checkForClaimStatus(ctx, apps, blockNumber)

			r.checkForOutputExecution(ctx, apps, blockNumber)

		}
	}
}

// fetchMostRecentHeader fetches the most recent header up till the
// given default block
func (r *Service) fetchMostRecentHeader(
	ctx context.Context,
	defaultBlock DefaultBlock,
) (*types.Header, error) {

	var defaultBlockNumber int64
	switch defaultBlock {
	case DefaultBlock_Pending:
		defaultBlockNumber = rpc.PendingBlockNumber.Int64()
	case DefaultBlock_Latest:
		defaultBlockNumber = rpc.LatestBlockNumber.Int64()
	case DefaultBlock_Finalized:
		defaultBlockNumber = rpc.FinalizedBlockNumber.Int64()
	case DefaultBlock_Safe:
		defaultBlockNumber = rpc.SafeBlockNumber.Int64()
	default:
		return nil, fmt.Errorf("default block '%v' not supported", defaultBlock)
	}

	header, err :=
		r.client.HeaderByNumber(
			ctx,
			new(big.Int).SetInt64(defaultBlockNumber))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve header. %v", err)
	}

	if header == nil {
		return nil, fmt.Errorf("returned header is nil")
	}
	return header, nil
}

// getAppContracts retrieves the ApplicationContract and ConsensusContract for a given Application.
// Also validates if IConsensus configuration matches the blockchain registered one
func (r *Service) getAppContracts(app *Application,
) (ApplicationContract, ConsensusContract, error) {
	if app == nil {
		return nil, nil, fmt.Errorf("Application reference is nil. Should never happen")
	}

	applicationContract, err := r.contractFactory.NewApplication(app.IApplicationAddress)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("error building application contract"),
			err,
		)

	}
	consensusAddress, err := applicationContract.GetConsensus(nil)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("error retrieving application consensus"),
			err,
		)
	}

	if app.IConsensusAddress != consensusAddress {
		return nil, nil,
			fmt.Errorf("IConsensus addresses do not match. Deployed: %s. Configured: %s",
				consensusAddress,
				app.IConsensusAddress)
	}

	consensus, err := r.contractFactory.NewIConsensus(consensusAddress)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("error building consensus contract"),
			err,
		)

	}
	return applicationContract, consensus, nil
}

// getEpochLength reads the application epoch length given it's consensus contract
func getEpochLength(consensus ConsensusContract) (uint64, error) {
	// FIXME: move to ethutil

	epochLengthRaw, err := consensus.GetEpochLength(nil)
	if err != nil {
		return 0, errors.Join(
			fmt.Errorf("error retrieving application epoch length"),
			err,
		)
	}

	return epochLengthRaw.Uint64(), nil
}
