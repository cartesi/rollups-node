// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// Find an appropriate value to start scanning the blockchain for executed outputs of this application.
// Application deployment block is a safe block to start since this event is emitted by the application contract itself.
// We use input box block as a fallback.
func (r *Service) initializeNewApplicationOutputExecutionSync(
	ctx context.Context,
	app *appContracts,
	mostRecentBlockNumber uint64,
) (uint64, error) {
	r.Logger.Info("Initializing application output execution sync",
		"application", app.application.Name,
		"current_block", mostRecentBlockNumber,
	)
	callOpts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	}
	deploymentBlock, err := app.applicationContract.GetDeploymentBlockNumber(callOpts)
	if err != nil {
		r.Logger.Error("Error retrieving application deployment block number",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"error", err,
		)
		return 0, err
	}
	if deploymentBlock.Sign() <= 0 {
		r.Logger.Error("Invalid application deployment block number retrieved",
			"application", app.application.Name,
			"address", app.application.IApplicationAddress,
			"block_number", deploymentBlock.Uint64(),
		)
		return 0, errors.New("invalid application deployment block number retrieved")
	}

	lastOutputCheckBlock := deploymentBlock.Uint64() - 1
	err = r.repository.UpdateEventLastCheckBlock(ctx, []int64{app.application.ID}, MonitoredEvent_OutputExecuted, lastOutputCheckBlock)
	if err != nil {
		r.Logger.Error("Failed to update application LastOutputCheckBlock",
			"application", app.application.Name,
			"last_output_check_block", lastOutputCheckBlock,
			"error", err,
		)
		return 0, err
	}
	r.Logger.Debug("Application output execution sync initialized",
		"application", app.application.Name,
		"deployment_block", deploymentBlock.Uint64(),
		"next_search_block", lastOutputCheckBlock+1,
		"current_block", mostRecentBlockNumber,
	)
	app.application.LastOutputCheckBlock = lastOutputCheckBlock
	return app.application.LastOutputCheckBlock, nil
}

func (r *Service) checkForOutputExecution(
	ctx context.Context,
	apps []appContracts,
	mostRecentBlockNumber uint64,
) {

	appAddresses := appsToAddresses(apps)

	r.Logger.Debug("Checking for new Output Executed Events", "apps", appAddresses)

	for _, app := range apps {
		lastOutputCheck := app.application.LastOutputCheckBlock
		if lastOutputCheck == 0 { // New application. Find a safe start block to scan for outputs
			var err error
			lastOutputCheck, err = r.initializeNewApplicationOutputExecutionSync(ctx, &app, mostRecentBlockNumber)
			if err != nil {
				r.Logger.Error("Failed to initialize application output execution sync",
					"application", app.application.Name,
					"most_recent_block", mostRecentBlockNumber,
					"error", err,
				)
				continue
			}
		}

		if mostRecentBlockNumber > lastOutputCheck {

			r.Logger.Debug("Checking output execution for application",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"last_output_check_block", lastOutputCheck,
				"most_recent_block", mostRecentBlockNumber)

			r.readAndUpdateOutputs(ctx, app, lastOutputCheck, mostRecentBlockNumber)

		} else if mostRecentBlockNumber < lastOutputCheck {
			r.Logger.Warn(
				"Not reading output execution: most recent block is lower than the last processed one",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"last_output_check_block", lastOutputCheck,
				"most_recent_block", mostRecentBlockNumber,
			)
		} else {
			r.Logger.Debug("Not reading output execution: already checked the most recent blocks",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"last_output_check_block", lastOutputCheck,
				"most_recent_block", mostRecentBlockNumber,
			)
		}
	}

}

