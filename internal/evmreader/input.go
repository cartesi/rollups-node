// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// Find the last block number that should be considered checked when scanning the blockchain
// for inputs of this application. The next search should start from (lastBlockChecked + 1).
//
// The main purpose of this function is to reduce the range of blocks that need to be scanned,
// avoiding unnecessary lookups. Currently, it is only used when handling new applications.
//
// Rules applied in order:
// 1. Default fallback: the block before the InputBox was deployed.
// 2. If the application has never received inputs, use the most recent block.
// 3. If the application has no inputs since its deployment, use the deployment block.
// 4. Otherwise, return the block before the InputBox deployment (conservative bound).
//
// Note: this function returns the *last block checked*, not the first block to search.
// Example:
//
//	lastBlockChecked := findLastInputCheckBlock(...)
//	nextSearchBlock  := lastBlockChecked + 1
func findLastInputCheckBlock(app *appContracts, mostRecentBlockNumber uint64, mostRecentBlockNumberCallOpts *bind.CallOpts) uint64 {
	var noiBig *big.Int
	var err error
	blockBeforeInputBox := app.application.IInputBoxBlock - 1

	// find if the application has ever received any input. sync to present if not
	noiBig, err = app.inputSource.GetNumberOfInputs(
		mostRecentBlockNumberCallOpts,
		app.application.IApplicationAddress,
	)
	if err != nil {
		return blockBeforeInputBox
	}
	if noiBig.Uint64() == 0 {
		return mostRecentBlockNumber
	}

	// find if the application has received an input since its deployment. sync to that block if not
	// we'll need its deployment block number to do that
	deploymentBlockNumberBig, err := app.applicationContract.GetDeploymentBlockNumber(mostRecentBlockNumberCallOpts)
	if err != nil {
		return blockBeforeInputBox
	}

	noiBig, err = app.inputSource.GetNumberOfInputs(&bind.CallOpts{
		BlockNumber: deploymentBlockNumberBig,
	},
		app.application.IApplicationAddress,
	)
	if err != nil {
		return blockBeforeInputBox
	}
	if noiBig.Uint64() == 0 {
		return deploymentBlockNumberBig.Uint64()
	}

	// TODO(mpolitzer): Application has inputs previous to its deployment. We can reduce the number of blocks to scan by
	// doing a binary search over GetNumberOfInputs and finding the block where 0 -> 1 transition happens. As a simpler,
	// also correct implementation. We return the first possible block an input could appear on.
	return blockBeforeInputBox
}

// initializeNewApplicationInputSync initializes input synchronization for a new application
// by finding the appropriate starting block and updating the database
func (r *Service) initializeNewApplicationInputSync(
	ctx context.Context,
	app *appContracts,
	mostRecentBlockNumber uint64,
	mostRecentBlockNumberCallOpts *bind.CallOpts,
) (uint64, error) {
	lastInputCheckBlock := findLastInputCheckBlock(app,
		mostRecentBlockNumber,
		mostRecentBlockNumberCallOpts,
	)

	err := r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_InputAdded, lastInputCheckBlock)
	if err != nil {
		r.Logger.Error("Failed to update application LastInputCheckBlock",
			"application", app.application.Name,
			"last_input_check_block", lastInputCheckBlock,
			"error", err,
		)
		return 0, err
	}

	r.Logger.Info("Initializing application input sync",
		"application", app.application.Name,
		"inputbox_block", app.application.IInputBoxBlock,
		"next_search_block", lastInputCheckBlock+1,
		"current_block", mostRecentBlockNumber,
	)

	app.application.LastInputCheckBlock = lastInputCheckBlock
	return lastInputCheckBlock, nil
}

