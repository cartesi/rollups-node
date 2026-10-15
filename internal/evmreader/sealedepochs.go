// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func (r *Service) initialSealedEpochSearchBlock(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) (uint64, error) {
	r.Logger.Debug("Initializing application sealed epoch sync",
		"application", app.application.Name,
		"current_block", mostRecentBlockNumber,
	)
	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	}
	deploymentBlock, err := app.daveConsensus.GetDeploymentBlockNumber(callOpts)
	if err != nil {
		if errors.Is(err, bind.ErrNoCode) {
			r.Logger.Debug("Skipping sealed epoch sync before consensus deployment",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"consensus_address", app.application.IConsensusAddress,
				"block", mostRecentBlockNumber,
			)
			return 0, fmt.Errorf("%w: consensus %s at block %d: %w",
				errContractNotDeployedAtBlock,
				app.application.IConsensusAddress,
				mostRecentBlockNumber,
				err)
		}
		return 0, fmt.Errorf("failed to retrieve DaveConsensus deployment block: %w", err)
	}
	if deploymentBlock == nil || !deploymentBlock.IsUint64() || deploymentBlock.Sign() == 0 ||
		deploymentBlock.Uint64() > mostRecentBlockNumber {
		return 0, fmt.Errorf("invalid DaveConsensus deployment block %v at head %d", deploymentBlock, mostRecentBlockNumber)
	}

	// This is a local search floor, not evidence of a completed scan. Persisting
	// it before the constructor seal is stored could leave a cursor with no epoch.
	return deploymentBlock.Uint64(), nil
}

func (r *Service) scanDaveConsensusEpochsAndInputs(
	ctx context.Context,
	applications []appContracts,
	mostRecentBlockNumber uint64,
) bool {
	success := true
	r.Logger.Debug("Checking for new epochs and inputs", "apps", applications)

	// Process each application individually since each has its own DaveConsensus contract
	for _, app := range applications {
		if app.inputSource == nil {
			success = false
			r.Logger.Error("Cannot scan DaveConsensus epochs: InputBox adapter is missing",
				"application", app.application.Name,
				"address", app.application.IApplicationAddress,
				"input_box", app.application.IInputBoxAddress,
			)
			continue
		}
		r.Logger.Debug("Processing DaveConsensus application",
			"application", app.application.Name,
			"consensus_address", app.application.IConsensusAddress)

		observationEndBlock := foreclosureBoundedEndBlock(app.application, mostRecentBlockNumber)
		err := r.processApplicationSealedEpochs(ctx, app, observationEndBlock)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return false // shutting down
			}
			if errors.Is(err, ethutil.ErrIncompleteProcessing) {
				// This epoch has a known seal boundary. Finish its batches before
				// the open scan can ingest inputs belonging to a later epoch.
				continue
			}
			if errors.Is(err, errContractNotDeployedAtBlock) {
				// Registration can precede deployment visibility at the configured
				// block. Neither sealed nor open epochs can be observed yet.
				continue
			}
			if errors.Is(err, errApplicationStatusReported) {
				// Only a recorded integrity status makes this fault app-local.
				// A failed escalation from an execution terminal still fails the scan.
				if app.application.Status != ApplicationStatus_Corrupted && app.application.Status != ApplicationStatus_Diverged {
					success = false
				}
				continue
			}
			success = false
			r.Logger.Error("Error processing application sealed epochs",
				"application", app.application.Name,
				"consensus_address", app.application.IConsensusAddress,
				"error", err)
			continue
		}

		// Open-epoch input ingestion is L1 observation, not machine execution.
		// Keep it active for non-executing applications and bound it at the
		// foreclosure block just like the sealed-epoch scan above.
		err = r.processApplicationOpenEpoch(ctx, app, observationEndBlock)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return false // shutting down
			}
			if errors.Is(err, errApplicationStatusReported) {
				if app.application.Status != ApplicationStatus_Corrupted && app.application.Status != ApplicationStatus_Diverged {
					success = false
				}
				continue
			}
			success = false
			r.Logger.Error("Error processing application open epoch",
				"application", app.application.Name,
				"consensus_address", app.application.IConsensusAddress,
				"error", err)
			continue
		}
	}
	return success
}

