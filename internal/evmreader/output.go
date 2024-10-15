// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"bytes"
	"context"
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

func (r *EvmReader) checkForOutputExecution(
	ctx context.Context,
	apps []application,
	mostRecentBlockNumber uint64,
) {

	appAddresses := appsToAddresses(apps)

	slog.Debug("Checking for new Output Executed Events", "apps", appAddresses)

	for _, app := range apps {

		LastOutputCheck := app.LastOutputCheckBlock

		// Safeguard: Only check blocks starting from the block where the InputBox
		// contract was deployed as Inputs can be added to that same block
		if LastOutputCheck < r.inputBoxDeploymentBlock {
			LastOutputCheck = r.inputBoxDeploymentBlock
		}

		if mostRecentBlockNumber > LastOutputCheck {

			slog.Info("Checking output execution for application",
				"app", app.ContractAddress,
				"last output check block", LastOutputCheck,
				"most recent block", mostRecentBlockNumber)

			r.readAndUpdateOutputs(ctx, app, LastOutputCheck, mostRecentBlockNumber)

		} else if mostRecentBlockNumber < LastOutputCheck {
			slog.Warn(
				"Not reading output execution: most recent block is lower than the last processed one", //nolint:lll
				"app", app.ContractAddress,
				"last output check block", LastOutputCheck,
				"most recent block", mostRecentBlockNumber,
			)
		} else {
			slog.Info("Not reading output execution: already checked the most recent blocks",
				"app", app.ContractAddress,
				"last output check block", LastOutputCheck,
				"most recent block", mostRecentBlockNumber,
			)
		}
	}

}

func (r *EvmReader) readAndUpdateOutputs(
	ctx context.Context, app application, lastOutputCheck, mostRecentBlockNumber uint64) {

	contract := app.applicationContract

	opts := &bind.FilterOpts{
		Start: lastOutputCheck + 1,
		End:   &mostRecentBlockNumber,
	}

	outputExecutedEvents, err := contract.RetrieveOutputExecutionEvents(opts)
	if err != nil {
		slog.Error("Error reading output events", "app", app.ContractAddress, "error", err)
		return
	}

	// Should we check the output hash??
	var executedOutputs []*Output
	for _, event := range outputExecutedEvents {

		// Compare output to check it is the correct one
		output, err := r.repository.GetOutput(ctx, event.OutputIndex, app.ContractAddress)
		if err != nil {
			slog.Error("Error retrieving output",
				"app", app.ContractAddress, "index", event.OutputIndex, "error", err)
			return
		}

		if !bytes.Equal(output.RawData, event.Output) {
			slog.Debug("Output mismatch",
				"app", app.ContractAddress, "index", event.OutputIndex,
				"actual", output.RawData, "event's", event.Output)

			slog.Error("Output mismatch. Application is in an invalid state",
				"app", app.ContractAddress,
				"index", event.OutputIndex)

			return
		}

		slog.Info("Output executed", "app", app.ContractAddress, "index", event.OutputIndex)
		output.TransactionHash = &event.Raw.TxHash
		executedOutputs = append(executedOutputs, output)
	}

	err = r.repository.UpdateOutputExecutionTransaction(
		ctx, app.ContractAddress, executedOutputs, mostRecentBlockNumber)
	if err != nil {
		slog.Error("Error storing output execution statuses", "app", app, "error", err)
	}

}
