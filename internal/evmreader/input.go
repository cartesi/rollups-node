// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"

	"github.com/cartesi/rollups-node/internal/errutil"
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
		return 0, errors.New("application has no InputBox block number defined")
	}
	lastInputCheckBlock := app.application.IInputBoxBlock - 1

	err := r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_InputAdded, lastInputCheckBlock)
	if err != nil {
		return 0, fmt.Errorf("initialize input cursor at block %d: %w", lastInputCheckBlock, err)
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
) bool {
	r.Logger.Debug("Checking for new inputs")

	units, success := r.buildIConsensusInputScanUnits(ctx, applications, mostRecentBlockNumber)
	for _, unit := range units {
		if ctx.Err() != nil {
			return false
		}
		success = r.scanIConsensusInputUnit(ctx, unit) && success
	}
	return success
}

func (r *Service) buildIConsensusInputScanUnits(
	ctx context.Context,
	applications []appContracts,
	endBlock uint64,
) ([]iConsensusInputScanUnit, bool) {
	success := true
	appsByInputBox := map[common.Address][]appContracts{}
	for _, app := range applications {
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
			if ctx.Err() != nil {
				return nil, false
			}
			lastInputCheckBlock := app.application.LastInputCheckBlock
			if lastInputCheckBlock == 0 { // New application. Find a safe start block to scan for inputs
				var err error
				lastInputCheckBlock, err = r.initializeNewApplicationInputSync(
					ctx,
					&app,
					foreclosureBoundedEndBlock(app.application, endBlock),
				)
				if err != nil {
					success = false
					r.reportInputScanError(ctx, app, 0, endBlock, err)
					continue
				}
			}
			scanEndBlock := foreclosureBoundedEndBlock(app.application, endBlock)
			if lastInputCheckBlock > scanEndBlock {
				level := slog.LevelWarn
				if app.application.IInputBoxBlock != 0 && lastInputCheckBlock == app.application.IInputBoxBlock-1 {
					// Registration can initialize the cursor before deployment is
					// visible at the configured observation block.
					level = slog.LevelDebug
				}
				r.Logger.Log(ctx, level,
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
	return units, success
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
) bool {
	success := true
	appAddresses := appsToAddresses(unit.apps)

	if unit.endBlock > unit.lastInputCheckBlock {
		r.Logger.Debug("Checking inputs for applications",
			"apps", appAddresses,
			"last_processed_block", unit.lastInputCheckBlock,
			"most_recent_block", unit.endBlock,
		)

		for _, app := range unit.apps {
			if ctx.Err() != nil {
				return false
			}
			err := r.readAndStoreApplicationInputs(ctx, unit.lastInputCheckBlock, unit.endBlock, app)
			if err != nil {
				success = false
				r.reportInputScanError(ctx, app, unit.lastInputCheckBlock, unit.endBlock, err)
			}
		}
		return success
	}

	if unit.endBlock < unit.lastInputCheckBlock {
		r.Logger.Warn(
			"Input search skipped: most recent block is lower than the last processed one",
			"apps", appAddresses,
			"last_processed_block", unit.lastInputCheckBlock,
			"most_recent_block", unit.endBlock,
		)
		return success
	}

	r.Logger.Debug("Input search skipped: already checked the most recent block",
		"apps", appAddresses,
		"last_processed_block", unit.lastInputCheckBlock,
		"most_recent_block", unit.endBlock,
	)
	return success
}

// The application scan owns operational error logging. Helpers return the cause;
// status transitions retain their existing, separately reported diagnostics.
func (r *Service) reportInputScanError(ctx context.Context, app appContracts, from, to uint64, err error) {
	if errors.Is(err, errApplicationStatusReported) {
		return
	}
	level := slog.LevelError
	if ctx.Err() == context.Canceled && errutil.IsOnlyCancellation(err) {
		level = slog.LevelDebug
	}
	r.Logger.Log(ctx, level, "Application input scan interrupted",
		"application", app.application.Name,
		"address", app.application.IApplicationAddress,
		"last_processed_block", from,
		"most_recent_block", to,
		"error", err,
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

// recordInputCorruption keeps a recorded integrity fault local to the application.
// A failed status write still fails the scan. Status helpers update app.Status
// only after a successful write and preserve existing integrity terminals.
func (r *Service) recordInputCorruption(
	ctx context.Context,
	app *Application,
	reasonFmt string,
	args ...any,
) error {
	reason := fmt.Sprintf(reasonFmt, args...)
	err := r.setApplicationCorrupted(ctx, app, "%s", reason)
	if app.Status == ApplicationStatus_Corrupted || app.Status == ApplicationStatus_Diverged {
		r.Logger.Warn("Input observation degraded for application", "application", app.Name, "reason", reason)
		return nil
	}
	return err
}

// readAndStoreApplicationInputs completes one application's scan before its
// cursor can advance. A failed fetch, epoch lookup, or write leaves it retryable.
func (r *Service) readAndStoreApplicationInputs(
	ctx context.Context,
	lastProcessedBlock uint64,
	mostRecentBlockNumber uint64,
	app appContracts,
) error {
	address := app.application.IApplicationAddress
	epochLength := app.application.EpochLength
	if epochLength == 0 {
		return r.recordInputCorruption(ctx, app.application, "Application has epoch length of zero")
	}

	inputs, err := r.readApplicationInputs(ctx, app, lastProcessedBlock+1, mostRecentBlockNumber)
	if err != nil {
		return err
	}
	currentEpoch, err := r.repository.GetEpoch(ctx, address.String(), calculateEpochIndex(epochLength, lastProcessedBlock))
	if err != nil {
		return fmt.Errorf("retrieve current epoch: %w", err)
	}
	epochInputMap, err := indexInputsIntoEpochs(epochLength, currentEpoch, inputs, mostRecentBlockNumber)
	if err != nil {
		if errors.Is(err, ErrInputForNonOpenEpoch) {
			return r.recordInputCorruption(ctx, app.application, "Should never happen. %v", err)
		}
		return fmt.Errorf("index inputs: %w", err)
	}

	if len(epochInputMap) == 0 {
		// No epoch needs to be stored or closed. Only this application's
		// successfully processed range is eligible for the cursor-only update.
		err = r.repository.UpdateEventLastCheckBlock(
			ctx, []int64{app.application.ID}, MonitoredEvent_InputAdded, mostRecentBlockNumber)
		if err != nil {
			return fmt.Errorf("update input cursor: %w", err)
		}
		return nil
	}

	// This transaction stores inputs, closes epochs, and advances the cursor.
	// An epoch closure with no new inputs must use the same atomic write.
	err = r.repository.CreateEpochsAndInputs(ctx, address.String(), epochInputMap, mostRecentBlockNumber)
	if err != nil {
		if errors.Is(err, repository.ErrInputLogIdentityConflict) {
			return r.recordInputCorruption(ctx, app.application,
				"stored input L1 log identity conflicts with rescanned chain data"+
					" (possible reorg past the input cursor); operator reset required. %v", err)
		}
		return fmt.Errorf("store inputs and epochs: %w", err)
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
	r.Logger.Debug("Inputs and epochs stored successfully",
		"application", app.application.Name,
		"address", address,
		"start_block", lastProcessedBlock+1,
		"end_block", mostRecentBlockNumber,
		"epoch_count", len(epochInputMap),
		"input_count", len(inputs),
	)
	return nil
}

// readApplicationInputs preserves the failure cause for the scan caller. It
// validates the fetched logs against the counter observed in the same walk.
func (r *Service) readApplicationInputs(
	ctx context.Context,
	app appContracts,
	startBlock, endBlock uint64,
) ([]*Input, error) {
	inputCount, err := r.repository.GetNumberOfInputs(ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return nil, fmt.Errorf("get stored input count: %w", err)
	}
	inputs, endCount, err := r.fetchInputs(
		ctx, app, startBlock, endBlock, new(big.Int).SetUint64(inputCount), 0, math.MaxUint64)
	if err != nil {
		return nil, fmt.Errorf("fetch inputs: %w", err)
	}
	expectedNew := endCount - inputCount
	if uint64(len(inputs)) != expectedNew {
		return nil, fmt.Errorf("input count mismatch: stored %d, on-chain %d, expected %d new inputs, got %d",
			inputCount, endCount, expectedNew, len(inputs))
	}
	return inputs, nil
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