func (r *Service) processApplicationSealedEpochs(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) error {
	lastEpochCheckBlock := app.application.LastEpochCheckBlock
	initializing := lastEpochCheckBlock == 0
	if initializing {
		deploymentBlock, err := r.initialSealedEpochSearchBlock(ctx, app, mostRecentBlockNumber)
		if err != nil {
			return fmt.Errorf("failed to determine start block for epoch search: %w", err)
		}
		lastEpochCheckBlock = deploymentBlock - 1
	}

	if mostRecentBlockNumber < lastEpochCheckBlock {
		if app.application.ForecloseBlock != 0 && app.application.ForecloseBlock == mostRecentBlockNumber {
			r.Logger.Debug("Sealed epoch scan already covers foreclosure block",
				"application", app.application.Name, "foreclose_block", mostRecentBlockNumber,
				"last_epoch_check_block", lastEpochCheckBlock)
			return nil // The independent input cursor can still require a drain.
		}
		r.Logger.Warn(
			"Not reading sealed epochs: most recent block is lower than the last processed one",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"last_epoch_check_block", app.application.LastEpochCheckBlock,
			"most_recent_block", mostRecentBlockNumber,
		)
		return nil
	} else if mostRecentBlockNumber == lastEpochCheckBlock {
		r.Logger.Debug("Not reading sealed epochs: already checked the most recent blocks",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"last_epoch_check_block", app.application.LastEpochCheckBlock,
			"most_recent_block", mostRecentBlockNumber,
		)
		return nil
	}

	nextSearchBlock := lastEpochCheckBlock + 1
	r.Logger.Debug("Checking sealed epochs for application",
		"application", app.application.Name,
		"last_epoch_check_block", lastEpochCheckBlock,
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

	// A failed window may already have stored a prefix of its epochs. Seed from
	// the completed scan boundary, not the newest row, so that prefix is replayed.
	prevValue := big.NewInt(-1)
	if !initializing {
		var err error
		prevValue, err = oracle(ctx, lastEpochCheckBlock)
		if err != nil {
			return err
		}
	}
	onHit := func(block uint64, prevValue, _ *big.Int) error {
		return r.processEpochTransition(ctx, app, block, prevValue)
	}

	// Use FindTransitions to find epoch transitions
	_, lastProcessedBlock, err := ethutil.FindTransitions(ctx, nextSearchBlock, mostRecentBlockNumber, prevValue, nil, oracle, onHit)
	var incomplete bool
	switch {
	case errors.Is(err, ethutil.ErrIncompleteProcessing):
		incomplete = true
	case err != nil:
		return fmt.Errorf("failed to walk epoch transitions: %w", err)
	}

	// Update the last check block for this application
	err = r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_EpochSealed, lastProcessedBlock)
	if err != nil {
		return fmt.Errorf("failed to update last epoch check block: %w", err)
	}
	if incomplete {
		return ethutil.ErrIncompleteProcessing
	}
	if initializing {
		r.Logger.Info("Application sealed epoch sync initialized",
			"application", app.application.Name, "consensus_address", app.application.IConsensusAddress,
			"deployment_block", nextSearchBlock,
			"last_epoch_check_block", mostRecentBlockNumber)
	}

	r.Logger.Debug("Sealed epoch search completed", "application", app.application.Name, "most_recent_block", mostRecentBlockNumber)

	return nil
}

