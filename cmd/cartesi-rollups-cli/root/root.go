// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/db"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/execute"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/inspect"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/send"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/validate"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "cartesi-rollups-cli",
	Short: "Command line interface for Cartesi Rollups",
	Long: `This command line interface provides functionality to help develop and debug the
Cartesi Rollups node.`,
}

var verbose bool

func init() {
	Cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	Cmd.AddCommand(send.Cmd)
	Cmd.AddCommand(read.Cmd)
	Cmd.AddCommand(inspect.Cmd)
	Cmd.AddCommand(validate.Cmd)
	Cmd.AddCommand(execute.Cmd)
	Cmd.AddCommand(app.Cmd)
	Cmd.AddCommand(db.Cmd)
	Cmd.DisableAutoGenTag = true
}
