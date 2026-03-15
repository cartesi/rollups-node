// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/cartesi/rollups-node/internal/events"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func (r *Service) initializeNewApplicationSealedEpochSync(
	ctx context.Context,
	app *appContracts,
	mostRecentBlockNumber uint64,
) error {
	r.Logger.Info("Initializing application sealed epoch sync",
		"application", app.application.Name,
		"current_block", mostRecentBlockNumber,
	)
	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	}
	deploymentBlock, err := app.daveConsensus.GetDeploymentBlockNumber(callOpts)
	if err != nil {
		r.Logger.Error("Error retrieving dave consensus deployment block number",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"consensus_address", app.application.IConsensusAddress,
			"error", err,
		)
		return fmt.Errorf("failed to retrieve DaveConsensus deployment block: %w", err)
	}
	if deploymentBlock.Sign() <= 0 {
		r.Logger.Error("Invalid dave consensus deployment block number retrieved",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"consensus_address", app.application.IConsensusAddress,
			"block_number", deploymentBlock.Uint64(),
		)
		return errors.New("invalid dave consensus deployment block number retrieved")
	}

	lastEpochCheckBlock := deploymentBlock.Uint64() - 1
	err = r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_EpochSealed, lastEpochCheckBlock)
	if err != nil {
		r.Logger.Error("Failed to update application LastEpochCheckBlock",
			"application", app.application.Name,
			"last_epoch_check_block", lastEpochCheckBlock,
			"error", err,
		)
		return err
	}
	r.Logger.Debug("Application sealed epoch sync initialized",
		"application", app.application.Name,
		"deployment_block", deploymentBlock.Uint64(),
		"next_search_block", lastEpochCheckBlock+1,
		"current_block", mostRecentBlockNumber,
	)
	app.application.LastEpochCheckBlock = lastEpochCheckBlock
	return nil
}

func (r *Service) checkForEpochsAndInputs(
	ctx context.Context,
	applications []appContracts,
	mostRecentBlockNumber uint64,
) {
	if !r.inputReaderEnabled {
		return
	}

	r.Logger.Debug("Checking for new epochs and inputs", "apps", applications)

	// Process each application individually since each has its own DaveConsensus contract
	for _, app := range applications {
		r.Logger.Debug("Processing DaveConsensus application",
			"application", app.application.Name,
			"consensus_address", app.application.IConsensusAddress)

		err := r.processApplicationSealedEpochs(ctx, app, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Error processing application sealed epochs",
				"application", app.application.Name,
				"consensus_address", app.application.IConsensusAddress,
				"error", err)
			continue
		}

		err = r.processApplicationOpenEpoch(ctx, app, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Error processing application open epoch",
				"application", app.application.Name,
				"consensus_address", app.application.IConsensusAddress,
				"error", err)
			continue
		}
	}
}

