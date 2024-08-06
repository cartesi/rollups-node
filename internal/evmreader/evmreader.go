// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/inputbox"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// Interface for Input reading
type InputSource interface {
	// Wrapper for FilterInputAdded(), which is automatically generated
	// by go-ethereum and cannot be used for testing
	RetrieveInputs(
		opts *bind.FilterOpts,
		appContract []common.Address,
		index []*big.Int,
	) ([]inputbox.InputBoxInputAdded, error)
}

// Interface for the node repository
type EvmReaderRepository interface {
	InsertInputsAndUpdateLastProcessedBlock(
		ctx context.Context,
		inputs []Input,
		blockNumber uint64,
		appAddress common.Address,
	) error
	GetAllRunningApplications(
		ctx context.Context,
	) ([]Application, error)
	GetNodeConfig(
		ctx context.Context,
	) (*NodePersistentConfig, error)
	GetEpoch(
		ctx context.Context,
		indexKey uint64,
		appAddressKey common.Address,
	) (*Epoch, error)
	InsertEpoch(
		ctx context.Context,
		epoch *Epoch,
	) (uint64, error)
}

// EthClient mimics part of ethclient.Client functions to narrow down the
// interface needed by the EvmReader. It must be bound to an HTTP endpoint
type EthClient interface {
	HeaderByNumber(
		ctx context.Context,
		number *big.Int,
	) (*types.Header, error)
}

// EthWsClient mimics part of ethclient.Client functions to narrow down the
// interface needed by the EvmReader. It must be bound to a WS endpoint
type EthWsClient interface {
	SubscribeNewHead(
		ctx context.Context,
		ch chan<- *types.Header,
	) (ethereum.Subscription, error)
}

type SubscriptionError struct {
	Cause error
}

func (e *SubscriptionError) Error() string {
	return fmt.Sprintf("Subscription error : %v", e.Cause)
}

// EvmReader reads inputs from the blockchain
type EvmReader struct {
	client      EthClient
	wsClient    EthWsClient
	inputSource InputSource
	repository  EvmReaderRepository
	config      NodePersistentConfig
}

func (r *EvmReader) String() string {
	return "evm-reader"
}

// Creates a new EvmReader
func NewEvmReader(
	client EthClient,
	wsClient EthWsClient,
	inputSource InputSource,
	repository EvmReaderRepository,
	config NodePersistentConfig,
) EvmReader {
	return EvmReader{
		client:      client,
		wsClient:    wsClient,
		inputSource: inputSource,
		repository:  repository,
		config:      config,
	}
}

func (r *EvmReader) Run(
	ctx context.Context,
	ready chan<- struct{},
) error {

	for {
		watchForNewInputsError := r.watchForNewBlocks(ctx, ready)
		// If the error is a SubscriptionError restart watchForNewBlocks
		// that will restart the subscription
		if _, ok := watchForNewInputsError.(*SubscriptionError); !ok {
			return watchForNewInputsError
		}
		slog.Error(watchForNewInputsError.Error())
		slog.Info("Restarting subscription")
	}
}

// Watch for new blocks and reads new inputs based on the
// default block configuration, which have not been processed yet.
func (r *EvmReader) watchForNewBlocks(
	ctx context.Context,
	ready chan<- struct{},
) error {
	headers := make(chan *types.Header)
	sub, err := r.wsClient.SubscribeNewHead(ctx, headers)
	if err != nil {
		return fmt.Errorf("could not start subscription: %v", err)
	}
	slog.Info("Subscribed to new block events")
	ready <- struct{}{}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sub.Err():
			return &SubscriptionError{Cause: err}
		case <-headers:
			// Every time a new block arrives
			err = r.checkForNewInputs(ctx)
			if err != nil {
				slog.Error("Error checking got new inputs",
					"error",
					err,
				)
			}

		}
	}
}

