// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

type iConsensusInputScanUnit struct {
	inputBoxAddress     common.Address
	lastInputCheckBlock uint64
	endBlock            uint64
	apps                []appContracts
}

type iConsensusInputScanRange struct {
	lastInputCheckBlock uint64
	endBlock            uint64
}

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

func (r *Service) scanIConsensusInputs(
	ctx context.Context,
	applications []appContracts,
	mostRecentBlockNumber uint64,
) {
	if !r.inputReaderEnabled {
		return
	}

	r.Logger.Debug("Checking for new inputs")

	for _, unit := range r.buildIConsensusInputScanUnits(ctx, applications, mostRecentBlockNumber) {
		r.scanIConsensusInputUnit(ctx, unit)
	}
}

func (r *Service) buildIConsensusInputScanUnits(
	ctx context.Context,
	applications []appContracts,
	endBlock uint64,
) []iConsensusInputScanUnit {
	appsByInputBox := map[common.Address][]appContracts{}
	for _, app := range applications {
		if !app.application.HasDataAvailabilitySelector(DataAvailability_InputBox) {
			continue
		}
		key := app.application.IInputBoxAddress
		appsByInputBox[key] = append(appsByInputBox[key], app)
	}

	var units []iConsensusInputScanUnit
	for inputBoxAddress, inputBoxApps := range appsByInputBox {
		r.Logger.Debug("Checking inputs for applications with the same InputBox",
			"inputbox_address", inputBoxAddress,
			"most_recent_block", endBlock,
		)

		appsByLastInputCheckBlock := make(map[iConsensusInputScanRange][]appContracts)
		for _, app := range inputBoxApps {
			lastInputCheckBlock := app.application.LastInputCheckBlock
			if lastInputCheckBlock == 0 { // New application. Find a safe start block to scan for inputs
				var err error
				lastInputCheckBlock, err = r.initializeNewApplicationInputSync(
					ctx,
					&app,
					foreclosureBoundedEndBlock(app.application, endBlock),
				)
				if err != nil {
					r.Logger.Error("Failed to initialize application input sync",
						"application", app.application.Name,
						"most_recent_block", endBlock,
						"error", err,
					)
					continue
				}
			}
			scanEndBlock := foreclosureBoundedEndBlock(app.application, endBlock)
			if lastInputCheckBlock > scanEndBlock {
				r.Logger.Warn(
					"Input search skipped: most recent block is lower than the last processed one",
					"application", app.application.Name,
					"last_processed_block", lastInputCheckBlock,
					"most_recent_block", scanEndBlock,
				)
				continue
			}
			if lastInputCheckBlock == scanEndBlock {
				r.Logger.Debug("Input search skipped: already checked the most recent block",
					"application", app.application.Name,
					"last_processed_block", lastInputCheckBlock,
					"most_recent_block", scanEndBlock,
				)
				continue
			}

			scanRange := iConsensusInputScanRange{
				lastInputCheckBlock: lastInputCheckBlock,
				endBlock:            scanEndBlock,
			}
			appsByLastInputCheckBlock[scanRange] = append(appsByLastInputCheckBlock[scanRange], app)
		}

		for scanRange, apps := range appsByLastInputCheckBlock {
			units = append(units, iConsensusInputScanUnit{
				inputBoxAddress:     inputBoxAddress,
				lastInputCheckBlock: scanRange.lastInputCheckBlock,
				endBlock:            scanRange.endBlock,
				apps:                apps,
			})
		}
	}
	return units
}

func foreclosureBoundedEndBlock(app *Application, endBlock uint64) uint64 {
	if app != nil && app.ForecloseBlock != 0 && app.ForecloseBlock < endBlock {
		return app.ForecloseBlock
	}
	return endBlock
}

