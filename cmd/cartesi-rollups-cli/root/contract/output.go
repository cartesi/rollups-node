// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"fmt"
	"math/big"
	"os"
	"strconv"

	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/spf13/cobra"
)

var outputCmd = &cobra.Command{
	Use:   "output <application-address> [output-index]",
	Short: "Check output execution status",
	Args:  cobra.RangeArgs(1, 2), //nolint:mnd
	RunE:  runOutput,
}

func runOutput(cmd *cobra.Command, args []string) error {
	cc, cancel, err := initChainClient(cmd, args)
	if err != nil {
		return err
	}
	defer cancel()
	defer cc.eth.Close()

	if err := cc.ensureContract(cc.appAddr, "application"); err != nil {
		return err
	}

	app, err := iapplication.NewIApplicationCaller(cc.appAddr, cc.eth)
	if err != nil {
		return fmt.Errorf("bind IApplication: %w", err)
	}

	totalRaw, err := app.GetNumberOfExecutedOutputs(cc.callOpts)
	if err != nil {
		return fmt.Errorf("GetNumberOfExecutedOutputs: %w", err)
	}
	total, err := safeUint64(totalRaw, "executed outputs")
	if err != nil {
		return err
	}

	// Single output mode: check a specific output index.
	if len(args) > 1 {
		idx, parseErr := strconv.ParseUint(args[1], 10, 64)
		if parseErr != nil {
			return fmt.Errorf("invalid output index: %q", args[1])
		}
		executed, execErr := app.WasOutputExecuted(cc.callOpts, new(big.Int).SetUint64(idx))
		if execErr != nil {
			return fmt.Errorf("WasOutputExecuted(%d): %w", idx, execErr)
		}

		result := &OutputResult{OutputIndex: idx, Executed: executed}
		if jsonParam {
			return outputJSON(result)
		}

		p := &printer{w: os.Stdout}
		p.withSection(fmt.Sprintf("Output #%d", idx), func() {
			p.field("Executed", fmt.Sprintf("%t", executed))
		})
		p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
		return nil
	}

	// Batch mode: show summary of executed outputs.
	result := &OutputBatchResult{TotalExecuted: total}
	if jsonParam {
		return outputJSON(result)
	}

	p := &printer{w: os.Stdout}
	p.withSection(fmt.Sprintf("Outputs  (%d executed)", total), func() {})
	p.footer(cc.blockNum, cc.chainID, cc.resolveTimestamp(cc.blockNum))
	return nil
}