func (r *Service) readAndUpdateOutputs(
	ctx context.Context, app appContracts, lastOutputCheck, mostRecentBlockNumber uint64) {

	nextSearchBlock := lastOutputCheck + 1
	outputExecutedEvents, err := r.readOutputExecutionsFromBlockChain(ctx, app, nextSearchBlock, mostRecentBlockNumber)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return // shutting down
		}
		r.Logger.Error("Error reading output events",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"error", err)
		return
	}

	if len(outputExecutedEvents) == 0 {
		r.Logger.Debug("No output executed events found",
			"application", app.application.Name, "address", app.application.IApplicationAddress)
		err := r.repository.UpdateEventLastCheckBlock(
			ctx, []int64{app.application.ID}, MonitoredEvent_OutputExecuted, mostRecentBlockNumber)
		if err != nil {
			r.Logger.Error("Failed to update LastOutputCheckBlock for applications without inputs",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"block_number", mostRecentBlockNumber,
				"error", err,
			)
			// We don't return an error here as there is no output execution to process
			// and this is just an update to the last check block
		} else {
			r.Logger.Debug("Updated LastOutputCheckBlock for applications without inputs",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"block_number", mostRecentBlockNumber,
			)
		}
		return
	}
	r.Logger.Debug("Found output executed events",
		"application", app.application.Name,
		"address", app.application.IApplicationAddress,
		"output_executed_events", len(outputExecutedEvents),
	)

	// Should we check the output hash??
	executedOutputs := make([]*Output, 0, len(outputExecutedEvents))
	for _, event := range outputExecutedEvents {

		// Compare output to check it is the correct one
		output, err := r.repository.GetOutput(ctx, app.application.IApplicationAddress.Hex(), event.OutputIndex)
		if err != nil {
			r.Logger.Error("Error retrieving output",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"index", event.OutputIndex,
				"error", err)
			return
		}

		if output == nil {
			r.Logger.Warn("Found OutputExecuted event but output does not exist in the database yet",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"index", event.OutputIndex)
			return
		}

		if !bytes.Equal(output.RawData, event.Output) {
			// setApplicationInoperable always returns non-nil (the reason text itself).
			// The DB error case is already logged inside setApplicationState.
			// On DB success the app is marked inoperable and won't reappear next tick.
			// On DB failure the app reappears as Enabled next tick, retrying this path.
			_ = r.setApplicationInoperable(ctx, app.application,
				"Output mismatch. Application is in an invalid state. Output Index %d, raw data %s != event data %s",
				output.Index,
				"0x"+hex.EncodeToString(output.RawData),
				"0x"+hex.EncodeToString(event.Output),
			)
			return
		}

		r.Logger.Info("Output executed",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"index", event.OutputIndex)
		output.ExecutionTransactionHash = &event.Raw.TxHash
		executedOutputs = append(executedOutputs, output)
	}

	err = r.repository.UpdateOutputsExecution(
		ctx, app.application.IApplicationAddress.Hex(), executedOutputs, mostRecentBlockNumber)
	if err != nil {
		r.Logger.Error("Error storing output execution statuses",
			"application", app.application.Name, "address", app.application.IApplicationAddress,
			"error", err)
	}

}

func (r *Service) readOutputExecutionsFromBlockChain(
	ctx context.Context,
	app appContracts,
	startBlock, endBlock uint64,
) ([]*iapplication.IApplicationOutputExecuted, error) {
	r.Logger.Debug("Fetching Output Execution events for application",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", endBlock,
	)

	// Define oracle function that returns the number of output execution events at a given block
	oracle := func(ctx context.Context, block uint64) (*big.Int, error) {
		callOpts := &bind.CallOpts{
			Context:     ctx,
			BlockNumber: new(big.Int).SetUint64(block),
		}
		numOutputs, err := app.applicationContract.GetNumberOfExecutedOutputs(callOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to get number of executed outputs at block %d: %w", block, err)
		}
		return numOutputs, nil
	}

	var executedOutputs []*iapplication.IApplicationOutputExecuted
	// Define onHit function that accumulates executed outputs at transition blocks
	onHit := func(block uint64) error {
		filterOpts := &bind.FilterOpts{
			Context: ctx,
			Start:   block,
			End:     &block,
		}
		execEvents, err := app.applicationContract.RetrieveOutputExecutionEvents(filterOpts)
		if err != nil {
			return fmt.Errorf("failed to retrieve output execution events at block %d: %w", block, err)
		}
		executedOutputs = append(executedOutputs, execEvents...)
		return nil
	}

	prevValue := &big.Int{}
	execCount, err := r.repository.GetNumberOfExecutedOutputs(ctx, app.application.IApplicationAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get number of executed outputs from repository: %w", err)
	}
	prevValue.SetUint64(execCount)

	// Use FindTransitions to find blocks where outputs were executed
	_, err = ethutil.FindTransitions(ctx, startBlock, endBlock, prevValue, oracle, onHit)
	if err != nil {
		return nil, fmt.Errorf("failed to walk output execution transitions: %w", err)
	}

	r.Logger.Debug("Fetched output executed events for application",
		"application", app.application.Name,
		"start_block", startBlock,
		"end_block", endBlock,
		"prev_executed_output_count", prevValue.Uint64(),
		"new_executed_outputs", len(executedOutputs),
	)
	return executedOutputs, nil
}
