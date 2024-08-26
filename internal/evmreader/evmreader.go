// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"

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
	RetrieveInputs(opts *bind.FilterOpts, appContract []common.Address, index []*big.Int,
	) ([]inputbox.InputBoxInputAdded, error)
}

// Interface for the node repository
type EvmReaderRepository interface {
	StoreEpochAndInputsTransaction(
		ctx context.Context, epochInputMap map[*Epoch][]Input, blockNumber uint64,
		appAddress Address,
	) (epochIndexIdMap map[uint64]uint64, epochIndexInputIdsMap map[uint64][]uint64, err error)

	GetAllRunningApplications(ctx context.Context) ([]Application, error)
	GetNodeConfig(ctx context.Context) (*NodePersistentConfig, error)
	GetEpoch(ctx context.Context, indexKey uint64, appAddressKey Address) (*Epoch, error)
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
}

type ApplicationContract interface {
	GetConsensus(opts *bind.CallOpts) (Address, error)
}

type ContractFactory interface {
	NewApplication(address Address) (ApplicationContract, error)
	NewIConsensus(address Address) (ConsensusContract, error)
}

type SubscriptionError struct {
	Cause error
}

func (e *SubscriptionError) Error() string {
	return fmt.Sprintf("Subscription error : %v", e.Cause)
}

// EvmReader reads inputs from the blockchain
type EvmReader struct {
	client                  EthClient
	wsClient                EthWsClient
	inputSource             InputSource
	repository              EvmReaderRepository
	contractFactory         ContractFactory
	inputBoxDeploymentBlock uint64
	defaultBlock            DefaultBlock
	epochLengthCache        map[Address]uint64
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
	inputBoxDeploymentBlock uint64,
	defaultBlock DefaultBlock,
	contractFactory ContractFactory,
) EvmReader {
	return EvmReader{
		client:                  client,
		wsClient:                wsClient,
		inputSource:             inputSource,
		repository:              repository,
		inputBoxDeploymentBlock: inputBoxDeploymentBlock,
		defaultBlock:            defaultBlock,
		contractFactory:         contractFactory,
	}
}

func (r *EvmReader) Run(ctx context.Context, ready chan<- struct{}) error {

	// Initialize epochLength cache
	r.epochLengthCache = make(map[Address]uint64)

	for {
		err := r.watchForNewBlocks(ctx, ready)
		// If the error is a SubscriptionError, re run watchForNewBlocks
		// that it will restart the websocket subscription
		if _, ok := err.(*SubscriptionError); !ok {
			return err
		}
		slog.Error(err.Error())
		slog.Info("Restarting subscription")
	}
}

// Watch for new blocks and reads new inputs based on the
// default block configuration, which have not been processed yet.
func (r *EvmReader) watchForNewBlocks(ctx context.Context, ready chan<- struct{}) error {
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
				slog.Error("Error checking for new inputs",
					"error",
					err,
				)
			}

		}
	}
}

