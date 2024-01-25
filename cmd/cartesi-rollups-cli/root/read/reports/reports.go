// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package reports

import (
	"encoding/json"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "reports",
	Short:   "Reads all reports. If an input is specified, reads all reports from that input",
	Example: examples,
	Run:     run,
}

const examples = `# Read all reports:
cartesi-rollups-cli read reports`

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

	var resp []readerclient.Report
	var err error

	if cmd.Flags().Changed("input-index") {
		resp, err = readerclient.GetInputReports(ctx, client, inputIndex)
		cobra.CheckErr(err)
	} else {
		resp, err = readerclient.GetReports(ctx, client)
		cobra.CheckErr(err)
	}

	val, err := json.MarshalIndent(resp, "", "    ")
	cobra.CheckErr(err)

	fmt.Print(string(val))
}
