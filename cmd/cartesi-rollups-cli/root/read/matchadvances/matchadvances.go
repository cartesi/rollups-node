// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package matchadvances

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "matches_advances <application-name-or-address> <epoch-index> <tournament-address> <id-hash> [parent]",
	Short:   "Reads matches advances",
	Example: examples,
	Args:    cobra.RangeArgs(4, 5), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all matches advances:
cartesi-rollups-cli read matches_advances echo-dapp

# Read specific match advances by address:
cartesi-rollups-cli read matches_advances echo-dapp 1 0x0123456789abcdef0123456789abcdef0123456789abcdef

# Read matches advances with pagination:
cartesi-rollups-cli read matches_advances echo-dapp --limit 10 --offset 20`

func init() {
	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

var (
	limit  uint64
	offset uint64
)

func init() {
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of matches advances to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of matches advances")

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

	epochIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
	if err != nil {
		cobra.CheckErr(fmt.Errorf("invalid value for epoch-index: %w", err))
	}

	// TODO: [maia] check out if the following check is necessary, because
	// such conversion is already done in the 'repo.GetMatchAdvanced' function,
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

	var result []byte
	if len(args) == 5 { // nolint: mnd
		// Get a specific match advance by address

		parentHex := args[4]

		matchAdvanced, err := repo.GetMatchAdvanced(ctx, nameOrAddress, epochIndex, tournamentAddressHex, idHashHex, parentHex)
		cobra.CheckErr(err)
		if matchAdvanced == nil {
			cobra.CheckErr(errors.New("not found"))
		}

		// Format response to match JSON-RPC API
		response := struct {
			Data *model.MatchAdvanced `json:"data"`
		}{
			Data: matchAdvanced,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	} else {
		// Limit is validated in PreRunE

		// List matches advances with filters
		matchAdvances, total, err := repo.ListMatchAdvances(ctx, nameOrAddress, epochIndex, tournamentAddressHex, idHashHex, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*model.MatchAdvanced `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: matchAdvances,
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
