// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package outputs

import (
	"encoding/json"
	"fmt"
	"os"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "outputs",
	Short:   "Reads outputs. If an input index is specified, reads all outputs from that input",
	Example: examples,
	Run:     run,
}

const examples = `# Read all notices:
cartesi-rollups-cli read outputs -a 0x000000000000000000000000000000000`

var (
	outputIndex uint64
	inputIndex  uint64
)

func init() {
	Cmd.Flags().Uint64Var(&inputIndex, "input-index", 0,
		"filter by input index")

	Cmd.Flags().Uint64Var(&outputIndex, "output-index", 0,
		"filter by output index")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Database == nil {
		panic("Database was not initialized")
	}

	application := common.HexToAddress(cmdcommon.ApplicationAddress)

	var result []byte
	if cmd.Flags().Changed("output-index") {
		if cmd.Flags().Changed("input-index") {
			fmt.Fprintf(os.Stderr, "Error: Only one of 'output-index' or 'input-index' can be used at a time.\n")
			os.Exit(1)
		}
		outputs, err := cmdcommon.Database.GetOutput(ctx, application, outputIndex)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(outputs, "", "    ")
		cobra.CheckErr(err)
	} else if cmd.Flags().Changed("input-index") {
		outputs, err := cmdcommon.Database.GetOutputsByInputIndex(ctx, application, inputIndex)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(outputs, "", "    ")
		cobra.CheckErr(err)
	} else {
		outputs, err := cmdcommon.Database.GetOutputs(ctx, application)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(outputs, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