// Check if is there new Inputs for all running Applications
func (r *EvmReader) checkForNewInputs(ctx context.Context) error {

	slog.Debug("Checking for new inputs")

	// Get All Applications
	apps, err := r.repository.GetAllRunningApplications(ctx)
	if err != nil {
		return err
	}

	if len(apps) == 0 {
		slog.Info("No running applications")
		return nil
	}

	groupedApps := r.classifyApplicationsByLastProcessedInput(apps)

	for lastProcessedBlock, apps := range groupedApps {

		appAddresses := appToAddresses(apps)

		// Safeguard: Only check blocks starting from the block where the InputBox
		// contract was deployed as Inputs can be added to that same block
		if lastProcessedBlock < r.inputBoxDeploymentBlock {
			lastProcessedBlock = r.inputBoxDeploymentBlock - 1
		}

		currentMostRecentFinalizedHeader, err := r.fetchMostRecentHeader(
			ctx,
			r.defaultBlock,
		)
		if err != nil {
			slog.Error("Error fetching most recent block",
				"default block", r.defaultBlock,
				"error", err)
			continue
		}
		currentMostRecentFinalizedBlockNumber := currentMostRecentFinalizedHeader.Number.Uint64()

		if currentMostRecentFinalizedBlockNumber > lastProcessedBlock {

			slog.Info("Checking inputs for applications",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", currentMostRecentFinalizedBlockNumber,
			)

			err = r.readAndStoreInputs(ctx,
				lastProcessedBlock+1,
				currentMostRecentFinalizedBlockNumber,
				apps,
			)
			if err != nil {
				slog.Error("Error reading inputs",
					"apps", appAddresses,
					"last processed block", lastProcessedBlock,
					"most recent block", currentMostRecentFinalizedBlockNumber,
					"error", err,
				)
				continue
			}
		} else if currentMostRecentFinalizedBlockNumber < lastProcessedBlock {
			slog.Warn(
				"Current most recent block is lower than the last processed one",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", currentMostRecentFinalizedBlockNumber,
			)
		} else {
			slog.Info("Already checked the most recent blocks",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", currentMostRecentFinalizedBlockNumber,
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

// Read and store inputs from the InputSource given specific filter options.
func (r *EvmReader) readAndStoreInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	apps []Application,
) error {
	appsToProcess := []common.Address{}

	for _, app := range apps {

		// Get App EpochLength
		err := r.addAppEpochLengthIntoCache(app)
		if err != nil {
			slog.Error("Error adding epoch length into cache",
				"app", app.ContractAddress,
				"error", err)
			continue
		}

		appsToProcess = append(appsToProcess, app.ContractAddress)

	}

	if len(appsToProcess) == 0 {
		slog.Warn("No valid running applications")
		return nil
	}

	// Retrieve Inputs from blockchain
	appInputsMap, err := r.readInputsFromBlockchain(ctx, appsToProcess, startBlock, endBlock)
	if err != nil {
		return fmt.Errorf("failed to read inputs from block %v to block %v. %w",
			startBlock,
			endBlock,
			err)
	}

	// Index Inputs into epochs and handle epoch finalization
	for address, inputs := range appInputsMap {

		epochLength := r.epochLengthCache[address]

		// Retrieves last open epoch from DB
		currentEpoch, err := r.repository.GetEpoch(ctx,
			calculateEpochIndex(epochLength, startBlock), address)
		if err != nil {
			slog.Error("Error retrieving existing current epoch",
				"app", address,
				"error", err,
			)
			continue
		}

		// Check current epoch status
		if currentEpoch != nil && currentEpoch.Status != EpochStatusOpen {
			slog.Error("Current epoch is not open",
				"app", address,
				"epoch-index", currentEpoch.Index,
				"status", currentEpoch.Status,
			)
			continue
		}

		// Initialize epochs inputs map
		var epochInputMap = make(map[*Epoch][]Input)

		// Index Inputs into epochs
		for _, input := range inputs {

			inputEpochIndex := calculateEpochIndex(epochLength, input.BlockNumber)

			// If input belongs into a new epoch, close the previous known one
			if currentEpoch != nil && currentEpoch.Index != inputEpochIndex {
				currentEpoch.Status = EpochStatusClosed
				slog.Info("Closing epoch",
					"app", currentEpoch.AppAddress,
					"epoch-index", currentEpoch.Index,
					"start", currentEpoch.FirstBlock,
					"end", currentEpoch.LastBlock)
				// Add it to inputMap, so it will be stored
				epochInputMap[currentEpoch] = []Input{}
				currentEpoch = nil
			}
			if currentEpoch == nil {
				currentEpoch = &Epoch{
					Index:      inputEpochIndex,
					FirstBlock: inputEpochIndex * epochLength,
					LastBlock:  (inputEpochIndex * epochLength) + epochLength - 1,
					Status:     EpochStatusOpen,
					AppAddress: address,
				}
			}

			slog.Info("Indexing new Input into epoch",
				"app", address,
				"index", input.Index,
				"block", input.BlockNumber,
				"epoch-index", inputEpochIndex)

			currentInputs, ok := epochInputMap[currentEpoch]
			if !ok {
				currentInputs = []Input{}
			}
			epochInputMap[currentEpoch] = append(currentInputs, *input)

		}

		// Indexed all inputs. Check if it is time to close this epoch
		if currentEpoch != nil && endBlock >= currentEpoch.LastBlock {
			currentEpoch.Status = EpochStatusClosed
			slog.Info("Closing epoch",
				"app", currentEpoch.AppAddress,
				"epoch-index", currentEpoch.Index,
				"start", currentEpoch.FirstBlock,
				"end", currentEpoch.LastBlock)
			// Add to inputMap so it is stored
			_, ok := epochInputMap[currentEpoch]
			if !ok {
				epochInputMap[currentEpoch] = []Input{}
			}
		}

		_, _, err = r.repository.StoreEpochAndInputsTransaction(
			ctx,
			epochInputMap,
			endBlock,
			address,
		)
		if err != nil {
			slog.Error("Error storing inputs and epochs",
				"app", address,
				"error", err,
			)
			continue
		}

		// Store everything
		if len(epochInputMap) > 0 {

			slog.Debug("Inputs and epochs stored successfully",
				"app", address,
				"start-block", startBlock,
				"end-block", endBlock,
				"total epochs", len(epochInputMap),
				"total inputs", len(inputs),
			)
		} else {
			slog.Debug("No inputs or epochs to store")
		}

	}

	return nil
}

// Checks the epoch length cache and read epoch length from IConsensus
// and add it to the cache if needed
func (r *EvmReader) addAppEpochLengthIntoCache(app Application) error {

	epochLength, ok := r.epochLengthCache[app.ContractAddress]
	if !ok {

		consensus, err := r.getIConsensus(app)
		if err != nil {
			return errors.Join(
				fmt.Errorf("error retrieving IConsensus contract for app: %s",
					app.ContractAddress),
				err)
		}

		epochLength, err = r.getEpochLengthFromContract(consensus)
		if err != nil {
			return errors.Join(
				fmt.Errorf("error retrieving epoch length from contracts for app %s",
					app.ContractAddress),
				err)
		}
		r.epochLengthCache[app.ContractAddress] = epochLength
		slog.Info("Got epoch length from IConsensus",
			"app", app.ContractAddress,
			"epoch length", epochLength)
	} else {
		slog.Debug("Got epoch length from cache",
			"app", app.ContractAddress,
			"epoch length", epochLength)
	}

	return nil
}

// Retrieve ConsensusContract for a given Application
func (r *EvmReader) getIConsensus(app Application) (ConsensusContract, error) {
	applicationContract, err := r.contractFactory.NewApplication(app.ContractAddress)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("error building application contract"),
			err,
		)

	}
	consensusAddress, err := applicationContract.GetConsensus(nil)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("error retrieving application consensus"),
			err,
		)
	}

	if app.IConsensusAddress != consensusAddress {
		return nil,
			fmt.Errorf("IConsensus addresses do not match. Deployed: %s. Configured: %s",
				consensusAddress,
				app.IConsensusAddress)
	}

	consensus, err := r.contractFactory.NewIConsensus(consensusAddress)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("error building consensus contract"),
			err,
		)

	}
	return consensus, nil
}

