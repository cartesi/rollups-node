// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// checkForNewInputs checks if is there new Inputs for all running Applications
func (r *Service) checkForNewInputs(
	ctx context.Context,
	apps []appContracts,
	mostRecentBlockNumber uint64,
) {
	if !r.inputReaderEnabled {
		return
	}

	r.Logger.Debug("Checking for new inputs")

	groupedApps := indexApps(byLastProcessedBlock, apps)

	for lastProcessedBlock, apps := range groupedApps {

		appAddresses := appsToAddresses(apps)

		// Safeguard: Only check blocks starting from the block where the InputBox
		// contract was deployed as Inputs can be added to that same block
		if lastProcessedBlock < r.inputBoxDeploymentBlock {
			lastProcessedBlock = r.inputBoxDeploymentBlock - 1
		}

		if mostRecentBlockNumber > lastProcessedBlock {

			r.Logger.Debug("Checking inputs for applications",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", mostRecentBlockNumber,
			)

			err := r.readAndStoreInputs(ctx,
				lastProcessedBlock+1,
				mostRecentBlockNumber,
				apps,
			)
			if err != nil {
				r.Logger.Error("Error reading inputs",
					"apps", appAddresses,
					"last processed block", lastProcessedBlock,
					"most recent block", mostRecentBlockNumber,
					"error", err,
				)
				continue
			}
		} else if mostRecentBlockNumber < lastProcessedBlock {
			r.Logger.Warn(
				"Not reading inputs: most recent block is lower than the last processed one",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", mostRecentBlockNumber,
			)
		} else {
			r.Logger.Info("Not reading inputs: already checked the most recent blocks",
				"apps", appAddresses,
				"last processed block", lastProcessedBlock,
				"most recent block", mostRecentBlockNumber,
			)
		}
	}
}

// readAndStoreInputs reads, inputs from the InputSource given specific filter options, indexes
// them into epochs and store the indexed inputs and epochs
func (r *Service) readAndStoreInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	apps []appContracts,
) error {

	if len(apps) == 0 {
		r.Logger.Warn("No valid running applications")
		return nil
	}

	// Retrieve Inputs from blockchain
	appInputsMap, err := r.readInputsFromBlockchain(ctx, apps, startBlock, endBlock)
	if err != nil {
		return fmt.Errorf("failed to read inputs from block %v to block %v. %w",
			startBlock,
			endBlock,
			err)
	}

	addrToApp := mapAddressToApp(apps)

	// Index Inputs into epochs and handle epoch finalization
	for address, inputs := range appInputsMap {

		app, exists := addrToApp[address]
		if !exists {
			r.Logger.Error("Application address on input not found",
				"address", address)
			continue
		}
		epochLength := app.application.EpochLength

		// Retrieves last open epoch from DB
		currentEpoch, err := r.repository.GetEpoch(ctx, address.String(), calculateEpochIndex(epochLength, startBlock))
		if err != nil {
			r.Logger.Error("Error retrieving existing current epoch",
				"application", app.application.Name,
				"address", address,
				"error", err,
			)
			continue
		}

		// Check current epoch status
		if currentEpoch != nil && currentEpoch.Status != EpochStatus_Open {
			r.Logger.Error("Current epoch is not open",
				"application", app.application.Name,
				"address", address,
				"epoch_index", currentEpoch.Index,
				"status", currentEpoch.Status,
			)
			continue
		}

		// Initialize epochs inputs map
		var epochInputMap = make(map[*Epoch][]*Input)

		// Index Inputs into epochs
		for _, input := range inputs {

			inputEpochIndex := calculateEpochIndex(epochLength, input.BlockNumber)

			// If input belongs into a new epoch, close the previous known one
			if currentEpoch != nil && currentEpoch.Index != inputEpochIndex {
				currentEpoch.Status = EpochStatus_Closed
				r.Logger.Info("Closing epoch",
					"application", app.application.Name,
					"address", address,
					"epoch_index", currentEpoch.Index,
					"start", currentEpoch.FirstBlock,
					"end", currentEpoch.LastBlock)
				_, ok := epochInputMap[currentEpoch]
				if !ok {
					epochInputMap[currentEpoch] = []*Input{}
				}
				currentEpoch = nil
			}
			if currentEpoch == nil {
				currentEpoch = &Epoch{
					Index:      inputEpochIndex,
					FirstBlock: inputEpochIndex * epochLength,
					LastBlock:  (inputEpochIndex * epochLength) + epochLength - 1,
					Status:     EpochStatus_Open,
				}
				epochInputMap[currentEpoch] = []*Input{}
			}

			r.Logger.Info("Found new Input",
				"application", app.application.Name,
				"address", address,
				"index", input.Index,
				"block", input.BlockNumber,
				"epoch_index", inputEpochIndex)

			currentInputs, ok := epochInputMap[currentEpoch]
			if !ok {
				currentInputs = []*Input{}
			}
			epochInputMap[currentEpoch] = append(currentInputs, input)

		}

		// Indexed all inputs. Check if it is time to close this epoch
		if currentEpoch != nil && endBlock >= currentEpoch.LastBlock {
			currentEpoch.Status = EpochStatus_Closed
			r.Logger.Info("Closing epoch",
				"application", app.application.Name,
				"address", address,
				"epoch_index", currentEpoch.Index,
				"start", currentEpoch.FirstBlock,
				"end", currentEpoch.LastBlock)
			// Add to inputMap so it is stored
			_, ok := epochInputMap[currentEpoch]
			if !ok {
				epochInputMap[currentEpoch] = []*Input{}
			}
		}

		err = r.repository.CreateEpochsAndInputs(
			ctx,
			address.String(),
			epochInputMap,
			endBlock,
		)
		if err != nil {
			r.Logger.Error("Error storing inputs and epochs",
				"application", app.application.Name,
				"address", address,
				"error", err,
			)
			continue
		}

		// Store everything
		if len(epochInputMap) > 0 {
			r.Logger.Debug("Inputs and epochs stored successfully",
				"application", app.application.Name,
				"address", address,
				"start-block", startBlock,
				"end-block", endBlock,
				"total epochs", len(epochInputMap),
				"total inputs", len(inputs),
			)
		} else {
			r.Logger.Debug("No inputs or epochs to store")
		}

	}

	return nil
}

// readInputsFromBlockchain read the inputs from the blockchain ordered by Input index
func (r *Service) readInputsFromBlockchain(
	ctx context.Context,
	apps []appContracts,
	startBlock, endBlock uint64,
) (map[common.Address][]*Input, error) {

	// Initialize app input map
	var appInputsMap = make(map[common.Address][]*Input)
	var appsAddresses = []common.Address{}
	for _, app := range apps {
		appInputsMap[app.application.IApplicationAddress] = []*Input{}
		appsAddresses = append(appsAddresses, app.application.IApplicationAddress)
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
		r.Logger.Debug("Received input",
			"address", event.AppContract,
			"index", event.Index,
			"block", event.Raw.BlockNumber)
		input := &Input{
			Index:                event.Index.Uint64(),
			Status:               InputCompletionStatus_None,
			RawData:              event.Input,
			BlockNumber:          event.Raw.BlockNumber,
			TransactionReference: common.BigToHash(event.Index),
		}

		// Insert Sorted
		appInputsMap[event.AppContract] = insertSorted(
			sortByInputIndex, appInputsMap[event.AppContract], input)
	}
	return appInputsMap, nil
}

// byLastProcessedBlock key extractor function intended to be used with `indexApps` function
func byLastProcessedBlock(app appContracts) uint64 {
	return app.application.LastProcessedBlock
}
