// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package vouchers

import (
	"encoding/json"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "vouchers",
	Short:   "Reads all vouchers. If an input is specified, reads all vouchers from that input",
	Example: examples,
	Run:     run,
}

const examples = `# Read all vouchers:
cartesi-rollups-cli read vouchers`

var (
	inputIndex      int
	graphqlEndpoint string
)

func init() {
	Cmd.Flags().IntVar(&inputIndex, "input-index", -1,
		"index of the input")

	Cmd.Flags().StringVar(&graphqlEndpoint, "graphql-endpoint", "http://localhost:10000/graphql",
		"address used to connect to graphql")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	client := graphql.NewClient(graphqlEndpoint, nil)

	var resp []readerclient.Voucher
	var err error

	if cmd.Flags().Changed("input-index") {
		resp, err = readerclient.GetInputVouchers(ctx, client, inputIndex)
		cobra.CheckErr(err)
	} else {
		resp, err = readerclient.GetVouchers(ctx, client)
		cobra.CheckErr(err)
	}

	val, err := json.MarshalIndent(resp, "", "    ")
	cobra.CheckErr(err)

	fmt.Print(string(val))
}