// Reads the application epoch length given it's consesus contract
func (r *EvmReader) getEpochLengthFromContract(consensus ConsensusContract) (uint64, error) {

	epochLengthRaw, err := consensus.GetEpochLength(nil)
	if err != nil {
		return 0, errors.Join(
			fmt.Errorf("error retrieving application epoch length"),
			err,
		)
	}

	return epochLengthRaw.Uint64(), nil
}

// Read inputs from the blockchain ordered by Input index
func (r *EvmReader) readInputsFromBlockchain(
	ctx context.Context,
	appsAddresses []Address,
	startBlock, endBlock uint64,
) (map[Address][]*Input, error) {

	// Initialize app input map
	var appInputsMap = make(map[Address][]*Input)
	for _, appsAddress := range appsAddresses {
		appInputsMap[appsAddress] = []*Input{}
	}

	opts := bind.FilterOpts{
		Context: ctx,
		Start:   startBlock,
		End:     &endBlock,
	}
	inputsEvents, err := r.inputSource.RetrieveInputs(&opts, appsAddresses, nil)
	if err != nil {
		return nil, err
	}

	// Order inputs as order is not enforced by RetrieveInputs method nor the APIs
	for _, event := range inputsEvents {
		slog.Debug("Received input",
			"app", event.AppContract,
			"index", event.Index,
			"block", event.Raw.BlockNumber)
		input := &Input{
			Index:            event.Index.Uint64(),
			CompletionStatus: InputStatusNone,
			RawData:          event.Input,
			BlockNumber:      event.Raw.BlockNumber,
			AppAddress:       event.AppContract,
		}

		// Insert Sorted
		appInputsMap[event.AppContract] = insertSorted(appInputsMap[event.AppContract], input)
	}
	return appInputsMap, nil
}

// Util functions

// Calculates the epoch index given the input block number
func calculateEpochIndex(epochLength uint64, blockNumber uint64) uint64 {
	return blockNumber / epochLength
}

func appToAddresses(apps []Application) []Address {
	var addresses []Address
	for _, app := range apps {
		addresses = append(addresses, app.ContractAddress)
	}
	return addresses
}

// insertSorted inserts the received input in the slice at the position defined
// by its index property.
func insertSorted(inputs []*Input, input *Input) []*Input {
	// Insert Sorted
	i, _ := slices.BinarySearchFunc(
		inputs,
		input,
		func(a, b *Input) int {
			return cmp.Compare(a.Index, b.Index)
		})
	return slices.Insert(inputs, i, input)
}
