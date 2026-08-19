// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package matchadvances

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
	Use:     "match_advances <application> <epoch index> <tournament address> <ID hash> [parent]",
	Short:   "Reads match advances",
	Example: examples,
	Args:    cobra.RangeArgs(4, 5), //nolint: mnd
	Run:     run,
	Long: `
Arguments:
	<application>            application name or address
	<epoch index>            decimal or hex encoded
	<tournament address>     hex encoded
	<ID hash>                hex encoded
	[parent]                 hex encoded

Supported Environment Variables:
  CARTESI_JSONRPC_API_URL                        JSON-RPC API URL
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

//nolint:lll // Long CLI examples are kept copy-pasteable.
const examples = `# Read specific match advanced:
cartesi-rollups-cli read match_advances echo-dapp 10 0x0073a8637d98649717bdc02ecb439c80aa8a10d0 0xdb99c9cdb2e2070a4e4e633c2e6874648dfe3971d14da843465b3d950df3dd19 0xdb99c9cdb2e2070a4e4e633c2e6874648dfe3971d14da843465b3d950df3dd19

# Read all match advances:
cartesi-rollups-cli read match_advances echo-dapp 10 0x0073a8637d98649717bdc02ecb439c80aa8a10d0 0xdb99c9cdb2e2070a4e4e633c2e6874648dfe3971d14da843465b3d950df3dd19

# Read all match advances with pagination:
cartesi-rollups-cli read match_advances echo-dapp 10 0x0073a8637d98649717bdc02ecb439c80aa8a10d0 0xdb99c9cdb2e2070a4e4e633c2e6874648dfe3971d14da843465b3d950df3dd19 --limit 10 --offset 10 --descending
`

var (
	limit      uint64
	offset     uint64
	descending bool
)

func init() {
	Cmd.Flags().Uint64Var(&limit, "limit", 50, //nolint: mnd
		"Maximum number of match advances to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of match advances")
	Cmd.Flags().BoolVar(&descending, "descending", false,
		"Sort results in descending order")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})

	Cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
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
	if len(args) >= 5 { //nolint:mnd // Five positional arguments select the get operation.
		var params api.GetMatchAdvanceParams
		params.Application = args[0]
		params.EpochIndex, err = config.AsHexString(args[1])
		cobra.CheckErr(err)
		params.TournamentAddress = args[2]
		params.IDHash = args[3]
		params.Parent = args[4]

		result, err = readServ.GetMatchAdvanced(ctx, params)
	} else {
		var params api.ListMatchAdvancesParams
		params.Application = args[0]
		params.EpochIndex, err = config.AsHexString(args[1])
		cobra.CheckErr(err)
		params.TournamentAddress = args[2]
		params.IDHash = args[3]
		params.Limit = limit
		params.Offset = offset
		params.Descending = descending

		result, err = readServ.ListMatchAdvances(ctx, params)
	}
	cobra.CheckErr(err)

	var out bytes.Buffer
	err = json.Indent(&out, result, "", "    ")
	cobra.CheckErr(err)
	out.WriteString("\n")

	_, err = out.WriteTo(os.Stdout)
	cobra.CheckErr(err)
}