func (r *Service) processEpochTransition(
	ctx context.Context,
	app appContracts,
	transitionBlock uint64,
	previousEpochNumber *big.Int,
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

	// Validate the whole block before storing any event. Several epochs can seal
	// in one block, so a nonempty page can still omit part of the transition.
	if sealedEpoch.EpochNumber == nil || !sealedEpoch.EpochNumber.IsUint64() {
		return fmt.Errorf("invalid sealed epoch number at block %d: %v", transitionBlock, sealedEpoch.EpochNumber)
	}
	expectedCount := new(big.Int).Sub(sealedEpoch.EpochNumber, previousEpochNumber)
	if expectedCount.Sign() <= 0 || !expectedCount.IsUint64() || expectedCount.Uint64() != uint64(len(sealedEvents)) {
		return fmt.Errorf("sealed epoch event count mismatch at block %d: expected %s, got %d",
			transitionBlock, expectedCount, len(sealedEvents))
	}
	expectedEpoch := new(big.Int).Set(previousEpochNumber)
	for _, event := range sealedEvents {
		expectedEpoch.Add(expectedEpoch, big.NewInt(1))
		if event == nil || event.EpochNumber == nil || event.EpochNumber.Cmp(expectedEpoch) != 0 ||
			event.Raw.BlockNumber != transitionBlock {
			return fmt.Errorf("invalid sealed epoch event sequence at block %d: expected epoch %s", transitionBlock, expectedEpoch)
		}
	}
	lastEvent := sealedEvents[len(sealedEvents)-1]
	if lastEvent.InputIndexLowerBound.Cmp(sealedEpoch.InputIndexLowerBound) != 0 ||
		lastEvent.InputIndexUpperBound.Cmp(sealedEpoch.InputIndexUpperBound) != 0 || lastEvent.Tournament != sealedEpoch.Tournament {
		return fmt.Errorf("last sealed epoch event does not match current sealed epoch at block %d", transitionBlock)
	}
	r.Logger.Info("Found sealed epoch events",
		"application", app.application.Name, "block", transitionBlock,
		"epoch_number", sealedEpoch.EpochNumber, "count", len(sealedEvents))

	// Process each sealed epoch event
	for _, event := range sealedEvents {
		err := r.processSealedEpochEvent(ctx, app, event)
		if err != nil {
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

	// A seal is emitted by a transaction, so it cannot occur at genesis.
	if event.Raw.BlockNumber == 0 {
		return errors.New("sealed epoch event has block number zero")
	}

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

	var unprocessedFirstBlock uint64
	var unprocessedIndexLowerBound uint64

	if epoch == nil {
		// Create new epoch from sealed event
		epoch = &Epoch{
			Index:                event.EpochNumber.Uint64(),
			FirstBlock:           firstBlock,
			LastBlock:            event.Raw.BlockNumber,
			InputIndexLowerBound: event.InputIndexLowerBound.Uint64(),
			InputIndexUpperBound: event.InputIndexUpperBound.Uint64(),
			TournamentAddress:    &event.Tournament,
			Status:               EpochStatus_Open, // Sealed epochs that are not fully processed are still open
		}

		// setup the entire epoch as unprocessed
		unprocessedFirstBlock = epoch.FirstBlock
		unprocessedIndexLowerBound = epoch.InputIndexLowerBound
	} else {
		if epoch.FirstBlock != firstBlock || epoch.InputIndexLowerBound != event.InputIndexLowerBound.Uint64() {
			return fmt.Errorf("epoch %d data mismatch with sealed event", epoch.Index)
		}
		if epoch.Status != EpochStatus_Open {
			if epoch.LastBlock != event.Raw.BlockNumber || epoch.InputIndexUpperBound != event.InputIndexUpperBound.Uint64() ||
				epoch.TournamentAddress == nil || *epoch.TournamentAddress != event.Tournament {
				return fmt.Errorf("sealed epoch %d data mismatch with replayed event", epoch.Index)
			}
			// The epoch and its inputs were stored atomically. Preserve any later
			// claim/proof progress when replaying a partially completed scan.
			return nil
		}

		// LastBlock may contain only a prefix of its inputs. Revisit it with
		// the saved index bound, which excludes inputs already stored.
		unprocessedFirstBlock = epoch.LastBlock
		unprocessedIndexLowerBound = epoch.InputIndexUpperBound

		epoch.LastBlock = event.Raw.BlockNumber
		epoch.InputIndexUpperBound = event.InputIndexUpperBound.Uint64()
		epoch.TournamentAddress = &event.Tournament
	}

	unprocessedIndexUpperBound := min(epoch.InputIndexUpperBound, r.inputBatchUpperBound(unprocessedIndexLowerBound))

	// Fetch inputs for this epoch from the InputBox.
	// Always search from unprocessedFirstBlock (not lastInputCheckBlock+1) because with
	// PRT's overlapping block boundaries (sealed epoch's LastBlock = next epoch's
	// FirstBlock), inputs for this epoch may exist at the overlap block — added in
	// a later transaction than the previous epoch's seal within the same block.
	var inputs []*Input
	if unprocessedIndexLowerBound < unprocessedIndexUpperBound {
		var err error
		var lastInputBlock uint64
		inputs, lastInputBlock, _, err = r.fetchInputs(ctx, app,
			unprocessedFirstBlock, epoch.LastBlock,
			new(big.Int).SetUint64(unprocessedIndexLowerBound),
			unprocessedIndexLowerBound, unprocessedIndexUpperBound)
		if err != nil && !errors.Is(err, ethutil.ErrIncompleteProcessing) {
			return fmt.Errorf("failed to fetch inputs for epoch %d: %w", epoch.Index, err)
		}

		actualCount := uint64(len(inputs))
		expectedCount := unprocessedIndexUpperBound - unprocessedIndexLowerBound
		if actualCount != expectedCount {
			return fmt.Errorf(
				"epoch %d input count mismatch: expected %d (indices %d to %d), got %d",
				epoch.Index, expectedCount,
				unprocessedIndexLowerBound, epoch.InputIndexUpperBound,
				actualCount)
		}

		// Epoch bounds include observed inputs, even in a partial block. The
		// application input cursor separately covers only complete blocks.
		epoch.LastBlock = inputObservationLastBlock(epoch.FirstBlock, lastInputBlock, inputs)
		epoch.InputIndexUpperBound = unprocessedIndexUpperBound
	}

	if unprocessedIndexUpperBound == event.InputIndexUpperBound.Uint64() {
		epoch.LastBlock = event.Raw.BlockNumber
		epoch.Status = EpochStatus_Closed // Fully processed sealed epochs are closed
	}

	// The seal covers every input before its transaction, but later inputs in
	// the same block belong to the next epoch. Publish only complete blocks;
	// the open-epoch scan must finish this boundary block before drain is safe.
	epochInputMap := map[*Epoch][]*Input{epoch: inputs}

	r.Logger.Debug("Storing sealed epoch", "application", app.application.Name, "epoch_number", epoch.Index)

	err = r.repository.CreateEpochsAndInputs(
		ctx,
		app.application.IApplicationAddress.String(),
		epochInputMap,
		epoch.LastBlock-1, // last block must be rescanned for inputs for the next epoch
	)
	if err != nil {
		if errors.Is(err, repository.ErrInputLogIdentityConflict) {
			return r.setApplicationCorrupted(ctx, app.application,
				"sealed epoch %d input L1 log identity conflicts with stored data; operator reset required: %v", epoch.Index, err)
		}
		return fmt.Errorf("failed to store epoch and inputs: %w", err)
	}

	if epoch.Status != EpochStatus_Closed {
		return ethutil.ErrIncompleteProcessing
	}

	r.Logger.Debug("Stored sealed epoch and inputs",
		"application", app.application.Name,
		"epoch_number", epoch.Index,
		"num_inputs", len(inputs),
		"block", event.Raw.BlockNumber)

	return nil
}

func (r *Service) processApplicationOpenEpoch(
	ctx context.Context,
	app appContracts,
	mostRecentBlockNumber uint64,
) error {
	// The input cursor covers complete blocks. A sealed batch in this tick
	// can advance it only to the block before its seal.
	if mostRecentBlockNumber < app.application.LastInputCheckBlock {
		r.Logger.Warn(
			"Not checking for inputs on current open epoch: most recent block is lower than the last processed one",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"last_input_check_block", app.application.LastInputCheckBlock,
			"most_recent_block", mostRecentBlockNumber,
		)
		return nil
	} else if mostRecentBlockNumber == app.application.LastInputCheckBlock {
		r.Logger.Debug("Not checking for inputs on current open epoch: already checked the most recent blocks",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"last_input_check_block", app.application.LastInputCheckBlock,
			"most_recent_block", mostRecentBlockNumber,
		)
		return nil
	}

	r.Logger.Debug("Checking for inputs on current open epoch",
		"application", app.application.Name,
		"most_recent_block", mostRecentBlockNumber,
	)

	lastEpoch, err := r.repository.GetLastNonOpenEpoch(ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return fmt.Errorf("failed to get last non open epoch: %w", err)
	}
	if lastEpoch == nil {
		return r.setApplicationCorrupted(ctx, app.application,
			"invalid state. no non open epochs found for application")
	}

	nextEpochNumber := lastEpoch.Index + 1
	openEpoch, err := r.repository.GetEpoch(ctx, app.application.IApplicationAddress.Hex(), nextEpochNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch epoch %d: %w", nextEpochNumber, err)
	}
	if openEpoch == nil {
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
	if lastInputCheckBlock >= mostRecentBlockNumber {
		return nil
	}

	// Fetch inputs for this epoch from the InputBox
	inputCount, err := r.repository.GetNumberOfInputs(
		ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return fmt.Errorf(
			"failed to get number of inputs from repository: %w", err)
	}
	inputs, lastInputBlock, endCount, err := r.fetchInputs(ctx, app,
		lastInputCheckBlock+1, mostRecentBlockNumber,
		new(big.Int).SetUint64(inputCount),
		openEpoch.InputIndexUpperBound, r.inputBatchUpperBound(openEpoch.InputIndexUpperBound))
	if err != nil {
		return fmt.Errorf("failed to fetch inputs for epoch %d: %w", openEpoch.Index, err)
	}
	if endCount < inputCount || endCount-inputCount != uint64(len(inputs)) {
		return fmt.Errorf("open epoch %d input count mismatch: stored %d, on-chain %d, got %d",
			openEpoch.Index, inputCount, endCount, len(inputs))
	}

	// increase the upper bound according to the number of fetched inputs.
	openEpoch.InputIndexUpperBound += uint64(len(inputs))
	openEpoch.LastBlock = inputObservationLastBlock(openEpoch.FirstBlock, lastInputBlock, inputs)

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
		lastInputBlock,
	)

	if err != nil {
		if errors.Is(err, repository.ErrInputLogIdentityConflict) {
			return r.setApplicationCorrupted(ctx, app.application,
				"open epoch %d input L1 log identity conflicts with stored data; operator reset required: %v", openEpoch.Index, err)
		}
		return fmt.Errorf("failed to store epoch and inputs: %w", err)
	}

	r.Logger.Debug("Stored open epoch and inputs",
		"application", app.application.Name,
		"epoch_number", nextEpochNumber,
		"num_inputs", len(inputs),
		"block", mostRecentBlockNumber)

	return nil
}

// inputObservationLastBlock keeps epoch bounds valid when the first observed
// block is only partially ingested and lastCompleteBlock precedes firstBlock.
func inputObservationLastBlock(firstBlock, lastCompleteBlock uint64, inputs []*Input) uint64 {
	lastBlock := max(firstBlock, lastCompleteBlock)
	for _, input := range inputs {
		lastBlock = max(lastBlock, input.BlockNumber)
	}
	return lastBlock
}