func (r *Service) scanIConsensusInputUnit(
	ctx context.Context,
	unit iConsensusInputScanUnit,
) {
	appAddresses := appsToAddresses(unit.apps)

	if unit.endBlock > unit.lastInputCheckBlock {
		r.Logger.Debug("Checking inputs for applications",
			"apps", appAddresses,
			"last_processed_block", unit.lastInputCheckBlock,
			"most_recent_block", unit.endBlock,
		)

		err := r.readAndStoreInputs(ctx,
			unit.lastInputCheckBlock,
			unit.endBlock,
			unit.apps,
		)
		if err != nil {
			r.Logger.Error("Error reading inputs",
				"apps", appAddresses,
				"last_processed_block", unit.lastInputCheckBlock,
				"most_recent_block", unit.endBlock,
				"error", err,
			)
		}
		return
	}

	if unit.endBlock < unit.lastInputCheckBlock {
		r.Logger.Warn(
			"Input search skipped: most recent block is lower than the last processed one",
			"apps", appAddresses,
			"last_processed_block", unit.lastInputCheckBlock,
			"most_recent_block", unit.endBlock,
		)
		return
	}

	r.Logger.Debug("Input search skipped: already checked the most recent block",
		"apps", appAddresses,
		"last_processed_block", unit.lastInputCheckBlock,
		"most_recent_block", unit.endBlock,
	)
}

// ErrInputForNonOpenEpoch indicates that an input was received for an epoch
// that is not open — a state that should never occur during normal operation.
var ErrInputForNonOpenEpoch = errors.New("received input for non-open epoch")

