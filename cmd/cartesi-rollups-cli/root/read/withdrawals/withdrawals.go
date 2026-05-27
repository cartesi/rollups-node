// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package withdrawals

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
	Use:     "withdrawals <application> [account index]",
	Aliases: []string{"withdraws"},
	Short:   "Reads post-foreclosure withdrawal events",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), //nolint:mnd
	Run:     run,
	Long: `
Arguments:
	<application>       application name or address
	[account index]     decimal or hex encoded

Supported Environment Variables:
  CARTESI_JSONRPC_API_URL                        JSON-RPC API URL
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read specific withdrawal by account index:
cartesi-rollups-cli read withdrawals echo-dapp 10

# Read all withdrawals:
cartesi-rollups-cli read withdrawals echo-dapp

# Read all withdrawals with filter:
cartesi-rollups-cli read withdrawals echo-dapp --account-index 10

# Read all withdrawals with pagination:
cartesi-rollups-cli read withdrawals echo-dapp --limit 10 --offset 10 --descending
`

var (
	accountIndex string
	limit        uint64
	offset       uint64
	descending   bool
)

func init() {
	Cmd.Flags().StringVar(&accountIndex, "account-index", "",
		"Filter withdrawals by account index (decimal or hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, //nolint:mnd
		"Maximum number of withdrawals to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of withdrawals")
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
		var params api.GetWithdrawalParams
		params.Application = args[0]
		params.AccountIndex, err = config.AsHexString(args[1])
		cobra.CheckErr(err)

		result, err = readServ.GetWithdrawal(ctx, params)
	} else {
		var params api.ListWithdrawalsParams
		params.Application = args[0]

		if cmd.Flags().Changed("account-index") {
			accountIndexHex, hexErr := config.AsHexString(accountIndex)
			cobra.CheckErr(hexErr)
			params.AccountIndex = &accountIndexHex
		}
		params.Limit = limit
		params.Offset = offset
		params.Descending = descending

		result, err = readServ.ListWithdrawals(ctx, params)
	}
	cobra.CheckErr(err)

	var out bytes.Buffer
	err = json.Indent(&out, result, "", "    ")
	cobra.CheckErr(err)
	out.WriteString("\n")

	out.WriteTo(os.Stdout)
}
