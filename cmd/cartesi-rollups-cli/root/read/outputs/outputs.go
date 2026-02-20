// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package outputs

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/outputs"
)

var Cmd = &cobra.Command{
	Use:     "outputs [application-name-or-address] [output-index]",
	Short:   "Reads outputs",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all outputs:
cartesi-rollups-cli read outputs echo-dapp

# Read specific output by index:
cartesi-rollups-cli read outputs echo-dapp 42

# Read outputs filtered by epoch index and input index:
cartesi-rollups-cli read outputs echo-dapp --epoch-index 0x3

# Read outputs filtered by output type and voucher address:
cartesi-rollups-cli read outputs echo-dapp --output-type 0x237a816f --voucher-address 0x0123456789abcdef0123456789abcdef0123456789abcdef

# Read outputs with pagination:
cartesi-rollups-cli read outputs echo-dapp --limit 20 --offset 0`

var (
	epochIndex     uint64
	inputIndex     uint64
	outputType     string
	voucherAddress string
	limit          uint64
	offset         uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0,
		"Filter outputs by epoch index (decimal or hex encoded)")
	Cmd.Flags().Uint64Var(&inputIndex, "input-index", 0,
		"Filter outputs by input index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&outputType, "output-type", "",
		"Filter outputs by output type (first 4 bytes of raw data hex encoded)")
	Cmd.Flags().StringVar(&voucherAddress, "voucher-address", "",
		"Filter outputs by voucher address (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of outputs to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of outputs")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if limit > jsonrpc.LIST_ITEM_LIMIT {
			return fmt.Errorf("limit cannot exceed %d", jsonrpc.LIST_ITEM_LIMIT)
		} else if limit == 0 {
			limit = jsonrpc.LIST_ITEM_LIMIT
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.String())
	cobra.CheckErr(err)
	defer repo.Close()

	parsedAbi, err := outputs.OutputsMetaData.GetAbi()
	cobra.CheckErr(err)

	var result []byte
	if len(args) == 2 { // nolint: mnd
		// Get a specific output by index
		outputIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid output index value: %w", err))
		}

		output, err := repo.GetOutput(ctx, nameOrAddress, outputIndex)
		cobra.CheckErr(err)
		if output == nil {
			cobra.CheckErr(errors.New("not found"))
		}

		// Create decoded output
		decoded, _ := jsonrpc.DecodeOutput(output, parsedAbi)

		// Format response to match JSON-RPC API
		response := struct {
			Data *jsonrpc.DecodedOutput `json:"data"`
		}{
			Data: decoded,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	} else {
		// Create filter based on flags
		filter := repository.OutputFilter{}

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			filter.EpochIndex = &epochIndex
		}

		// Add input index filter if provided
		if cmd.Flags().Changed("input-index") {
			filter.InputIndex = &inputIndex
		}

		// Add output type filter if provided
		if cmd.Flags().Changed("output-type") {
			outputTypeBytes, err := jsonrpc.ParseOutputType(outputType)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid output type: %w", err))
			}
			filter.OutputType = &outputTypeBytes
		}

		// Add voucher address filter if provided
		if cmd.Flags().Changed("voucher-address") {
			voucherAddr, err := config.ToAddressFromString(voucherAddress)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid voucher address: %w", err))
			}
			filter.VoucherAddress = &voucherAddr
		}

		// Limit is validated in PreRunE

		// List outputs with filters
		outputList, total, err := repo.ListOutputs(ctx, nameOrAddress, filter, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Create decoded outputs
		var decodedOutputs []*jsonrpc.DecodedOutput
		for _, output := range outputList {
			decoded, _ := jsonrpc.DecodeOutput(output, parsedAbi)
			decodedOutputs = append(decodedOutputs, decoded)
		}

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*jsonrpc.DecodedOutput `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: decodedOutputs,
			Pagination: struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			}{
				TotalCount: total,
				Limit:      limit,
				Offset:     offset,
			},
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