// Check if is there new Inputs for all running Applications
func (r *EvmReader) checkForNewInputs(ctx context.Context) error {

	// Get All Applications
	apps, err := r.repository.GetAllRunningApplications(ctx)
	if err != nil {
		return err
	}

	groupedApps := r.classifyApplicationsByLastProcessedInput(apps)

	for lastProcessedBlock, apps := range groupedApps {

		// Safeguard: Only check blocks starting from the block where the InputBox
		// contract was deployed as Inputs can be added to that same block
		if lastProcessedBlock < r.config.InputBoxDeploymentBlock {
			lastProcessedBlock = r.config.InputBoxDeploymentBlock - 1
		}

		currentMostRecentFinalizedHeader, err := r.fetchMostRecentHeader(
			ctx,
			r.config.DefaultBlock,
		)
		if err != nil {
			slog.Error("Error fetching most recent block",
				"last default block",
				r.config.DefaultBlock,
				"error",
				err)
			continue
		}
		currentMostRecentFinalizedBlockNumber := currentMostRecentFinalizedHeader.Number.Uint64()

		if currentMostRecentFinalizedBlockNumber > lastProcessedBlock {

			err = r.readInputs(ctx,
				lastProcessedBlock+1,
				currentMostRecentFinalizedBlockNumber,
				apps,
			)
			if err != nil {
				slog.Error("Error reading inputs",
					"start",
					lastProcessedBlock+1,
					"end",
					currentMostRecentFinalizedBlockNumber,
					"error",
					err)
				continue
			}
		} else if lastProcessedBlock < currentMostRecentFinalizedBlockNumber {
			slog.Warn(
				"current most recent block is lower than the last processed one",
				"most recent block",
				currentMostRecentFinalizedBlockNumber,
				"last processed",
				lastProcessedBlock,
			)
		}
	}

	return nil
}

// Group Applications that have processed til the same block height
func (r *EvmReader) classifyApplicationsByLastProcessedInput(
	apps []Application,
) map[uint64][]Application {
	result := make(map[uint64][]Application)
	for _, app := range apps {
		result[app.LastProcessedBlock] = append(result[app.LastProcessedBlock], app)
	}

	return result
}

// Fetch the most recent header up till the
// given default block
func (r *EvmReader) fetchMostRecentHeader(
	ctx context.Context,
	defaultBlock DefaultBlock,
) (*types.Header, error) {

	var defaultBlockNumber int64
	switch defaultBlock {
	case DefaultBlockStatusPending:
		defaultBlockNumber = rpc.PendingBlockNumber.Int64()
	case DefaultBlockStatusLatest:
		defaultBlockNumber = rpc.LatestBlockNumber.Int64()
	case DefaultBlockStatusFinalized:
		defaultBlockNumber = rpc.FinalizedBlockNumber.Int64()
	case DefaultBlockStatusSafe:
		defaultBlockNumber = rpc.SafeBlockNumber.Int64()
	default:
		return nil, fmt.Errorf("Default block '%v' not supported", defaultBlock)
	}

	header, err :=
		r.client.HeaderByNumber(
			ctx,
			new(big.Int).SetInt64(defaultBlockNumber))
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve header. %v", err)
	}

	if header == nil {
		return nil, fmt.Errorf("Returned header is nil")
	}
	return header, nil
}

// Read inputs from the InputSource given specific filter options.
func (r *EvmReader) readInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	apps []Application,
) error {
	filter := []common.Address{}

	var inputsMap = make(map[common.Address][]Input)
	for _, app := range apps {
		filter = append(filter, app.ContractAddress)
		inputsMap[app.ContractAddress] = []Input{}
	}

	opts := bind.FilterOpts{
		Context: ctx,
		Start:   startBlock,
		End:     &endBlock,
	}

	inputsEvents, err := r.inputSource.RetrieveInputs(&opts, filter, nil)
	if err != nil {
		return fmt.Errorf("Failed to read inputs from block %v to block %v. %v",
			startBlock,
			endBlock,
			err)
	}

	for _, event := range inputsEvents {
		slog.Debug("received input ", "app", event.AppContract, "index", event.Index)
		input := Input{
			Index:            event.Index.Uint64(),
			CompletionStatus: InputStatusNone,
			RawData:          event.Input,
			BlockNumber:      event.Raw.BlockNumber,
			AppAddress:       event.AppContract,
		}
		inputsMap[event.AppContract] = append(inputsMap[event.AppContract], input)
	}

	for address, inputs := range inputsMap {
		if len(inputs) > 0 {
			slog.Debug("Storing Inputs",
				"app", address,
				"start-block",
				startBlock,
				"end-block",
				endBlock,
				"total",
				len(inputs),
			)
		}
		err = r.repository.InsertInputsAndUpdateLastProcessedBlock(
			ctx,
			inputs,
			endBlock,
			address,
		)
		if err != nil {
			slog.Error("Error inserting inputs",
				"app",
				address,
				"error",
				err,
			)
			continue
		}
		if len(inputs) > 0 {
			slog.Info(
				"Inputs stored successfully",
				"app",
				address,
				"start-block",
				startBlock,
				"end-block",
				endBlock,
				"total",
				len(inputs),
			)
		}
	}

	return nil
}