func (r *Service) processApplicationSealedEpochs(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) error {
	// Find the starting block for epoch search
	if app.application.LastEpochCheckBlock == 0 {
		err := r.initializeNewApplicationSealedEpochSync(ctx, &app, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Failed to initialize application sealed epoch sync",
				"application", app.application.Name,
				"most_recent_block", mostRecentBlockNumber,
				"error", err,
			)
			return fmt.Errorf("failed to determine start block for epoch search: %w", err)
		}
	}

	if mostRecentBlockNumber < app.application.LastEpochCheckBlock {
		r.Logger.Warn(
			"Not reading sealed epochs: most recent block is lower than the last processed one",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"last_epoch_check_block", app.application.LastEpochCheckBlock,
			"most_recent_block", mostRecentBlockNumber,
		)
		return nil
	} else if mostRecentBlockNumber == app.application.LastEpochCheckBlock {
		r.Logger.Debug("Not reading sealed epochs: already checked the most recent blocks",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"last_epoch_check_block", app.application.LastEpochCheckBlock,
			"most_recent_block", mostRecentBlockNumber,
		)
		return nil
	}

	nextSearchBlock := app.application.LastEpochCheckBlock + 1
	r.Logger.Debug("Checking sealed epochs for application",
		"application", app.application.Name,
		"last_epoch_check_block", app.application.LastEpochCheckBlock,
		"next_search_block", nextSearchBlock,
		"most_recent_block", mostRecentBlockNumber,
	)

	// Create oracle function that returns the current sealed epoch number for a given block
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		r.Logger.Debug("Retrieving current sealed epoch", "application", app.application.Name, "block", block)
		callOpts := &bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}

		sealedEpoch, err := app.daveConsensus.GetCurrentSealedEpoch(callOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to get current sealed epoch at block %d: %w", block, err)
		}

		return sealedEpoch.EpochNumber, nil
	}

	// Create onHit function that processes epoch transitions
	onHit := func(block uint64) error {
		r.Logger.Debug("Epoch transition found", "application", app.application.Name, "block", block)
		return r.processEpochTransition(ctx, app, block)
	}

	prevValue := big.NewInt(-1)
	lastEpoch, err := r.repository.GetLastNonOpenEpoch(ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return fmt.Errorf("failed to get last non open epoch: %w", err)
	}
	if lastEpoch != nil {
		prevValue = new(big.Int).SetUint64(lastEpoch.Index)
		// assert that the last epoch's last block is less than the next search block
		if lastEpoch.LastBlock > nextSearchBlock {
			return r.setApplicationInoperable(ctx, app.application,
				"application last non open epoch last block %d is greater than next search block %d",
				lastEpoch.LastBlock,
				nextSearchBlock,
			)
		}
	}

	// Use FindTransitions to find epoch transitions
	_, err = ethutil.FindTransitions(ctx, nextSearchBlock, mostRecentBlockNumber, prevValue, oracle, onHit)
	if err != nil {
		return fmt.Errorf("failed to walk epoch transitions: %w", err)
	}

	// Update the last check block for this application
	err = r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_EpochSealed, mostRecentBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to update last epoch check block: %w", err)
	}

	r.Logger.Debug("Sealed epoch search completed", "application", app.application.Name, "most_recent_block", mostRecentBlockNumber)

	return nil
}

func (r *Service) processEpochTransition(
	ctx context.Context,
	app appContracts,
	transitionBlock uint64,
) error {
	r.Logger.Debug("Processing epoch transition", "application", app.application.Name, "block", transitionBlock)

	// Get the sealed epoch information at this block
	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(transitionBlock),
	}

	sealedEpoch, err := app.daveConsensus.GetCurrentSealedEpoch(callOpts)
	if err != nil {
		return fmt.Errorf("failed to get sealed epoch at transition block %d: %w", transitionBlock, err)
	}

	r.Logger.Info("Found sealed epoch event",
		"application", app.application.Name,
		"block", transitionBlock,
		"epoch_number", sealedEpoch.EpochNumber,
		"input_lower_bound", sealedEpoch.InputIndexLowerBound,
		"input_upper_bound", sealedEpoch.InputIndexUpperBound,
		"tournament", sealedEpoch.Tournament)

	// Retrieve the actual EpochSealed events for this transition
	filterOpts := &bind.FilterOpts{
		Context: ctx,
		Start:   transitionBlock,
		End:     &transitionBlock,
	}

	sealedEvents, err := app.daveConsensus.RetrieveSealedEpochs(filterOpts)
	if err != nil {
		return fmt.Errorf("failed to retrieve sealed epoch events at block %d: %w", transitionBlock, err)
	}

	// Process each sealed epoch event
	for _, event := range sealedEvents {
		err := r.processSealedEpochEvent(ctx, app, event)
		if err != nil {
			r.Logger.Error("Error processing sealed epoch event",
				"epoch_number", event.EpochNumber,
				"block", transitionBlock,
				"error", err)
			return fmt.Errorf("failed to process sealed epoch event at block %d: %w", transitionBlock, err)
		}
	}

	return nil
}

