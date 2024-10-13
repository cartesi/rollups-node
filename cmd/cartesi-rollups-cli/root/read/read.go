// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package read

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/epochs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/inputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/outputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/reports"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:              "read",
	Short:            "Read the node state from the database",
	PersistentPreRun: common.Setup,
}

func init() {
	Cmd.PersistentFlags().StringVarP(
		&common.ApplicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)
	cobra.CheckErr(Cmd.MarkPersistentFlagRequired("address"))

	Cmd.PersistentFlags().StringVarP(
		&common.PostgresEndpoint,
		"postgres-endpoint",
		"p",
		"postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable",
		"Postgres endpoint",
	)

	Cmd.AddCommand(epochs.Cmd)
	Cmd.AddCommand(inputs.Cmd)
	Cmd.AddCommand(outputs.Cmd)
	Cmd.AddCommand(reports.Cmd)
}
