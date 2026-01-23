// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package tournaments

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
	Use:     "tournaments [application-name-or-address] [tournament-address]",
	Short:   "Reads tournaments",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all tournaments:
cartesi-rollups-cli read tournaments echo-dapp

# Read specific tournament by address:
cartesi-rollups-cli read tournaments echo-dapp 0x0123456789abcdef0123456789abcdef0123456789abcdef

# Read tournaments filtered by level:
cartesi-rollups-cli read tournaments echo-dapp --level 1

# Read tournaments with pagination:
cartesi-rollups-cli read tournaments echo-dapp --limit 10 --offset 20`

func init() {
	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

var (
	epochIndex              uint64
	level                   uint64
	parentTournamentAddress string
	parentMatchIdHash       string
	limit                   uint64
	offset                  uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0,
		"Filter tournaments by epoch index (decimal or hex encoded)")
	Cmd.Flags().Uint64Var(&level, "level", 0,
		"Filter tournaments by level (decimal or hex encoded)")
	Cmd.Flags().StringVar(&parentTournamentAddress, "parent-tournament-address", "",
		"Filter tournaments by its parent address (hex encoded)")
	Cmd.Flags().StringVar(&parentMatchIdHash, "parent-match-id-hash", "",
		"Filter tournaments by its parent match ID hash (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of tournaments to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of tournaments")

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
	if len(args) == 2 { // nolint: mnd
		// Get a specific tournament by address

		// TODO: [maia] check out if the following check is necessary, because
		// such conversion is already done in the 'repo.GetTournament' function,
		// but it doesn't seem to check for invalid addresses.
		tournamentAddressHex := args[1]
		_, err := config.ToAddressFromString(tournamentAddressHex)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid tournament address: %w", err))
		}

		tournament, err := repo.GetTournament(ctx, nameOrAddress, tournamentAddressHex)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data *model.Tournament `json:"data"`
		}{
			Data: tournament,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	} else {
		// Create filter based on flags
		filter := repository.TournamentFilter{}

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			filter.EpochIndex = &epochIndex
		}

		// Add level filter if provided
		if cmd.Flags().Changed("level") {
			filter.Level = &level
		}

		// Add parent tournament address filter if provided
		if cmd.Flags().Changed("parent-tournament-address") {
			parentAddr, err := config.ToAddressFromString(parentTournamentAddress)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid tournament address: %w", err))
			}
			filter.ParentTournamentAddress = &parentAddr
		}

		// Add parent match ID hash filter if provided
		if cmd.Flags().Changed("parent-match-id-hash") {
			matchHash, err := config.ToHashFromString(parentMatchIdHash)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("invalid parent match ID hash: %w", err))
			}
			filter.ParentMatchIDHash = &matchHash
		}

		// Limit is validated in PreRunE

		// List tournaments with filters
		tournaments, total, err := repo.ListTournaments(ctx, nameOrAddress, filter, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*model.Tournament `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: tournaments,
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
