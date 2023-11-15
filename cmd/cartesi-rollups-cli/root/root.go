// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package root

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/send"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "cartesi-rollups-cli",
	Short: "Command line interface for Cartesi Rollups",
	Long: `This command line interface provides functionality to help develop and debug the
Cartesi Rollups node.`,
}

func init() {
	Cmd.AddCommand(send.Cmd)
}
