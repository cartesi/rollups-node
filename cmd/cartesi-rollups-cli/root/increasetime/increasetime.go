// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package increasetime

import (
	"context"
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "increase-time",
	Short:   "Increases evm time of the current machine",
	Example: examples,
	Run:     run,
}

const examples = `# Increases evm time by one day (86400 seconds):
cartesi-rollups-cli increase-time`

const defaultTime = 86400

var (
	time          int
	anvilEndpoint string
)

func init() {
	Cmd.Flags().IntVar(&time, "time", defaultTime,
		"The amount of time to increase in the evm, in seconds")

	Cmd.Flags().StringVar(&anvilEndpoint, "anvil-endpoint", "http://localhost:8545",
		"anvil address used to send to the request")
}

func run(cmd *cobra.Command, args []string) {

	cobra.CheckErr(ethutil.AdvanceDevnetTime(context.Background(), anvilEndpoint, time))

	slog.Info("Ok")
}