func (r *Service) processSealedEpochEvent(
	ctx context.Context,
	app appContracts,
	event *idaveconsensus.IDaveConsensusEpochSealed,
) error {
	r.Logger.Debug("Processing sealed epoch event",
		"epoch_number", event.EpochNumber,
		"input_lower_bound", event.InputIndexLowerBound,
		"input_upper_bound", event.InputIndexUpperBound,
		"tournament", event.Tournament)

	firstBlock := uint64(0)
	epochNumber := event.EpochNumber.Uint64()
	if epochNumber == 0 {
		if app.application.IInputBoxBlock == 0 {
			r.Logger.Error("Application has no InputBox block number defined",
				"application", app.application.Name,
				"inputbox", app.application.IInputBoxAddress,
				"iinputbox_block", app.application.IInputBoxBlock,
			)
			return errors.New("application has no InputBox block number defined")
		}
		firstBlock = app.application.IInputBoxBlock
	} else {
		prevEpochNumber := epochNumber - 1
		prevEpoch, err := r.repository.GetEpoch(ctx, app.application.IApplicationAddress.Hex(), prevEpochNumber)
		if err != nil {
			return fmt.Errorf("failed to fetch epoch %d: %w", prevEpochNumber, err)
		}
		if prevEpoch == nil {
			return fmt.Errorf("failed to fetch previous epoch %d: should not be nil", prevEpochNumber)
		}

		prevEpoch.ClaimTransactionHash = &event.Raw.TxHash
		err = r.repository.UpdateEpochClaimTransactionHash(ctx, app.application.IApplicationAddress.Hex(), prevEpoch)
		if err != nil {
			return fmt.Errorf("failed to update previous epoch %d: %w", prevEpochNumber, err)
		}
		firstBlock = prevEpoch.LastBlock
	}

	epoch, err := r.repository.GetEpoch(ctx, app.application.IApplicationAddress.Hex(), epochNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch epoch %d: %w", epochNumber, err)
	}

	if epoch == nil {
		// Create new epoch from sealed event
		epoch = &Epoch{
			Index:                event.EpochNumber.Uint64(),
			FirstBlock:           firstBlock, // Will be calculated based on epoch length
			LastBlock:            event.Raw.BlockNumber,
			InputIndexLowerBound: event.InputIndexLowerBound.Uint64(),
			InputIndexUpperBound: event.InputIndexUpperBound.Uint64(),
			TournamentAddress:    &event.Tournament,
			Status:               EpochStatus_Closed, // Sealed epochs are closed
		}
	} else {
		if epoch.FirstBlock != firstBlock || epoch.InputIndexLowerBound != event.InputIndexLowerBound.Uint64() {
			return fmt.Errorf("epoch %d data mismatch with sealed event", epoch.Index)
		}
		epoch.LastBlock = event.Raw.BlockNumber
		epoch.InputIndexUpperBound = event.InputIndexUpperBound.Uint64()
		epoch.TournamentAddress = &event.Tournament
		epoch.Status = EpochStatus_Closed // Sealed epochs are closed
	}

	// Fetch inputs for this epoch from the InputBox.
	// Always search from epoch.FirstBlock (not lastInputCheckBlock+1) because with
	// PRT's overlapping block boundaries (sealed epoch's LastBlock = next epoch's
	// FirstBlock), inputs for this epoch may exist at the overlap block — added in
	// a later transaction than the previous epoch's seal within the same block.
	// Use InputIndexLowerBound as prevValue rather than the DB input count, since
	// the open epoch may have already stored some of these inputs ahead of us.
	var inputs []*Input
	if epoch.InputIndexUpperBound > epoch.InputIndexLowerBound {
		var err error
		prevValue := new(big.Int).SetUint64(epoch.InputIndexLowerBound)
		inputs, err = r.fetchInputs(ctx, app,
			epoch.FirstBlock, epoch.LastBlock,
			prevValue,
			epoch.InputIndexLowerBound, epoch.InputIndexUpperBound)
		if err != nil {
			return fmt.Errorf("failed to fetch inputs for epoch %d: %w", epoch.Index, err)
		}

		expectedCount := epoch.InputIndexUpperBound - epoch.InputIndexLowerBound
		if uint64(len(inputs)) != expectedCount {
			return fmt.Errorf(
				"epoch %d input count mismatch: expected %d (indices %d to %d), got %d",
				epoch.Index, expectedCount,
				epoch.InputIndexLowerBound, epoch.InputIndexUpperBound,
				len(inputs))
		}
	}
	// Store epoch and inputs
	epochInputMap := map[*Epoch][]*Input{epoch: inputs}

	r.Logger.Debug("Storing sealed epoch", "application", app.application.Name, "epoch_number", epoch.Index)

	err = r.repository.CreateEpochsAndInputs(
		ctx,
		app.application.IApplicationAddress.String(),
		epochInputMap,
		event.Raw.BlockNumber,
	)
	if err != nil {
		return fmt.Errorf("failed to store epoch and inputs: %w", err)
	}

	r.Logger.Debug("Stored sealed epoch and inputs",
		"application", app.application.Name,
		"epoch_number", epoch.Index,
		"num_inputs", len(inputs),
		"block", event.Raw.BlockNumber)

	// Publish advisory events after successful DB write.
	if len(inputs) > 0 {
		r.publisher.Publish(ctx, events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: app.application.ID,
		})
	}
	r.publisher.Publish(ctx, events.Notification{
		Channel:       events.ChannelEpochClosed,
		ApplicationID: app.application.ID,
		EpochIndex:    epoch.Index,
	})

	return nil
}