// indexInputsIntoEpochs is a pure function that indexes a sorted list of inputs
// into their respective epochs based on block number and epoch length.
// It creates new epochs as needed and closes epochs when subsequent inputs fall
// into a later epoch. It also closes the final epoch if mostRecentBlockNumber
// has advanced past its last block.
// The returned map contains epoch pointers as keys and their inputs as values.
// An epoch may appear with an empty input slice if it was closed without receiving
// new inputs in this batch.
// Returns ErrInputForNonOpenEpoch if an input targets an already-closed epoch.
func indexInputsIntoEpochs(
	epochLength uint64,
	currentEpoch *Epoch,
	inputs []*Input,
	mostRecentBlockNumber uint64,
) (map[*Epoch][]*Input, error) {
	epochInputMap := make(map[*Epoch][]*Input)

	for _, input := range inputs {
		inputEpochIndex := calculateEpochIndex(epochLength, input.BlockNumber)

		// If input belongs into a new epoch, close the previous known one
		if currentEpoch != nil {
			if currentEpoch.Index == inputEpochIndex {
				// Input can only be added to open epochs
				if currentEpoch.Status != EpochStatus_Open {
					return nil, fmt.Errorf(
						"%w: epoch %d status %s, input %d",
						ErrInputForNonOpenEpoch,
						currentEpoch.Index, currentEpoch.Status, input.Index)
				}
				currentEpoch.InputIndexUpperBound = input.Index + 1
			} else {
				if currentEpoch.Status == EpochStatus_Open {
					currentEpoch.Status = EpochStatus_Closed
					currentEpoch.InputIndexUpperBound = input.Index
					if _, ok := epochInputMap[currentEpoch]; !ok {
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

		epochInputMap[currentEpoch] = append(epochInputMap[currentEpoch], input)
	}

	// Indexed all inputs. Check if it is time to close the last epoch
	if currentEpoch != nil && currentEpoch.Status == EpochStatus_Open &&
		mostRecentBlockNumber >= currentEpoch.LastBlock {
		currentEpoch.Status = EpochStatus_Closed
		if _, ok := epochInputMap[currentEpoch]; !ok {
			epochInputMap[currentEpoch] = []*Input{}
		}
	}

	return epochInputMap, nil
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
			// setApplicationCorrupted always returns non-nil (the reason text itself).
			// The DB error case is already logged inside setApplicationStatus.
			// On DB success the app is marked inoperable and won't reappear next tick.
			// On DB failure the app reappears as Enabled next tick, retrying this path.
			_ = r.setApplicationCorrupted(ctx, app.application,
				"Application has epoch length of zero")
			continue
		}

		// Retrieves last open epoch from DB
		currentEpoch, err := r.repository.GetEpoch(ctx, address.String(), calculateEpochIndex(epochLength, lastProcessedBlock))
		if err != nil {
			// Shutdown cancels the ctx mid-query; downgrade to Debug
			// for the graceful-stop case. DeadlineExceeded would still
			// flow through the Error branch.
			if errors.Is(err, context.Canceled) {
				r.Logger.Debug("GetEpoch canceled during shutdown",
					"application", app.application.Name,
					"address", address,
					"error", err,
				)
			} else {
				r.Logger.Error("Error retrieving existing current epoch",
					"application", app.application.Name,
					"address", address,
					"error", err,
				)
			}
			continue
		}

		// Index inputs into epochs using pure function
		epochInputMap, err := indexInputsIntoEpochs(
			epochLength, currentEpoch, inputs, mostRecentBlockNumber)
		if err != nil {
			if errors.Is(err, ErrInputForNonOpenEpoch) {
				return r.setApplicationCorrupted(ctx, app.application,
					"Should never happen. %v", err)
			}
			return fmt.Errorf("error indexing inputs: %w", err)
		}

		for epoch, epochInputs := range epochInputMap {
			if epoch.Status == EpochStatus_Closed {
				r.Logger.Info("Closing epoch",
					"application", app.application.Name,
					"address", address,
					"epoch_index", epoch.Index,
					"start", epoch.FirstBlock,
					"end", epoch.LastBlock)
			}
			for _, input := range epochInputs {
				r.Logger.Info("Found new Input",
					"application", app.application.Name,
					"address", address,
					"index", input.Index,
					"block", input.BlockNumber,
					"epoch_index", epoch.Index)
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
				if errors.Is(err, repository.ErrInputLogIdentityConflict) {
					// A stored input's L1 log identity disagrees with rescanned
					// chain data. Retrying the same insert every tick cannot
					// succeed; without escalation the app would stall silently
					// with Status OK. See setApplicationCorrupted contract in
					// the epochLength == 0 branch above.
					_ = r.setApplicationCorrupted(ctx, app.application,
						"stored input L1 log identity conflicts with rescanned chain data"+
							" (possible reorg past the input cursor); operator reset required. %v", err)
					continue
				}
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

	// Update LastInputCheckBlock for applications that were successfully scanned
	// but didn't have any inputs.
	// For apps WITH inputs, LastInputCheckBlock is already updated atomically inside
	// CreateEpochsAndInputs (same DB transaction as the epoch/input inserts).
	// This separate call for no-input apps is NOT in the same transaction. If the process
	// crashes between the two, no-input apps will re-scan the block range on restart.
	// This is benign: the re-scan finds no inputs and updates the checkpoint idempotently.
	// Only apps present in appInputsMap were successfully scanned. Apps that failed
	// to fetch are absent from the map and their checkpoint must NOT advance,
	// otherwise inputs in the failed block range would be permanently skipped.
	appsToUpdate := []int64{}
	for _, app := range apps {
		appAddress := app.application.IApplicationAddress
		if inputs, exists := appInputsMap[appAddress]; exists && len(inputs) == 0 {
			appsToUpdate = append(appsToUpdate, app.application.ID)
		}
	}
	// Update LastInputCheckBlock for applications without inputs
	if len(appsToUpdate) > 0 {
		err := r.repository.UpdateEventLastCheckBlock(ctx, appsToUpdate, MonitoredEvent_InputAdded, mostRecentBlockNumber)
		if err != nil {
			// Shutdown cancels the ctx mid-update; downgrade to Debug
			// for the graceful-stop case. DeadlineExceeded would still
			// flow through the Error branch.
			if errors.Is(err, context.Canceled) {
				r.Logger.Debug("UpdateEventLastCheckBlock canceled during shutdown",
					"app_ids", appsToUpdate,
					"block_number", mostRecentBlockNumber,
					"error", err,
				)
			} else {
				r.Logger.Error("Failed to update LastInputCheckBlock for applications without inputs",
					"app_ids", appsToUpdate,
					"block_number", mostRecentBlockNumber,
					"error", err,
				)
			}
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

// readInputsFromBlockchain fetches inputs for each application independently.
// On per-app failure, the failing app is omitted from the returned map (not
// present as a key) and processing continues for the remaining apps. The error
// return is reserved for fatal failures that prevent any work.
// Callers must use map-key presence to distinguish success (key exists, possibly
// with an empty slice) from failure (key absent) — apps absent from the map must
// NOT have their checkpoint advanced.
func (r *Service) readInputsFromBlockchain(
	ctx context.Context,
	apps []appContracts,
	startBlock, endBlock uint64,
) (map[common.Address][]*Input, error) {

	// Initialize app input map
	var appInputsMap = make(map[common.Address][]*Input)

	for _, app := range apps {
		inputCount, err := r.repository.GetNumberOfInputs(
			ctx, app.application.IApplicationAddress.String())
		if err != nil {
			r.Logger.Error("Error getting input count for application",
				"application", app.application.Name,
				"error", err.Error(),
			)
			continue
		}
		prevValue := new(big.Int).SetUint64(inputCount)
		inputs, endCount, err := r.fetchInputs(
			ctx, app, startBlock, endBlock,
			prevValue, 0, math.MaxUint64)
		if err != nil {
			r.Logger.Error("Error fetching inputs for application",
				"application", app.application.Name,
				"start_block", startBlock,
				"end_block", endBlock,
				"error", err.Error(),
			)
			continue
		}

		// Validate input count: the on-chain counter delta observed during the
		// transition walk should match the number of inputs fetched. This mirrors
		// the DaveConsensus sealed epoch validation at sealedepochs.go and guards
		// against silent input loss from FindTransitions missing a transition.
		expectedNew := endCount - inputCount
		if uint64(len(inputs)) != expectedNew {
			r.Logger.Error(
				"Input count mismatch: on-chain delta does not match fetched inputs",
				"application", app.application.Name,
				"db_count", inputCount,
				"on_chain_end_count", endCount,
				"expected_new", expectedNew,
				"got", len(inputs),
				"start_block", startBlock,
				"end_block", endBlock,
			)
			continue
		}

		appInputsMap[app.application.IApplicationAddress] = inputs
	}

	return appInputsMap, nil
}

// fetchInputs locates blocks where new inputs were added via FindTransitions
// and accumulates them. The prevValue, lowerBound, and upperBound are caller-
// determined: IConsensus passes prevValue from the DB input count with full
// bounds [0, MaxUint64), while DaveConsensus sealed epochs use the epoch's
// InputIndexLowerBound as prevValue and the epoch's own index range as bounds.
//
// The second return value is the input counter observed at endBlock during the
// same transition walk, letting callers validate the on-chain count delta
// without a second eth_call pinned to endBlock.
func (r *Service) fetchInputs(
	ctx context.Context,
	app appContracts,
	startBlock, endBlock uint64,
	prevValue *big.Int,
	lowerBound, upperBound uint64,
) ([]*Input, uint64, error) {
	prevCount := uint64(0)
	if prevValue != nil {
		prevCount = prevValue.Uint64()
	}
	if startBlock > endBlock {
		// Empty range: no new inputs, and the counter is unchanged from prevValue.
		return nil, prevCount, nil
	}

	r.Logger.Debug("Fetching inputs",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", endBlock,
		"lower_bound", lowerBound,
		"upper_bound", upperBound,
	)

	var observedEndValue *big.Int
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		callOpts := &bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}
		numInputs, err := app.inputSource.GetNumberOfInputs(
			callOpts, app.application.IApplicationAddress)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get number of inputs at block %d: %w",
				block, err)
		}
		if block == endBlock {
			observedEndValue = new(big.Int).Set(numInputs)
		}
		return numInputs, nil
	}

	var sortedInputs []*Input
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
			return fmt.Errorf(
				"failed to retrieve inputs at block %d: %w",
				block, err)
		}
		for _, event := range inputEvents {
			idx := event.Index.Uint64()
			if idx < lowerBound || idx >= upperBound {
				continue
			}
			input := &Input{
				Index:           idx,
				Status:          InputCompletionStatus_None,
				RawData:         event.Input,
				BlockNumber:     event.Raw.BlockNumber,
				TransactionHash: event.Raw.TxHash,
				LogIndex:        uint64(event.Raw.Index),
			}
			var duplicate bool
			sortedInputs, duplicate = insertSorted(
				sortByInputIndex, sortedInputs, input)
			if duplicate {
				r.Logger.Warn("Duplicate input event detected, skipping",
					"application", app.application.Name,
					"index", input.Index,
					"block", input.BlockNumber,
				)
			}
		}
		return nil
	}

	_, err := ethutil.FindTransitions(
		ctx, startBlock, endBlock, prevValue, oracle, onHit)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"failed to walk input transitions: %w", err)
	}
	if observedEndValue == nil {
		return nil, 0, fmt.Errorf("failed to observe input count at end block %d", endBlock)
	}

	r.Logger.Debug("Fetched inputs",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", endBlock,
		"prev_input_count", prevCount,
		"end_input_count", observedEndValue.Uint64(),
		"new_inputs", len(sortedInputs),
	)
	return sortedInputs, observedEndValue.Uint64(), nil
}
