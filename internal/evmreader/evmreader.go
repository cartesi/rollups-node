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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/cartesi/rollups-node/internal/appstatus"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

// Interface for the node repository
type EvmReaderRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
	UpdateEventLastCheckBlock(ctx context.Context, appIDs []int64, event MonitoredEvent, blockNumber uint64) error
	GetEventLastCheckBlock(ctx context.Context, appID int64, event MonitoredEvent) (uint64, error)

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)

	// Input monitor
	CreateEpochsAndInputs(
		ctx context.Context, nameOrAddress string,
		epochInputMap map[*Epoch][]*Input, blockNumber uint64,
	) error
	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter, p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	UpdateEpochClaimTransactionHash(ctx context.Context, nameOrAddress string, e *Epoch) error
	GetLastNonOpenEpoch(ctx context.Context, nameOrAddress string) (*Epoch, error)

	GetNumberOfInputs(ctx context.Context, nameOrAddress string) (uint64, error)

	// Output execution monitor
	GetOutput(ctx context.Context, nameOrAddress string, indexKey uint64) (*Output, error)
	UpdateOutputsExecution(ctx context.Context, nameOrAddress string, executedOutputs []*Output, blockNumber uint64) error
	GetNumberOfExecutedOutputs(ctx context.Context, nameOrAddress string) (uint64, error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
	ChainID(ctx context.Context) (*big.Int, error)
}

type SubscriptionError struct {
	Cause error
}

func (e *SubscriptionError) Error() string {
	return fmt.Sprintf("Subscription error : %v", e.Cause)
}

func (e *SubscriptionError) Unwrap() error {
	return e.Cause
}

// Internal struct to hold application and it's contracts together
type appContracts struct {
	application         *Application
	applicationContract ApplicationContractAdapter
	inputSource         InputSourceAdapter
	daveConsensus       DaveConsensusAdapter
}

// cachedAdapters stores contract adapters along with the configuration fields
// used to create them, enabling staleness detection when app config changes.
type cachedAdapters struct {
	applicationContract ApplicationContractAdapter
	inputSource         InputSourceAdapter
	daveConsensus       DaveConsensusAdapter
	consensusAddr       common.Address
	inputBoxAddr        common.Address
	isDaveConsensus     bool
	hasInputBoxDA       bool
}

func (r *Service) Run(ctx context.Context, ready chan struct{}) error {
	var consecutiveFailures uint64
	for {
		headersProcessed, err := r.watchForNewBlocks(ctx, ready)
		if ctx.Err() != nil {
			return nil
		}
		r.Logger.Error("watchForNewBlocks exited",
			"error", err, "headers_processed", headersProcessed)

		// Only reset the retry counter if the connection actually processed
		// at least one block header. This prevents infinite retries when the
		// subscription connects but immediately fails before doing useful work.
		if headersProcessed > 0 {
			consecutiveFailures = 0
		} else {
			consecutiveFailures++
		}

		if consecutiveFailures > r.blockchainMaxRetries {
			r.Logger.Error("Max consecutive failures reached. Exiting",
				"consecutive_failures", consecutiveFailures,
				"max_retries", r.blockchainMaxRetries,
			)
			return err
		}

		r.Logger.Info("Restarting subscription",
			"consecutive_failures", consecutiveFailures,
			"max_retries", r.blockchainMaxRetries,
		)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.blockchainSubscriptionRetryInterval):
		}
	}
}

func getAllRunningApplications(ctx context.Context, er EvmReaderRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{
		State: Pointer(ApplicationState_Enabled),
	}
	return er.ListApplications(ctx, f, repository.Pagination{}, false)
}

func (r *Service) setApplicationInoperable(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetInoperablef(ctx, r.Logger, r.repository, app, reasonFmt, args...)
}