func (r *Service) processApplicationOpenEpoch(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) error {
	r.Logger.Debug("Checking for inputs on current open epoch",
		"application", app.application.Name,
		"most_recent_block", mostRecentBlockNumber,
	)

	lastEpoch, err := r.repository.GetLastNonOpenEpoch(ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return fmt.Errorf("failed to get last non open epoch: %w", err)
	}
	if lastEpoch == nil {
		return r.setApplicationInoperable(ctx, app.application,
			"invalid state. no non open epochs found for application")
	}

	nextEpochNumber := lastEpoch.Index + 1
	openEpoch, err := r.repository.GetEpoch(ctx, app.application.IApplicationAddress.Hex(), nextEpochNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch epoch %d: %w", nextEpochNumber, err)
	}
	if openEpoch == nil {
		// Create epoch from sealed event
		openEpoch = &Epoch{
			Index:                nextEpochNumber,
			FirstBlock:           lastEpoch.LastBlock,
			LastBlock:            mostRecentBlockNumber,
			InputIndexLowerBound: lastEpoch.InputIndexUpperBound,
			InputIndexUpperBound: lastEpoch.InputIndexUpperBound,
			Status:               EpochStatus_Open,
		}
	}

	lastInputCheckBlock, err := r.repository.GetEventLastCheckBlock(ctx, app.application.ID, MonitoredEvent_InputAdded)
	if err != nil {
		return fmt.Errorf("failed to get last input check block: %w", err)
	}

	// Fetch inputs for this epoch from the InputBox
	inputCount, err := r.repository.GetNumberOfInputs(
		ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return fmt.Errorf(
			"failed to get number of inputs from repository: %w", err)
	}
	prevValue := new(big.Int).SetUint64(inputCount)
	inputs, err := r.fetchInputs(ctx, app,
		lastInputCheckBlock, mostRecentBlockNumber,
		prevValue,
		openEpoch.InputIndexLowerBound, math.MaxUint64)
	if err != nil {
		return fmt.Errorf("failed to fetch inputs for epoch %d: %w", openEpoch.Index, err)
	}

	// increase the upper bound according to the number of fetched inputs
	openEpoch.InputIndexUpperBound += uint64(len(inputs))
	openEpoch.LastBlock = mostRecentBlockNumber

	r.Logger.Debug("Storing open epoch",
		"application", app.application.Name,
		"epoch_number", openEpoch.Index,
		"new_inputs", len(inputs),
	)
	// Store epoch and inputs
	epochInputMap := map[*Epoch][]*Input{openEpoch: inputs}

	err = r.repository.CreateEpochsAndInputs(
		ctx,
		app.application.IApplicationAddress.String(),
		epochInputMap,
		mostRecentBlockNumber,
	)

	if err != nil {
		return fmt.Errorf("failed to store epoch and inputs: %w", err)
	}

	r.Logger.Debug("Stored open epoch and inputs",
		"application", app.application.Name,
		"epoch_number", nextEpochNumber,
		"num_inputs", len(inputs),
		"block", mostRecentBlockNumber)

	// Publish advisory events after successful DB write.
	if len(inputs) > 0 {
		r.publisher.Publish(ctx, events.Notification{
			Channel:       events.ChannelInputReceived,
			ApplicationID: app.application.ID,
		})
	}

	return nil
}
