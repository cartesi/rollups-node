// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package outputs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/service"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "outputs <application> [output index]",
	Short:   "Reads outputs",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), //nolint: mnd
	Run:     run,
	Long: `
Arguments:
	<application>      application name or address
	[output index]     decimal or hex encoded

Supported Environment Variables:
  CARTESI_JSONRPC_API_URL                        JSON-RPC API URL
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read specific output:
cartesi-rollups-cli read outputs echo-dapp 10

# Read all outputs:
cartesi-rollups-cli read outputs echo-dapp

# Read all outputs with filter:
cartesi-rollups-cli read outputs echo-dapp --epoch-index 10 --input-index 10 --output-type 0x237a816f --voucher-address 0x95eac57f9d67c5e0f255d5a19eb5d3fd00cafa73

# Read all outputs with pagination:
cartesi-rollups-cli read outputs echo-dapp --limit 10 --offset 10 --descending
`

var (
	epochIndex     string
	inputIndex     string
	outputType     string
	voucherAddress string
	limit          uint64
	offset         uint64
	descending     bool
)

func init() {
	Cmd.Flags().StringVar(&epochIndex, "epoch-index", "",
		"Filter outputs by epoch index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&inputIndex, "input-index", "",
		"Filter outputs by input index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&outputType, "output-type", "",
		"Filter outputs by output type (first 4 bytes of raw data hex encoded)")
	Cmd.Flags().StringVar(&voucherAddress, "voucher-address", "",
		"Filter outputs by voucher address (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, //nolint: mnd
		"Maximum number of outputs to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of outputs")
	Cmd.Flags().BoolVar(&descending, "descending", false,
		"Sort results in descending order")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if limit > jsonrpc.LIST_ITEM_LIMIT {
			return fmt.Errorf("limit cannot exceed %d", jsonrpc.LIST_ITEM_LIMIT)
		}
		if limit == 0 {
			limit = jsonrpc.LIST_ITEM_LIMIT
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	useJsonrpc, err := cmd.Flags().GetBool("jsonrpc")
	cobra.CheckErr(err)

	readServ, err := service.CreateReadService(ctx, useJsonrpc)
	cobra.CheckErr(err)
	defer readServ.Close()

	var result json.RawMessage
	if len(args) >= 2 {
		var params jsonrpc.GetOutputParams
		params.Application = args[0]
		params.OutputIndex, err = config.AsHexString(args[1])
		cobra.CheckErr(err)

		result, err = readServ.GetOutput(ctx, params)
	} else {
		var params jsonrpc.ListOutputsParams
		params.Application = args[0]

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			epochIndexHex, hexErr := config.AsHexString(epochIndex)
			cobra.CheckErr(hexErr)
			params.EpochIndex = &epochIndexHex
		}

		// Add input index filter if provided
		if cmd.Flags().Changed("input-index") {
			inputIndexHex, hexErr := config.AsHexString(inputIndex)
			cobra.CheckErr(hexErr)
			params.InputIndex = &inputIndexHex
		}

		// Add output type filter if provided
		if cmd.Flags().Changed("output-type") {
			params.OutputType = &outputType
		}

		// Add voucher address filter if provided
		if cmd.Flags().Changed("voucher-address") {
			params.VoucherAddress = &voucherAddress
		}
		params.Limit = limit
		params.Offset = offset
		params.Descending = descending

		result, err = readServ.ListOutputs(ctx, params)
	}
	cobra.CheckErr(err)

	var out bytes.Buffer
	err = json.Indent(&out, result, "", "    ")
	cobra.CheckErr(err)
	out.WriteString("\n")

	out.WriteTo(os.Stdout)
}
