// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// Find an appropriate value to start scanning the blockchain for executed outputs of this application.
// Application deployment block is a safe block to start since this event is emitted by the application contract itself.
// We use input box block as a fallback.
func findLastOutputCheckBlock(app *appContracts, mostRecentBlockNumberCallOpts *bind.CallOpts) uint64 {
	deploymentBlockNumberBig, err := app.applicationContract.GetDeploymentBlockNumber(mostRecentBlockNumberCallOpts)
	if err != nil {
		return app.application.IInputBoxBlock - 1
	}
	return deploymentBlockNumberBig.Uint64() - 1
}

func (r *Service) checkForOutputExecution(
	ctx context.Context,
	apps []appContracts,
	mostRecentBlockNumber uint64,
) {

	appAddresses := appsToAddresses(apps)

	r.Logger.Debug("Checking for new Output Executed Events", "apps", appAddresses)

	mostRecentBlockNumberCallOpts := &bind.CallOpts{
		BlockNumber: new(big.Int).SetUint64(mostRecentBlockNumber),
	}

	for _, app := range apps {
		lastOutputCheck := app.application.LastOutputCheckBlock
		if lastOutputCheck == 0 { // New application. Find a safe start block to scan for outputs
			lastOutputCheck = findLastOutputCheckBlock(&app, mostRecentBlockNumberCallOpts)
			r.Logger.Info("Initializing application output execution sync",
				"application", app.application.Name,
				"inputbox_block", app.application.IInputBoxBlock,
				"next_search_block", lastOutputCheck+1,
				"current_block", mostRecentBlockNumber,
			)
			app.application.LastOutputCheckBlock = lastOutputCheck
		}

		if mostRecentBlockNumber > lastOutputCheck {

			r.Logger.Debug("Checking output execution for application",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"last_output_check block", lastOutputCheck,
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
			r.Logger.Warn("Not reading output execution: already checked the most recent blocks",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"last output check block", lastOutputCheck,
				"most recent block", mostRecentBlockNumber,
			)
		}
	}

}

func (r *Service) readAndUpdateOutputs(
	ctx context.Context, app appContracts, lastOutputCheck, mostRecentBlockNumber uint64) {

	contract := app.applicationContract

	opts := &bind.FilterOpts{
		Context: ctx,
		Start:   lastOutputCheck + 1,
		End:     &mostRecentBlockNumber,
	}

	outputExecutedEvents, err := contract.RetrieveOutputExecutionEvents(opts)
	if err != nil {
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

	// Should we check the output hash??
	var executedOutputs []*Output
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
			r.Logger.Debug("Output mismatch",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"index", event.OutputIndex,
				"actual", output.RawData,
				"event's", event.Output)

			r.Logger.Error("Output mismatch. Application is in an invalid state",
				"application", app.application.Name, "address", app.application.IApplicationAddress,
				"index", event.OutputIndex)

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