// watchForNewBlocks subscribes to new block headers and processes them.
// Returns the number of headers processed and any error that caused it to stop.
func (r *Service) watchForNewBlocks(ctx context.Context, ready chan<- struct{}) (uint64, error) {
	headers := make(chan *types.Header)
	sub, err := r.wsClient.SubscribeNewHead(ctx, headers)
	if err != nil {
		return 0, fmt.Errorf("could not start subscription: %w", err)
	}
	r.Logger.Info("Subscribed to new block events")
	select {
	case ready <- struct{}{}:
	default:
	}
	defer sub.Unsubscribe()

	liveness := time.NewTimer(r.wsLivenessTimeout)
	defer liveness.Stop()

	adapterCache := make(map[common.Address]cachedAdapters)
	var headersProcessed uint64
	for {
		var header *types.Header
		select {
		case <-ctx.Done():
			return headersProcessed, ctx.Err()
		case err := <-sub.Err():
			if err == nil {
				err = errors.New("subscription closed unexpectedly")
			}
			return headersProcessed, &SubscriptionError{Cause: err}
		case <-liveness.C:
			// Before declaring stalled, check if a header arrived simultaneously.
			// Go's select picks randomly when multiple cases are ready, so the
			// liveness timer may win even though a header is available.
			select {
			case header = <-headers:
			default:
				return headersProcessed, &SubscriptionError{
					Cause: fmt.Errorf(
						"no new block header received for %s, assuming stalled connection",
						r.wsLivenessTimeout,
					),
				}
			}
		case header = <-headers:
		}

		if header == nil {
			continue
		}
		headersProcessed++
		liveness.Reset(r.wsLivenessTimeout)

		// Every time a new block arrives
		r.Logger.Debug("New block header received",
			"blockNumber", header.Number, "blockHash", header.Hash())

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

		// Evict cache entries for applications that are no longer enabled.
		activeAddrs := make(map[common.Address]struct{}, len(runningApps))
		for _, app := range runningApps {
			activeAddrs[app.IApplicationAddress] = struct{}{}
		}
		for addr := range adapterCache {
			if _, active := activeAddrs[addr]; !active {
				r.Logger.Debug("Evicting cached adapters for removed application",
					"address", addr)
				delete(adapterCache, addr)
			}
		}

		// Build Contracts (adapters are cached per application address)
		var apps []appContracts
		var daveConsensusApps []appContracts
		var iconsensusApps []appContracts
		for _, app := range runningApps {
			addr := app.IApplicationAddress
			cached, cacheHit := adapterCache[addr]
			if cacheHit {
				// Invalidate cache if the app's contract configuration changed.
				if cached.consensusAddr != app.IConsensusAddress ||
					cached.inputBoxAddr != app.IInputBoxAddress ||
					cached.isDaveConsensus != app.IsDaveConsensus() ||
					cached.hasInputBoxDA !=
						app.HasDataAvailabilitySelector(DataAvailability_InputBox) {
					r.Logger.Info(
						"Application contract configuration changed, recreating adapters",
						"application", app.Name, "address", addr)
					delete(adapterCache, addr)
					cacheHit = false
				}
			}
			if !cacheHit {
				appContract, inputSource, daveConsensus, err :=
					r.adapterFactory.CreateAdapters(app)
				if err != nil {
					r.Logger.Error("Error retrieving application contracts",
						"app", app, "error", err)
					continue
				}
				cached = cachedAdapters{
					applicationContract: appContract,
					inputSource:         inputSource,
					daveConsensus:       daveConsensus,
					consensusAddr:       app.IConsensusAddress,
					inputBoxAddr:        app.IInputBoxAddress,
					isDaveConsensus:     app.IsDaveConsensus(),
					hasInputBoxDA: app.HasDataAvailabilitySelector(
						DataAvailability_InputBox),
				}
				adapterCache[addr] = cached
			}
			aContracts := appContracts{
				application:         app,
				applicationContract: cached.applicationContract,
				inputSource:         cached.inputSource,
				daveConsensus:       cached.daveConsensus,
			}

			apps = append(apps, aContracts)
			if app.IsDaveConsensus() {
				daveConsensusApps = append(daveConsensusApps, aContracts)
			} else {
				iconsensusApps = append(iconsensusApps, aContracts)
			}
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

			r.Logger.Debug(fmt.Sprintf(
				"Using block %d and not %d because of commitment policy: %s",
				mostRecentHeader.Number.Uint64(),
				header.Number.Uint64(), r.defaultBlock))
		}

		r.checkForEpochsAndInputs(ctx, daveConsensusApps, blockNumber)

		r.checkForNewInputs(ctx, iconsensusApps, blockNumber)

		r.checkForOutputExecution(ctx, apps, blockNumber)
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
		return nil, fmt.Errorf("failed to retrieve header: %w", err)
	}

	if header == nil {
		return nil, fmt.Errorf("returned header is nil")
	}
	return header, nil
}

type AdapterFactory interface {
	CreateAdapters(app *Application) (ApplicationContractAdapter, InputSourceAdapter, DaveConsensusAdapter, error)
}

type DefaultAdapterFactory struct {
	Client *ethclient.Client
	Filter ethutil.Filter
}

func (f *DefaultAdapterFactory) CreateAdapters(app *Application) (ApplicationContractAdapter, InputSourceAdapter, DaveConsensusAdapter, error) {
	if app == nil {
		return nil, nil, nil, fmt.Errorf("application reference is nil, should never happen")
	}

	applicationContract, err := NewApplicationContractAdapter(app.IApplicationAddress, f.Client, f.Filter)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error building application contract: %w", err)
	}

	var inputSource InputSourceAdapter
	if app.HasDataAvailabilitySelector(DataAvailability_InputBox) {
		inputSource, err = NewInputSourceAdapter(app.IInputBoxAddress, f.Client, f.Filter)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error building inputbox contract: %w", err)
		}
	}

	var daveConsensus DaveConsensusAdapter
	if app.IsDaveConsensus() {
		daveConsensus, err = NewDaveConsensusAdapter(app.IConsensusAddress, f.Client, f.Filter)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error building daveconsensus contract: %w", err)
		}
	}

	return applicationContract, inputSource, daveConsensus, nil
}
