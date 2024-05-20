// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package mine

import (
	"context"
	"log/slog"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "mine",
	Short:   "Mine a new block",
	Example: examples,
	Run:     run,
}

const examples = `# Mine a new block:
cartesi-rollups-cli mine`

var (
	anvilEndpoint string
)

func init() {

	Cmd.Flags().StringVar(&anvilEndpoint, "anvil-endpoint", "http://localhost:8545",
		"address of anvil endpoint to be used to send the mining request")
}

func run(cmd *cobra.Command, args []string) {

	blockNumber, err := ethutil.MineNewBlock(context.Background(), anvilEndpoint)

	cobra.CheckErr(err)

	slog.Info("Ok", "block number", blockNumber)
}
