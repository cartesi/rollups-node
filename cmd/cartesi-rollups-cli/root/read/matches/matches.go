// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package matches

import (
	"encoding/json"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "matches [application-name-or-address] [epoch-index] [tournament-address] [id-hash]",
	Short:   "Reads matches",
	Example: examples,
	Args:    cobra.RangeArgs(1, 4), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all matches:
cartesi-rollups-cli read matches echo-dapp

# Read specific match by epoch, tournament address, and hash:
cartesi-rollups-cli read matches echo-dapp 1 0x0123456789abcdef0123456789abcdef0123456789abcdef 0x1b7087e9580fb7946f37a40ced7ffeded336da790e00cc88e5f4e8e25301546a

# Read matches filtered by tournament address:
cartesi-rollups-cli read matches echo-dapp --tournament-address 0x0123456789abcdef0123456789abcdef0123456789abcdef

# Read matches with pagination:
cartesi-rollups-cli read matches echo-dapp --limit 10 --offset 20`

func init() {
	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

var (
	epochIndex        uint64
	tournamentAddress string
	limit             uint64
	offset            uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0,
		"Filter matches by epoch index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&tournamentAddress, "tournament-address", "",
		"Filter matches by tournament address (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of matches to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of matches")

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

	var result []byte
	if len(args) == 4 { // nolint: mnd
		// Get a specific match by address

		epochIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid value for epoch-index: %w", err))
		}

		// TODO: [maia] check out if the following check is necessary, because
		// such conversion is already done in the 'repo.GetMatch' function,
		// but it doesn't seem to check for invalid addresses.
		tournamentAddressHex := args[2]
		_, err = config.ToAddressFromString(tournamentAddressHex)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid tournament address: %w", err))
		}
		idHashHex := args[3]
		_, err = config.ToHashFromString(idHashHex)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid ID hash: %w", err))
		}

		match, err := repo.GetMatch(ctx, nameOrAddress, epochIndex, tournamentAddressHex, idHashHex)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data *model.Match `json:"data"`
		}{
			Data: match,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	} else {
		// Create filter based on flags
		filter := repository.MatchFilter{}

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			filter.EpochIndex = &epochIndex
		}

		// Add parent commitment address filter if provided
		if cmd.Flags().Changed("tournament-address") {
			// TODO: [maia] check out if the following check is necessary.
			_, err := config.ToAddressFromString(tournamentAddress)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid tournament address: %w", err))
			}
			filter.TournamentAddress = &tournamentAddress
		}

		// Limit is validated in PreRunE

		// List matches with filters
		matches, total, err := repo.ListMatches(ctx, nameOrAddress, filter, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*model.Match `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: matches,
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
