// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package reports

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository"
)

var Cmd = &cobra.Command{
	Use:     "reports",
	Short:   "Reads reports. If an input index is specified, reads all reports from that input",
	Example: examples,
	Run:     run,
}

const examples = `# Read all reports:
cartesi-rollups-cli read reports -n echo-dapp`

var (
	inputIndex  uint64
	reportIndex uint64
)

func init() {
	Cmd.Flags().Uint64Var(&inputIndex, "input-index", 0,
		"index of the input")

	Cmd.Flags().Uint64Var(&reportIndex, "report-index", 0,
		"index of the report")
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
	if cmd.Flags().Changed("report-index") {
		if cmd.Flags().Changed("input-index") {
			fmt.Fprintf(os.Stderr, "Error: Only one of 'output-index' or 'input-index' can be used at a time.\n")
			os.Exit(1)
		}
		reports, err := cmdcommon.Repository.GetReport(ctx, nameOrAddress, reportIndex)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	} else if cmd.Flags().Changed("input-index") {
		f := repository.ReportFilter{InputIndex: &inputIndex}
		p := repository.Pagination{}
		reports, err := cmdcommon.Repository.ListReports(ctx, nameOrAddress, f, p)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	} else {
		f := repository.ReportFilter{}
		p := repository.Pagination{}
		reports, err := cmdcommon.Repository.ListReports(ctx, nameOrAddress, f, p)
		cobra.CheckErr(err)
		result, err = json.MarshalIndent(reports, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
