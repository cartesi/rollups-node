// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package epochs

import (
	"encoding/json"
	"fmt"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "epochs",
	Short:   "Reads epochs",
	Example: examples,
	Run:     run,
}

const examples = `# Read all reports:
cartesi-rollups-cli read epochs -a 0x000000000000000000000000000000000`

var (
	epochIndex uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0,
		"index of the epoch")

}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Database == nil {
		panic("Database was not initialized")
	}

	application := common.HexToAddress(cmdcommon.ApplicationAddress)

	var result []byte
	if cmd.Flags().Changed("epoch-index") {
		reports, err := cmdcommon.Database.GetEpoch(ctx, epochIndex, application)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	} else {
		reports, err := cmdcommon.Database.GetEpochs(ctx, application)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
