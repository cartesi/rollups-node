// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deps

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/deps/start"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "deps",
	Short: "Read the node state from the GraphQL API",
}

func init() {

	Cmd.AddCommand(start.Cmd)

}
