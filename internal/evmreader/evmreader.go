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

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

// Interface for the node repository
type EvmReaderRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
	UpdateEventLastCheckBlock(ctx context.Context, appIDs []int64, event MonitoredEvent, blockNumber uint64) error

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)

	// Input monitor
	CreateEpochsAndInputs(
		ctx context.Context, nameOrAddress string,
		epochInputMap map[*Epoch][]*Input, blockNumber uint64,
	) error
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool) ([]*Epoch, uint64, error)

	// Output execution monitor
	GetOutput(ctx context.Context, nameOrAddress string, indexKey uint64) (*Output, error)
	UpdateOutputsExecution(ctx context.Context, nameOrAddress string, executedOutputs []*Output, blockNumber uint64) error
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
	ChainID(ctx context.Context) (*big.Int, error)
}

type ApplicationContractAdapter interface {
	RetrieveOutputExecutionEvents(
		opts *bind.FilterOpts,
	) ([]*iapplication.IApplicationOutputExecuted, error)
	GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error)
}

// Interface for Input reading
type InputSourceAdapter interface {
	// Wrapper for FilterInputAdded(), which is automatically generated
	// by go-ethereum and cannot be used for testing
	RetrieveInputs(opts *bind.FilterOpts, appAddresses []common.Address, index []*big.Int,
	) ([]iinputbox.IInputBoxInputAdded, error)
	GetNumberOfInputs(opts *bind.CallOpts, appContract common.Address) (*big.Int, error)
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
	applicationContract ApplicationContractAdapter
	inputSource         InputSourceAdapter
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

func getAllRunningApplications(ctx context.Context, er EvmReaderRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{
		State: Pointer(ApplicationState_Enabled),
	}
	return er.ListApplications(ctx, f, repository.Pagination{}, false)
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
			runningApps, _, err := getAllRunningApplications(ctx, r.repository)
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
				applicationContract, inputSource, err := r.adapterFactory.CreateAdapters(app, r.client)

				if err != nil {
					r.Logger.Error("Error retrieving application contracts", "app", app, "error", err)
					continue
				}
				aContracts := appContracts{
					application:         app,
					applicationContract: applicationContract,
					inputSource:         inputSource,
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

type AdapterFactory interface {
	CreateAdapters(app *Application, client EthClientInterface) (ApplicationContractAdapter, InputSourceAdapter, error)
}

type DefaultAdapterFactory struct {
	Filter ethutil.Filter
}

func (f *DefaultAdapterFactory) CreateAdapters(app *Application, client EthClientInterface) (ApplicationContractAdapter, InputSourceAdapter, error) {
	if app == nil {
		return nil, nil, fmt.Errorf("Application reference is nil. Should never happen")
	}

	// Type assertion to get the concrete client if possible
	ethClient, ok := client.(*ethclient.Client)
	if !ok {
		return nil, nil, fmt.Errorf("client is not an *ethclient.Client, cannot create adapters")
	}

	applicationContract, err := NewApplicationContractAdapter(app.IApplicationAddress, ethClient, f.Filter)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("error building application contract"),
			err,
		)
	}

	inputSource, err := NewInputSourceAdapter(app.IInputBoxAddress, ethClient, f.Filter)
	if err != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("error building inputbox contract"),
			err,
		)
	}

	return applicationContract, inputSource, nil
}
