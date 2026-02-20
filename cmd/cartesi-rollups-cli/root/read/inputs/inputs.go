// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package inputs

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/inputs"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "inputs [application-name-or-address] [input-index]",
	Short:   "Reads inputs ordered by index",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all inputs:
cartesi-rollups-cli read inputs echo-dapp

# Read a specific input by index:
cartesi-rollups-cli read inputs echo-dapp 42

# Read inputs filtered byt epoch index:
cartesi-rollups-cli read inputs echo-dapp --epoch-index 0x3

# Read inputs filtered by sender address:
cartesi-rollups-cli read inputs echo-dapp --sender 0x0123456789abcdef0123456789abcdef0123456789abcdef

# Read inputs with pagination:
cartesi-rollups-cli read inputs echo-dapp --epoch-index 0x3 --limit 20 --offset 0`

var (
	epochIndex uint64
	sender     string
	limit      uint64
	offset     uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0,
		"Filter inputs by epoch index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&sender, "sender", "",
		"Filter inputs by sender address (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of inputs to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of inputs")

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

	parsedAbi, err := inputs.InputsMetaData.GetAbi()
	cobra.CheckErr(err)

	var result []byte
	if len(args) == 1 {
		// Create filter based on flags
		filter := repository.InputFilter{}

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			filter.EpochIndex = &epochIndex
		}

		// Add sender filter if provided
		if sender != "" {
			senderAddr, err := config.ToAddressFromString(sender)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid sender address: %w", err))
			}
			filter.Sender = &senderAddr
		}

		// Limit is validated in PreRunE

		// List all inputs with filters
		inputList, total, err := repo.ListInputs(ctx, nameOrAddress, filter, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Create decoded inputs
		var decodedInputs []*jsonrpc.DecodedInput
		for _, input := range inputList {
			decoded, _ := jsonrpc.DecodeInput(input, parsedAbi)
			decodedInputs = append(decodedInputs, decoded)
		}

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*jsonrpc.DecodedInput `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: decodedInputs,
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
	} else {
		// Get specific input by index
		inputIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid value for input-index: %w", err))
		}

		input, err := repo.GetInput(ctx, nameOrAddress, inputIndex)
		cobra.CheckErr(err)

		if input == nil {
			cobra.CheckErr(errors.New("not found"))
		}

		// Create decoded input
		decoded, _ := jsonrpc.DecodeInput(input, parsedAbi)

		// Format response to match JSON-RPC API
		response := struct {
			Data *jsonrpc.DecodedInput `json:"data"`
		}{
			Data: decoded,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
