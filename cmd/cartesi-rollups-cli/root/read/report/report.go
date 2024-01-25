// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package report

import (
	"encoding/json"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "report",
	Short:   "Reads a report",
	Example: examples,
	Run:     run,
}

const examples = `# Read report 5 from input 6:
cartesi-rollups-cli read report --report-index 5 --input-index 6`

var (
	reportIndex     int
	inputIndex      int
	graphqlEndpoint string
)

func init() {
	Cmd.Flags().IntVar(&reportIndex, "report-index", 0,
		"index of the report")

	cobra.CheckErr(Cmd.MarkFlagRequired("report-index"))

	Cmd.Flags().IntVar(&inputIndex, "input-index", 0,
		"index of the input")

	cobra.CheckErr(Cmd.MarkFlagRequired("input-index"))

	Cmd.Flags().StringVar(&graphqlEndpoint, "graphql-endpoint", "http://localhost:10000/graphql",
		"address used to connect to graphql")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	client := graphql.NewClient(graphqlEndpoint, nil)

	resp, err := readerclient.GetReport(ctx, client, reportIndex, inputIndex)
	cobra.CheckErr(err)

	val, err := json.MarshalIndent(resp, "", "    ")
	cobra.CheckErr(err)

	fmt.Print(string(val))
}
