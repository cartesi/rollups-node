// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package tournaments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/service"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "tournaments <application> [address]",
	Short:   "Reads tournaments",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), //nolint: mnd
	Run:     run,
	Long: `
Arguments:
	<application>     application name or address
	[address]         hex encoded

Supported Environment Variables:
  CARTESI_JSONRPC_API_URL                        JSON-RPC API URL
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read specific tournament:
cartesi-rollups-cli read tournaments echo-dapp 0x0073a8637d98649717bdc02ecb439c80aa8a10d0

# Read all tournaments:
cartesi-rollups-cli read tournaments echo-dapp

# Read all tournaments with filter:
cartesi-rollups-cli read tournaments echo-dapp --epoch-index 10 --level 10 --parent-tournament-address 0x95eac57f9d67c5e0f255d5a19eb5d3fd00cafa73 --parent-match-id-hash 0xdb99c9cdb2e2070a4e4e633c2e6874648dfe3971d14da843465b3d950df3dd19

# Read all tournaments with pagination:
cartesi-rollups-cli read tournaments echo-dapp --limit 10 --offset 10 --descending
`

var (
	epochIndex              string
	level                   string
	parentTournamentAddress string
	parentMatchIDHash       string
	limit                   uint64
	offset                  uint64
	descending              bool
)

func init() {
	Cmd.Flags().StringVar(&epochIndex, "epoch-index", "",
		"Filter tournaments by epoch index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&level, "level", "",
		"Filter tournaments by level (decimal or hex encoded)")
	Cmd.Flags().StringVar(&parentTournamentAddress, "parent-tournament-address", "",
		"Filter tournaments by parent tournament address (hex encoded)")
	Cmd.Flags().StringVar(&parentMatchIDHash, "parent-match-id-hash", "",
		"Filter tournaments by parent match ID hash (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, //nolint: mnd
		"Maximum number of tournaments to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of tournaments")
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
		var params api.GetTournamentParams
		params.Application = args[0]
		params.Address = args[1]

		result, err = readServ.GetTournament(ctx, params)
	} else {
		var params api.ListTournamentsParams
		params.Application = args[0]

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			epochIndexHex, hexErr := config.AsHexString(epochIndex)
			cobra.CheckErr(hexErr)
			params.EpochIndex = &epochIndexHex
		}

		// Add level filter if provided
		if cmd.Flags().Changed("level") {
			levelHex, hexErr := config.AsHexString(level)
			cobra.CheckErr(hexErr)
			params.Level = &levelHex
		}

		// Add parent tournament address filter if provided
		if cmd.Flags().Changed("parent-tournament-address") {
			params.ParentTournamentAddress = &parentTournamentAddress
		}

		// Add parent match ID hash filter if provided
		if cmd.Flags().Changed("parent-match-id-hash") {
			params.ParentMatchIDHash = &parentMatchIDHash
		}
		params.Limit = limit
		params.Offset = offset
		params.Descending = descending

		result, err = readServ.ListTournaments(ctx, params)
	}
	cobra.CheckErr(err)

	var out bytes.Buffer
	err = json.Indent(&out, result, "", "    ")
	cobra.CheckErr(err)
	out.WriteString("\n")

	out.WriteTo(os.Stdout)
}