// checkForNewInputs checks if is there new Inputs for all running Applications
func (r *Service) checkForNewInputs(
	ctx context.Context,
	applications []appContracts,
	mostRecentBlockNumber uint64,
) {
	if !r.inputReaderEnabled {
		return
	}

	r.Logger.Debug("Checking for new inputs")

	mostRecentBlockNumberCallOpts := &bind.CallOpts{
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	}

	appsByInputBox := map[common.Address][]appContracts{}
	for _, app := range applications {
		if !app.application.HasDataAvailabilitySelector(DataAvailability_InputBox) {
			continue
		}
		key := app.application.IInputBoxAddress
		appsByInputBox[key] = append(appsByInputBox[key], app)
	}

	for inputBoxAddress, inputBoxApps := range appsByInputBox {
		r.Logger.Debug("Checking inputs for applications with the same InputBox",
			"inputbox_address", inputBoxAddress,
			"most recent block", mostRecentBlockNumber,
		)

		appsByLastInputCheckBlock := make(map[uint64][]appContracts)
		for _, app := range inputBoxApps {
			lastInputCheckBlock := app.application.LastInputCheckBlock
			if lastInputCheckBlock == 0 { // New application. Find a safe start block to scan for inputs
				var err error
				lastInputCheckBlock, err = r.initializeNewApplicationInputSync(ctx, &app,
					mostRecentBlockNumber, mostRecentBlockNumberCallOpts)
				if err != nil {
					continue
				}
			}
			appsByLastInputCheckBlock[lastInputCheckBlock] = append(appsByLastInputCheckBlock[lastInputCheckBlock], app)
		}

		for lastProcessedBlock, apps := range appsByLastInputCheckBlock {
			appAddresses := appsToAddresses(apps)

			if mostRecentBlockNumber > lastProcessedBlock {

				r.Logger.Debug("Checking inputs for applications",
					"apps", appAddresses,
					"last_processed_block", lastProcessedBlock,
					"most_recent_block", mostRecentBlockNumber,
				)

				err := r.readAndStoreInputs(ctx,
					lastProcessedBlock,
					mostRecentBlockNumber,
					apps,
				)
				if err != nil {
					r.Logger.Error("Error reading inputs",
						"apps", appAddresses,
						"last_processed_block", lastProcessedBlock,
						"most_recent_block", mostRecentBlockNumber,
						"error", err,
					)
					continue
				}
			} else if mostRecentBlockNumber < lastProcessedBlock {
				r.Logger.Warn(
					"Input search skipped: most recent block is lower than the last processed one",
					"apps", appAddresses,
					"last_processed_block", lastProcessedBlock,
					"most_recent_block", mostRecentBlockNumber,
				)
			} else {
				r.Logger.Debug("Input search skipped: already checked the most recent block",
					"apps", appAddresses,
					"last_processed_block", lastProcessedBlock,
					"most_recent_block", mostRecentBlockNumber,
				)
			}
		}
	}
}

