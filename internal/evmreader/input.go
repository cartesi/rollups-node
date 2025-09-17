// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// initializeNewApplicationInputSync initializes input synchronization for a new application
// by finding the appropriate starting block and updating the database
func (r *Service) initializeNewApplicationInputSync(
	ctx context.Context,
	app *appContracts,
	mostRecentBlockNumber uint64,
) (uint64, error) {
	r.Logger.Info("Initializing application input sync",
		"application", app.application.Name,
		"inputbox_block", app.application.IInputBoxBlock,
		"current_block", mostRecentBlockNumber,
	)
	if app.application.IInputBoxBlock == 0 {
		r.Logger.Error("Application has no InputBox block number defined",
			"application", app.application.Name,
			"inputbox", app.application.IInputBoxAddress,
			"iinputbox_block", app.application.IInputBoxBlock,
		)
		return 0, errors.New("application has no InputBox block number defined")
	}
	lastInputCheckBlock := app.application.IInputBoxBlock - 1

	err := r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_InputAdded, lastInputCheckBlock)
	if err != nil {
		r.Logger.Error("Failed to update application LastInputCheckBlock",
			"application", app.application.Name,
			"last_input_check_block", lastInputCheckBlock,
			"error", err,
		)
		return 0, err
	}
	r.Logger.Debug("Application input sync initialized",
		"application", app.application.Name,
		"inputbox_block", app.application.IInputBoxBlock,
		"last_input_check_block", lastInputCheckBlock,
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
			"most_recent_block", mostRecentBlockNumber,
		)

		appsByLastInputCheckBlock := make(map[uint64][]appContracts)
		for _, app := range inputBoxApps {
			lastInputCheckBlock := app.application.LastInputCheckBlock
			if lastInputCheckBlock == 0 { // New application. Find a safe start block to scan for inputs
				var err error
				lastInputCheckBlock, err = r.initializeNewApplicationInputSync(ctx, &app, mostRecentBlockNumber)
				if err != nil {
					r.Logger.Error("Failed to initialize application input sync",
						"application", app.application.Name,
						"most_recent_block", mostRecentBlockNumber,
						"error", err,
					)
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
			_ = r.setApplicationInoperable(ctx, app.application, "Application has epoch length of zero")
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
						return r.setApplicationInoperable(ctx, app.application,
							"Received inputs for an epoch that was not open. Should never happen. Epoch %d Status %s, Input %d",
							currentEpoch.Index, currentEpoch.Status, input.Index)
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

		// Store everything
		if len(epochInputMap) > 0 {
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
			r.Logger.Debug("Inputs and epochs stored successfully",
				"application", app.application.Name,
				"address", address,
				"start_block", nextSearchBlock,
				"end_block", mostRecentBlockNumber,
				"epoch_count", len(epochInputMap),
				"input_count", len(inputs),
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

func (r *Service) readInputsFromBlockchain(
	ctx context.Context,
	apps []appContracts,
	startBlock, endBlock uint64,
) (map[common.Address][]*Input, error) {

	// Initialize app input map
	var appInputsMap = make(map[common.Address][]*Input)

	for _, app := range apps {
		inputs, err := r.fetchApplicationInputs(ctx, app, startBlock, endBlock)
		if err != nil {
			r.Logger.Error("Error fetching inputs for application",
				"application", app.application.Name,
				"start_block", startBlock,
				"end_block", endBlock,
				"error", err.Error(),
			)
			continue
		}
		appInputsMap[app.application.IApplicationAddress] = inputs
	}

	return appInputsMap, nil
}

func (r *Service) fetchApplicationInputs(
	ctx context.Context,
	app appContracts,
	startBlock, endBlock uint64,
) ([]*Input, error) {
	r.Logger.Debug("Fetching inputs for application",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", endBlock,
	)

	// Define oracle function that returns the number of inputs at a given block
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		callOpts := &bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}
		numInputs, err := app.inputSource.GetNumberOfInputs(callOpts, app.application.IApplicationAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to get number of inputs at block %d: %w", block, err)
		}
		return numInputs, nil
	}

	var sortedInputs []*Input
	// Define onHit function that accumulates inputs at transition blocks
	onHit := func(block uint64) error {
		filterOpts := &bind.FilterOpts{
			Context: ctx,
			Start:   block,
			End:     &block,
		}
		inputEvents, err := app.inputSource.RetrieveInputs(
			filterOpts,
			[]common.Address{app.application.IApplicationAddress},
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve inputs at block %d: %w", block, err)
		}
		for _, event := range inputEvents {
			input := &Input{
				Index:                event.Index.Uint64(),
				Status:               InputCompletionStatus_None,
				RawData:              event.Input,
				BlockNumber:          event.Raw.BlockNumber,
				TransactionReference: event.Raw.TxHash,
			}
			sortedInputs = insertSorted(sortByInputIndex, sortedInputs, input)
		}
		return nil
	}

	inputCount, err := r.repository.GetNumberOfInputs(ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get number of inputs from repository: %w", err)
	}
	prevValue := new(big.Int).SetUint64(inputCount)

	// Use FindTransitions to find blocks where inputs were added
	_, err = ethutil.FindTransitions(ctx, startBlock, endBlock, prevValue, oracle, onHit)
	if err != nil {
		return nil, fmt.Errorf("failed to walk input transitions: %w", err)
	}

	r.Logger.Debug("Fetched inputs for application",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", endBlock,
		"prev_input_count", prevValue.Uint64(),
		"new_inputs", len(sortedInputs),
	)
	return sortedInputs, nil
}
