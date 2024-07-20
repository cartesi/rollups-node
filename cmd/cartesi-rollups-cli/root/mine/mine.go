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
	Use:   "mine",
	Short: "Mine blocks",
	Run:   run,
	Example: `# Mine 10 blocks with a 5-second interval between them:
cartesi-rollups-cli mine --number-of-blocks 10 --block-interval 5`,
}

var (
	ethEndpoint   string
	numBlocks     int
	blockInterval int
)

func init() {
	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")
	Cmd.Flags().IntVar(&numBlocks, "number-of-blocks", 1,
		"number of blocks to mine")
	Cmd.Flags().IntVar(&blockInterval, "block-interval", 1,
		"interval, in seconds, between the timestamps of each block")
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Debug("mining blocks",
		"numBlocks", numBlocks,
		"blockInterval", blockInterval)
	blockNumber, err := ethutil.MineBlocks(ctx,
		ethEndpoint,
		uint64(numBlocks),
		uint64(blockInterval))
	cobra.CheckErr(err)

	slog.Info("done", "last block number", blockNumber)
}
