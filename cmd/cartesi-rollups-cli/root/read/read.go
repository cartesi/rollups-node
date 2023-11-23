// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package read

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/input"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/inputs"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "read",
	Short: "Read the node state from the GraphQL API",
}

func init() {
	Cmd.AddCommand(input.Cmd)
	Cmd.AddCommand(inputs.Cmd)
}
