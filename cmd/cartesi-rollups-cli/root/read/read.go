// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package read

import (
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/commitments"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/epochs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/inputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/outputs"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/reports"
	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/read/tournaments"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "read",
	Short: "Read the node state from the database",
}

func init() {
	Cmd.AddCommand(epochs.Cmd)
	Cmd.AddCommand(inputs.Cmd)
	Cmd.AddCommand(outputs.Cmd)
	Cmd.AddCommand(reports.Cmd)
	Cmd.AddCommand(tournaments.Cmd)
	Cmd.AddCommand(commitments.Cmd)
}
