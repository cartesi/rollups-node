// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package status

import (
	"fmt"
	"os"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "status",
	Short:   "Display and set application status",
	Example: examples,
	Run:     run,
}

const examples = `# Get application status:
cartesi-rollups-cli app status -a 0x000000000000000000000000000000000`

var (
	enable  bool
	disable bool
)

func init() {
	Cmd.Flags().StringVarP(
		&cmdcommon.ApplicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("address"))

	Cmd.Flags().BoolVarP(
		&enable,
		"enable",
		"e",
		false,
		"Enable the application",
	)

	Cmd.Flags().BoolVarP(
		&disable,
		"disable",
		"d",
		false,
		"Disable the application",
	)

}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Database == nil {
		panic("Database was not initialized")
	}

	address := common.HexToAddress(cmdcommon.ApplicationAddress)
	application, err := cmdcommon.Database.GetApplication(ctx, address)
	cobra.CheckErr(err)

	if (!cmd.Flags().Changed("enable")) && (!cmd.Flags().Changed("disable")) {
		fmt.Println(application.Status)
		os.Exit(0)
	}

	if cmd.Flags().Changed("enable") && cmd.Flags().Changed("disable") {
		fmt.Fprintln(os.Stderr, "Cannot enable and disable at the same time")
		os.Exit(1)
	}

	status := model.ApplicationStatusRunning
	if cmd.Flags().Changed("disable") {
		status = model.ApplicationStatusNotRunning
	}

	err = cmdcommon.Database.UpdateApplicationStatus(ctx, address, status)
	cobra.CheckErr(err)

	fmt.Printf("Application status updated to %s\n", status)
}
