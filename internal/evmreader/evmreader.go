// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

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
	UpdateApplicationForeclosure(
		ctx context.Context,
		appID int64,
		block uint64,
		txHash common.Hash,
		blockNumber uint64,
	) error
	UpdateApplicationLastForecloseCheckBlock(ctx context.Context, appID int64, blockNumber uint64) error
	UpdateAccountsDriveProved(
		ctx context.Context,
		appID int64,
		block uint64,
		txHash common.Hash,
		root common.Hash,
		blockNumber uint64,
	) error
	UpdateApplicationLastAccountsDriveProvedCheckBlock(ctx context.Context, appID int64, blockNumber uint64) error
	StoreWithdrawalEvents(
		ctx context.Context,
		appID int64,
		withdrawals []*Withdrawal,
		blockNumber uint64,
	) error
	GetNumberOfWithdrawals(ctx context.Context, appID int64) (uint64, error)
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
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
	ListEpochs(ctx context.Context, nameOrAddress string, f repository.EpochFilter,
		p repository.Pagination, descending bool) ([]*Epoch, uint64, error)
	UpdateEpochClaimTransactionHash(ctx context.Context, nameOrAddress string, e *Epoch) error
	GetLastNonOpenEpoch(ctx context.Context, nameOrAddress string) (*Epoch, error)

	GetNumberOfInputs(ctx context.Context, nameOrAddress string) (uint64, error)

	// Output execution monitor
	GetOutput(ctx context.Context, nameOrAddress string, indexKey uint64) (*Output, error)
	UpdateOutputsExecution(ctx context.Context, nameOrAddress string, executedOutputs []*Output, blockNumber uint64) error
	GetNumberOfExecutedOutputs(ctx context.Context, nameOrAddress string) (uint64, error)
	GetNumberOfPendingExecutableOutputs(ctx context.Context, nameOrAddress string) (uint64, error)
}

// EthClientInterface defines the methods we need from ethclient.Client
type EthClientInterface interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	ChainID(ctx context.Context) (*big.Int, error)
}

// Internal struct to hold application and it's contracts together
type appContracts struct {
	application         *Application
	applicationContract ApplicationContractAdapter
	inputSource         InputSourceAdapter
	daveConsensus       DaveConsensusAdapter
}

func listEnabledApplications(ctx context.Context, er EvmReaderRepository) ([]*Application, uint64, error) {
	return er.ListApplications(ctx, repository.ApplicationFilter{Enabled: new(true)}, repository.Pagination{}, false)
}

func (r *Service) setApplicationDiverged(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetDivergedf(ctx, r.Logger, r.repository, app, reasonFmt, args...)
}

func (r *Service) setApplicationCorrupted(ctx context.Context, app *Application, reasonFmt string, args ...any) error {
	return appstatus.SetCorruptedf(ctx, r.Logger, r.repository, app, reasonFmt, args...)
}

func (r *Service) Tick() []error {
	blockNumber, err := r.fetchMostRecentHeader(r.Context, r.defaultBlock)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return []error{err}
	}

	if blockNumber != r.lastBlockNumber.Load() {
		r.lastBlockNumber.Store(blockNumber)
		r.Logger.Info("Got new block header", "block", blockNumber, "policy", r.defaultBlock)
	}

	// Scans run under the service context: cancellable on shutdown, free to take
	// as long as catch-up needs. Per-request bounds live on the HTTP transport.
	r.processBlockHead(r.Context, blockNumber, r.resolver)

	return nil
}

func (r *Service) processBlockHead(
	ctx context.Context,
	blockNumber uint64,
	resolver *applicationAdapterResolver,
) {
	r.Logger.Debug("Retrieving enabled applications")
	observableApps, _, err := listEnabledApplications(ctx, r.repository)
	if err != nil {
		r.Logger.Error("Error retrieving L1-observable applications", "error", err)
		return
	}

	if len(observableApps) == 0 {
		if r.hasEnabledApps {
			r.Logger.Info("No registered applications enabled for L1 observation")
		}
		r.hasEnabledApps = false
		return
	}
	if !r.hasEnabledApps {
		r.Logger.Info("Found applications enabled for L1 observation")
	}
	r.hasEnabledApps = true

	apps := resolver.buildAppContracts(observableApps)
	if len(apps) == 0 {
		r.Logger.Info("No correctly configured applications running")
		return
	}

	r.runBlockScanners(ctx, apps, blockNumber)
}

func (r *Service) runBlockScanners(
	ctx context.Context,
	apps []appContracts,
	blockNumber uint64,
) {
	// Detect foreclosure first so later scanners use the marker observed in this same tick.
	r.checkForForeclosure(ctx, apps, blockNumber)

	plan := buildBlockScanPlan(apps)

	r.scanDaveConsensusEpochsAndInputs(ctx, plan.daveEpochTargets, blockNumber)
	r.scanIConsensusInputs(ctx, plan.iConsensusInputTargets, blockNumber)

	r.checkForOutputExecution(ctx, plan.outputTargets, blockNumber)

	// Post-foreclosure observation dispatches to drive-prove discovery or withdrawal indexing.
	r.checkPostForeclosure(ctx, plan.postForeclosureTargets, blockNumber)
}

// fetchMostRecentHeader fetches the most recent header up till the
// given default block
func (r *Service) fetchMostRecentHeader(
	ctx context.Context,
	defaultBlock DefaultBlock,
) (uint64, error) {

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
		return 0, fmt.Errorf("default block '%v' not supported", defaultBlock)
	}

	header, err :=
		r.client.HeaderByNumber(
			ctx,
			new(big.Int).SetInt64(defaultBlockNumber))
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve header: %w", err)
	}

	if header == nil {
		return 0, fmt.Errorf("returned header is nil")
	}
	if header.Number == nil {
		return 0, fmt.Errorf("returned header number is nil")
	}
	return header.Number.Uint64(), nil
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
