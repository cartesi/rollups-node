// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/daveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

func (r *Service) checkForEpochsAndInputs(
	ctx context.Context,
	applications []appContracts,
	mostRecentBlockNumber uint64,
) {
	if !r.inputReaderEnabled {
		return
	}

	r.Logger.Debug("Checking for new epochs and inputs")

	// Process each application individually since each has its own DaveConsensus contract
	for _, app := range applications {
		r.Logger.Debug("Processing DaveConsensus application",
			"application", app.application.Name,
			"consensus_address", app.application.IConsensusAddress)

		err := r.processApplicationEpochs(ctx, app, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Error processing application epochs",
				"application", app.application.Name,
				"consensus_address", app.application.IConsensusAddress,
				"error", err)
			continue
		}
	}
}

func (r *Service) processApplicationEpochs(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) error {
	// Find the starting block for epoch search
	startBlock, err := r.findEpochSearchStartBlock(ctx, app, mostRecentBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to determine start block for epoch search: %w", err)
	}

	r.Logger.Debug("Starting epoch search",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", mostRecentBlockNumber)

	if startBlock >= mostRecentBlockNumber {
		r.Logger.Debug("No new blocks to search for epochs")
		return nil
	}

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

	if app.application.LastEpochCheckBlock == 0 {
		r.Logger.Debug("Processing initial epoch state", "application", app.application.Name, "block", startBlock)
		err := onHit(startBlock)
		if err != nil {
			return fmt.Errorf("failed to walk epoch transitions: %w", err)
		}
	}
	// Use FindTransitions to find epoch transitions
	err = FindTransitions(ctx, startBlock, mostRecentBlockNumber, oracle, onHit)
	if err != nil {
		return fmt.Errorf("failed to walk epoch transitions: %w", err)
	}

	// Update the last check block for this application
	err = r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_EpochSealed, mostRecentBlockNumber)
	if err != nil {
		return fmt.Errorf("failed to update last epoch check block: %w", err)
	}

	r.Logger.Debug("Epoch search completed", "application", app.application.Name, "up_to_block", mostRecentBlockNumber)

	return nil
}

func (r *Service) findEpochSearchStartBlock(ctx context.Context, app appContracts, mostRecentBlockNumber uint64) (uint64, error) {
	if app.application.LastEpochCheckBlock == 0 {
		r.Logger.Debug("Searching DaveConsensus deployment block",
			"application", app.application.Name, "consensus", app.application.IConsensusAddress.Hex())
		// Find DaveConsensus deployment block. We can start looking after application deployment
		appBlock, err := app.applicationContract.GetDeploymentBlockNumber(nil)
		if err != nil {
			return 0, fmt.Errorf("failed to get application deployment block: %w", err)
		}

		// Create oracle function that returns 1 if DaveConsensus is deployed at a given block, 0 otherwise
		oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
			r.Logger.Debug("Checking DaveConsensus deployment",
				"application", app.application.Name, "consensus", app.application.IConsensusAddress.Hex(), "block", block)
			callOpts := &bind.CallOpts{
				Context:     ctx,
				BlockNumber: new(big.Int).SetUint64(block),
			}

			_, err := app.daveConsensus.GetCurrentSealedEpoch(callOpts)
			if err != nil {
				if errors.Is(err, bind.ErrNoCode) {
					return big.NewInt(0), nil
				}
				return nil, fmt.Errorf("failed to get current sealed epoch at block %d: %w", block, err)
			}

			return big.NewInt(1), nil
		}

		var daveConsensusBlock uint64
		// Create onHit function that saves de deployment block transition
		onHit := func(block uint64) error {
			r.Logger.Debug("DaveConsensus deployment found",
				"application", app.application.Name, "consensus", app.application.IConsensusAddress.Hex(), "block", block)
			daveConsensusBlock = block
			return nil
		}

		// Use FindTransitions to find contract deployment transitions
		err = FindTransitions(ctx, appBlock.Uint64(), mostRecentBlockNumber, oracle, onHit)
		if err != nil {
			return 0, fmt.Errorf("failed to walk epoch transitions: %w", err)
		}

		return daveConsensusBlock, nil
	}
	return app.application.LastEpochCheckBlock, nil
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
			continue
		}
	}

	return nil
}

