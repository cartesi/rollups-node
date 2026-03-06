// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package matches

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
	Use:     "matches <application> [epoch index] [tournament address] [ID hash]",
	Short:   "Reads matches",
	Example: examples,
	Args:    cobra.RangeArgs(1, 4), //nolint: mnd
	Run:     run,
	Long: `
Arguments:
	<application>            application name or address
	[epoch index]            decimal or hex encoded
	[tournament address]     hex encoded
	[ID hash]                hex encoded

Supported Environment Variables:
  CARTESI_JSONRPC_API_URL                        JSON-RPC API URL
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read specific match:
cartesi-rollups-cli read matches echo-dapp 10 0x0073a8637d98649717bdc02ecb439c80aa8a10d0 0xdb99c9cdb2e2070a4e4e633c2e6874648dfe3971d14da843465b3d950df3dd19

# Read all matches:
cartesi-rollups-cli read matches echo-dapp

# Read all matches with filter:
cartesi-rollups-cli read matches echo-dapp --epoch-index 10 --tournament-address 0x0073a8637d98649717bdc02ecb439c80aa8a10d0

# Read all matches with pagination:
cartesi-rollups-cli read matches echo-dapp --limit 10 --offset 10 --descending
`

var (
	epochIndex        string
	tournamentAddress string
	limit             uint64
	offset            uint64
	descending        bool
)

func init() {
	Cmd.Flags().StringVar(&epochIndex, "epoch-index", "",
		"Filter matches by epoch index (decimal or hex encoded)")
	Cmd.Flags().StringVar(&tournamentAddress, "tournament-address", "",
		"Filter matches by tournament address (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, //nolint: mnd
		"Maximum number of matches to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of matches")
	Cmd.Flags().BoolVar(&descending, "descending", false,
		"Sort results in descending order")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 && len(args) < 4 { //nolint: mnd
			return fmt.Errorf(
				"expected 1 argument (list) or 4 arguments (get), got %d", len(args))
		}
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
	if len(args) >= 4 {
		var params api.GetMatchParams
		params.Application = args[0]
		params.EpochIndex, err = config.AsHexString(args[1])
		cobra.CheckErr(err)
		params.TournamentAddress = args[2]
		params.IDHash = args[3]

		result, err = readServ.GetMatch(ctx, params)
	} else {
		var params api.ListMatchesParams
		params.Application = args[0]

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			epochIndexHex, hexErr := config.AsHexString(epochIndex)
			cobra.CheckErr(hexErr)
			params.EpochIndex = &epochIndexHex
		}

		// Add tournament address filter if provided
		if cmd.Flags().Changed("tournament-address") {
			params.TournamentAddress = &tournamentAddress
		}
		params.Limit = limit
		params.Offset = offset
		params.Descending = descending

		result, err = readServ.ListMatches(ctx, params)
	}
	cobra.CheckErr(err)

	var out bytes.Buffer
	err = json.Indent(&out, result, "", "    ")
	cobra.CheckErr(err)
	out.WriteString("\n")

	out.WriteTo(os.Stdout)
}
