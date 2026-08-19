// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package read

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/commitments"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/epochs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/inputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/matchadvances"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/matches"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/outputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/reports"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/tournaments"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/withdrawals"
	"github.com/cartesi/rollups-node/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Cmd = &cobra.Command{
	Use:   "read",
	Short: "Read the node state from the database",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("jsonrpc") && cmd.Flags().Changed("jsonrpc-api-url") {
			if err := cmd.Flags().Set("jsonrpc", "true"); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	Cmd.PersistentFlags().String("jsonrpc-api-url", "",
		"JSON-RPC endpoint string in the URL format\n(eg.: 'https://localhost:10011/rpc')")
	cobra.CheckErr(viper.BindPFlag(config.JSONRPC_API_URL, Cmd.PersistentFlags().Lookup("jsonrpc-api-url")))

	Cmd.PersistentFlags().Bool("jsonrpc", false, "Use JSON-RPC API to retrieve data")

	Cmd.AddCommand(epochs.Cmd)
	Cmd.AddCommand(inputs.Cmd)
	Cmd.AddCommand(outputs.Cmd)
	Cmd.AddCommand(reports.Cmd)
	Cmd.AddCommand(withdrawals.Cmd)
	Cmd.AddCommand(tournaments.Cmd)
	Cmd.AddCommand(commitments.Cmd)
	Cmd.AddCommand(matches.Cmd)
	Cmd.AddCommand(matchadvances.Cmd)
}
