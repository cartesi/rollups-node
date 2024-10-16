// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// checkForNewInputs checks if is there new Inputs for all running Applications
func (r *EvmReader) checkForNewInputs(
	ctx context.Context,
	apps []application,
	mostRecentBlockNumber uint64,
) {

	slog.Debug("Checking for new inputs")

	groupedApps := indexApps(byLastProcessedBlock, apps)

	for lastProcessedBlock, apps := range groupedApps {

		appAddresses := appsToAddresses(apps)

		// Safeguard: Only check blocks starting from the block where the InputBox
		// contract was deployed as Inputs can be added to that same block
		if lastProcessedBlock < r.inputBoxDeploymentBlock {
			lastProcessedBlock = r.inputBoxDeploymentBlock - 1
		}

		if mostRecentBlockNumber > lastProcessedBlock {

			slog.Info("Checking inputs for applications",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", mostRecentBlockNumber,
			)

			err := r.ReadAndStoreInputs(ctx,
				lastProcessedBlock+1,
				mostRecentBlockNumber,
				apps,
			)
			if err != nil {
				slog.Error("Error reading inputs",
					"apps", appAddresses,
					"last processed block", lastProcessedBlock,
					"most recent block", mostRecentBlockNumber,
					"error", err,
				)
				continue
			}
		} else if mostRecentBlockNumber < lastProcessedBlock {
			slog.Warn(
				"Not reading inputs: most recent block is lower than the last processed one",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", mostRecentBlockNumber,
			)
		} else {
			slog.Info("Not reading inputs: already checked the most recent blocks",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", mostRecentBlockNumber,
			)
		}
	}
}

type TypeExportApplication = application

// ReadAndStoreInputs reads, inputs from the InputSource given specific filter options, indexes
// them into epochs and store the indexed inputs and epochs
func (r *EvmReader) ReadAndStoreInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	apps []TypeExportApplication,
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
			CalculateEpochIndex(epochLength, startBlock), address)
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

			inputEpochIndex := CalculateEpochIndex(epochLength, input.BlockNumber)

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

// addAppEpochLengthIntoCache checks the epoch length cache and read epoch length from IConsensus
// contract and add it to the cache if needed
func (r *EvmReader) addAppEpochLengthIntoCache(app application) error {

	epochLength, ok := r.epochLengthCache[app.ContractAddress]
	if !ok {

		epochLength, err := getEpochLength(app.ConsensusContract)
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

// readInputsFromBlockchain read the inputs from the blockchain ordered by Input index
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
			TransactionId:    event.Index.Bytes(),
		}

		// Insert Sorted
		appInputsMap[event.AppContract] = insertSorted(
			sortByInputIndex, appInputsMap[event.AppContract], input)
	}
	return appInputsMap, nil
}

// byLastProcessedBlock key extractor function intended to be used with `indexApps` function
func byLastProcessedBlock(app application) uint64 {
	return app.LastProcessedBlock
}

// getEpochLength reads the application epoch length given it's consensus contract
func getEpochLength(consensus ConsensusContract) (uint64, error) {

	epochLengthRaw, err := consensus.GetEpochLength(nil)
	if err != nil {
		return 0, errors.Join(
			fmt.Errorf("error retrieving application epoch length"),
			err,
		)
	}

	return epochLengthRaw.Uint64(), nil
}