// readAndStoreInputs reads, inputs from the InputSource given specific filter options, indexes
// them into epochs and store the indexed inputs and epochs
func (r *Service) readAndStoreInputs(
	ctx context.Context,
	lastProcessedBlock uint64,
	mostRecentBlockNumber uint64,
	apps []appContracts,
) error {

	if len(apps) == 0 {
		r.Logger.Warn("No valid running applications")
		return nil
	}

	// Retrieve Inputs from blockchain
	nextSearchBlock := lastProcessedBlock + 1
	appInputsMap, err := r.readInputsFromBlockchain(ctx, apps, nextSearchBlock, mostRecentBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to read inputs from block %v to block %v. %w",
			nextSearchBlock,
			mostRecentBlockNumber,
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
		if epochLength == 0 {
			reason := "Application has epoch length of zero"
			r.Logger.Error(reason, "application", app.application.Name, "address", address)
			err := r.repository.UpdateApplicationState(ctx, app.application.ID, ApplicationState_Inoperable, &reason)
			if err != nil {
				r.Logger.Error("failed to update application state to inoperable", "application", app.application.Name, "err", err)
			}
			continue
		}

		// Retrieves last open epoch from DB
		currentEpoch, err := r.repository.GetEpoch(ctx, address.String(), calculateEpochIndex(epochLength, lastProcessedBlock))
		if err != nil {
			r.Logger.Error("Error retrieving existing current epoch",
				"application", app.application.Name,
				"address", address,
				"error", err,
			)
			continue
		}

		// Initialize epochs inputs map
		var epochInputMap = make(map[*Epoch][]*Input)
		// Index Inputs into epochs
		for _, input := range inputs {

			inputEpochIndex := calculateEpochIndex(epochLength, input.BlockNumber)

			// If input belongs into a new epoch, close the previous known one
			if currentEpoch != nil {
				r.Logger.Debug("Current epoch and new input",
					"application", app.application.Name,
					"address", address,
					"epoch_index", currentEpoch.Index,
					"epoch_status", currentEpoch.Status,
					"input_epoch_index", inputEpochIndex,
				)
				if currentEpoch.Index == inputEpochIndex {
					// Input can only be added to open epochs
					if currentEpoch.Status != EpochStatus_Open {
						reason := "Received inputs for an epoch that was not open. Should never happen"
						r.Logger.Error(reason,
							"application", app.application.Name,
							"address", address,
							"epoch_index", currentEpoch.Index,
							"status", currentEpoch.Status,
						)
						err := r.repository.UpdateApplicationState(ctx, app.application.ID, ApplicationState_Inoperable, &reason)
						if err != nil {
							r.Logger.Error("failed to update application state to inoperable", "application", app.application.Name, "err", err)
						}
						return errors.New(reason)
					}
					currentEpoch.InputIndexUpperBound = input.Index + 1
				} else {
					if currentEpoch.Status == EpochStatus_Open {
						currentEpoch.Status = EpochStatus_Closed
						currentEpoch.InputIndexUpperBound = input.Index
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
					}
					currentEpoch = nil
				}
			}
			if currentEpoch == nil {
				currentEpoch = &Epoch{
					Index:                inputEpochIndex,
					FirstBlock:           inputEpochIndex * epochLength,
					LastBlock:            (inputEpochIndex * epochLength) + epochLength - 1,
					InputIndexLowerBound: input.Index,
					InputIndexUpperBound: input.Index + 1,
					Status:               EpochStatus_Open,
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

		// Indexed all inputs. Check if it is time to close the last epoch
		if currentEpoch != nil && currentEpoch.Status == EpochStatus_Open &&
			mostRecentBlockNumber >= currentEpoch.LastBlock {
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
			mostRecentBlockNumber,
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
				"start-block", nextSearchBlock,
				"end-block", mostRecentBlockNumber,
				"total epochs", len(epochInputMap),
				"total inputs", len(inputs),
			)
		} else {
			r.Logger.Debug("No inputs or epochs to store")
		}

	}

	// Update LastInputCheckBlock for applications that didn't have any inputs
	// (for apps with inputs, LastInputCheckBlock is already updated in CreateEpochsAndInputs)
	appsToUpdate := []int64{}
	// Find applications that didn't have any inputs in appInputsMap
	for _, app := range apps {
		appAddress := app.application.IApplicationAddress
		// If the app doesn't have any inputs in the map or has an empty slice
		if inputs, exists := appInputsMap[appAddress]; !exists || len(inputs) == 0 {
			appsToUpdate = append(appsToUpdate, app.application.ID)
		}
	}
	// Update LastInputCheckBlock for applications without inputs
	if len(appsToUpdate) > 0 {
		err := r.repository.UpdateEventLastCheckBlock(ctx, appsToUpdate, MonitoredEvent_InputAdded, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Failed to update LastInputCheckBlock for applications without inputs",
				"app_ids", appsToUpdate,
				"block_number", mostRecentBlockNumber,
				"error", err,
			)
			// We don't return an error here as we've already processed the inputs
			// and this is just an update to the last check block
		} else {
			r.Logger.Debug("Updated LastInputCheckBlock for applications without inputs",
				"app_ids", appsToUpdate,
				"block_number", mostRecentBlockNumber,
			)
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

	inputSource := apps[0].inputSource
	opts := bind.FilterOpts{
		Context: ctx,
		Start:   startBlock,
		End:     &endBlock,
	}
	inputsEvents, err := inputSource.RetrieveInputs(&opts, appsAddresses, nil)
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
