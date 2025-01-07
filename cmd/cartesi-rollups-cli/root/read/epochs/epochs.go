// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package epochs

import (
	"encoding/json"
	"fmt"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "epochs",
	Short:   "Reads epochs",
	Example: examples,
	Run:     run,
}

const examples = `# Read all reports:
cartesi-rollups-cli read epochs -n echo-dapp`

var (
	epochIndex uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0, "index of the epoch")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Repository == nil {
		panic("Repository was not initialized")
	}

	var nameOrAddress string
	pFlags := cmd.Flags()
	if pFlags.Changed("name") {
		nameOrAddress = pFlags.Lookup("name").Value.String()
	} else if pFlags.Changed("address") {
		nameOrAddress = pFlags.Lookup("address").Value.String()
	}

	var result []byte
	if cmd.Flags().Changed("epoch-index") {
		reports, err := cmdcommon.Repository.GetEpoch(ctx, nameOrAddress, epochIndex)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	} else {
		reports, err := cmdcommon.Repository.ListEpochs(ctx, nameOrAddress, repository.EpochFilter{}, repository.Pagination{})
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
