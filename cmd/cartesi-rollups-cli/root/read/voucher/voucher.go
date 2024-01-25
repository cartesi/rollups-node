// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package voucher

import (
	"encoding/json"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "voucher",
	Short:   "Reads a voucher",
	Example: examples,
	Run:     run,
}

const examples = `# Read voucher 5 from input 6:
cartesi-rollups-cli read voucher --voucher-index 5 --input-index 6`

var (
	voucherIndex    int
	inputIndex      int
	graphqlEndpoint string
)

func init() {
	Cmd.Flags().IntVar(&voucherIndex, "voucher-index", 0,
		"index of the voucher")

	cobra.CheckErr(Cmd.MarkFlagRequired("voucher-index"))

	Cmd.Flags().IntVar(&inputIndex, "input-index", 0,
		"index of the input")

	cobra.CheckErr(Cmd.MarkFlagRequired("input-index"))

	Cmd.Flags().StringVar(&graphqlEndpoint, "graphql-endpoint", "http://localhost:10000/graphql",
		"address used to connect to graphql")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	client := graphql.NewClient(graphqlEndpoint, nil)

	resp, err := readerclient.GetVoucher(ctx, client, voucherIndex, inputIndex)
	cobra.CheckErr(err)

	val, err := json.MarshalIndent(resp, "", "    ")
	cobra.CheckErr(err)

	fmt.Print(string(val))
}
