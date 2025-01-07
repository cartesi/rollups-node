// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package read

import (
	"fmt"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/epochs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/inputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/outputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/reports"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "read",
	Short: "Read the node state from the database",
}

var (
	name    string
	address string
)

func init() {
	Cmd.PersistentFlags().StringVarP(
		&name,
		"name",
		"n",
		"",
		"Application name",
	)

	Cmd.PersistentFlags().StringVarP(
		&address,
		"address",
		"a",
		"",
		"Application contract address",
	)

	Cmd.PersistentFlags().StringVarP(
		&common.PostgresEndpoint,
		"postgres-endpoint",
		"p",
		"postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable",
		"Postgres endpoint",
	)

	Cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if name == "" && address == "" {
			return fmt.Errorf("either 'name' or 'address' must be specified")
		}
		if name != "" && address != "" {
			return fmt.Errorf("only one of 'name' or 'address' can be specified")
		}
		return common.PersistentPreRun(cmd, args)
	}

	Cmd.AddCommand(epochs.Cmd)
	Cmd.AddCommand(inputs.Cmd)
	Cmd.AddCommand(outputs.Cmd)
	Cmd.AddCommand(reports.Cmd)
}