func (r *Service) processSealedEpochEvent(
	ctx context.Context,
	app appContracts,
	event *daveconsensus.DaveConsensusEpochSealed,
) error {
	r.Logger.Debug("Processing sealed epoch event",
		"epoch_number", event.EpochNumber,
		"input_lower_bound", event.InputIndexLowerBound,
		"input_upper_bound", event.InputIndexUpperBound,
		"tournament", event.Tournament)

	firstBlock := uint64(0)
	epochNumber := event.EpochNumber.Uint64()
	if epochNumber == 0 {
		firstBlock = app.application.IInputBoxBlock
	} else {
		prevEpochNumber := epochNumber - 1
		prevEpoch, err := r.repository.GetEpoch(ctx, app.application.IApplicationAddress.Hex(), prevEpochNumber)
		if err != nil || prevEpoch == nil {
			return fmt.Errorf("failed to fetch epoch %d: %w", prevEpochNumber, err)
		}

		prevEpoch.ClaimTransactionHash = &event.Raw.TxHash
		err = r.repository.UpdateEpoch(ctx, app.application.IApplicationAddress.Hex(), prevEpoch)
		if err != nil {
			return fmt.Errorf("failed to update previous epoch %d: %w", prevEpochNumber, err)
		}
		firstBlock = prevEpoch.LastBlock
	}

	// Create epoch from sealed event
	epoch := &Epoch{
		Index:                event.EpochNumber.Uint64(),
		FirstBlock:           firstBlock, // Will be calculated based on epoch length
		LastBlock:            event.Raw.BlockNumber,
		InputIndexLowerBound: event.InputIndexLowerBound.Uint64(),
		InputIndexUpperBound: event.InputIndexUpperBound.Uint64(),
		TournamentAddress:    &event.Tournament,
		Status:               EpochStatus_Closed, // Sealed epochs are closed
	}

	// Fetch inputs for this epoch from the InputBox
	var inputs []*Input
	if epoch.InputIndexUpperBound > epoch.InputIndexLowerBound {
		var err error
		inputs, err = r.fetchInputsForEpoch(ctx, app, epoch)
		if err != nil {
			return fmt.Errorf("failed to fetch inputs for epoch %d: %w", epoch.Index, err)
		}
	}
	// Store epoch and inputs
	epochInputMap := map[*Epoch][]*Input{
		epoch: inputs,
	}

	r.Logger.Debug("Storing sealed epoch",
		"application", app.application.Name,
		"epoch_number", epoch.Index,
	)
	err := r.repository.CreateEpochsAndInputs(
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

	return nil
}

func (r *Service) fetchInputsForEpoch(
	ctx context.Context,
	app appContracts,
	epoch *Epoch,
) ([]*Input, error) {
	r.Logger.Debug("Fetching inputs for epoch",
		"application", app.application.Name,
		"epoch_index", epoch.Index,
		"input_lower_bound", epoch.InputIndexLowerBound,
		"input_upper_bound", epoch.InputIndexUpperBound,
		"epoch_first_block", epoch.FirstBlock,
		"epoch_last_block", epoch.LastBlock,
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
			if event.Index.Uint64() >= epoch.InputIndexLowerBound && event.Index.Uint64() < epoch.InputIndexUpperBound {
				input := &Input{
					Index:                event.Index.Uint64(),
					Status:               InputCompletionStatus_None,
					RawData:              event.Input,
					BlockNumber:          event.Raw.BlockNumber,
					TransactionReference: event.Raw.TxHash,
				}
				sortedInputs = insertSorted(sortByInputIndex, sortedInputs, input)
			}
		}
		return nil
	}

	// Always process the first block in case there are inputs there
	err := onHit(epoch.FirstBlock)
	if err != nil {
		return nil, err
	}

	// Use FindTransitions to find blocks where inputs were added
	err = FindTransitions(ctx, epoch.FirstBlock, epoch.LastBlock, oracle, onHit)
	if err != nil {
		return nil, fmt.Errorf("failed to walk input transitions: %w", err)
	}

	r.Logger.Info("Fetched inputs for epoch",
		"application", app.application.Name,
		"epoch_index", epoch.Index,
		"input_count", len(sortedInputs),
	)
	return sortedInputs, nil
}
