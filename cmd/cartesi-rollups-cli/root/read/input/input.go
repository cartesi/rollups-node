// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package input

import (
	"encoding/json"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "input",
	Short:   "Reads an input",
	Example: examples,
	Run:     run,
}

const examples = `# Read specific input from GraphQL:
cartesi-rollups-cli read input --index 5`

var (
	index           int
	graphqlEndpoint string
)

func init() {
	Cmd.Flags().IntVar(&index, "index", 0,
		"index of the input")

	cobra.CheckErr(Cmd.MarkFlagRequired("index"))

	Cmd.Flags().StringVar(&graphqlEndpoint, "graphql-endpoint", "http://localhost:10000/graphql",
		"address used to connect to graphql")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	client := graphql.NewClient(graphqlEndpoint, nil)

	resp, err := readerclient.GetInput(ctx, client, index)
	cobra.CheckErr(err)

	val, err := json.MarshalIndent(resp, "", "    ")
	cobra.CheckErr(err)

	fmt.Print(string(val))
}
