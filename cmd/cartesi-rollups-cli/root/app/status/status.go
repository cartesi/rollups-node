// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package status

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
)

var Cmd = &cobra.Command{
	Use:     "status",
	Short:   "Display and set application status",
	Example: examples,
	Run:     run,
}

const examples = `# Get application status:
cartesi-rollups-cli app status -n echo-dapp`

var (
	name    string
	address string
	enable  bool
	disable bool
)

func init() {
	Cmd.Flags().StringVarP(
		&name,
		"name",
		"n",
		"",
		"Application name",
	)

	Cmd.Flags().StringVarP(
		&address,
		"address",
		"a",
		"",
		"Application contract address",
	)

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

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if name == "" && address == "" {
			return fmt.Errorf("either 'name' or 'address' must be specified")
		}
		if name != "" && address != "" {
			return fmt.Errorf("only one of 'name' or 'address' can be specified")
		}
		if cmd.Flags().Changed("enable") && cmd.Flags().Changed("disable") {
			return fmt.Errorf("Cannot enable and disable at the same time")
		}
		return nil
	}

}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Repository == nil {
		panic("Repository was not initialized")
	}

	var nameOrAddress string
	if cmd.Flags().Changed("name") {
		nameOrAddress = name
	} else if cmd.Flags().Changed("address") {
		nameOrAddress = address
	}

	app, err := cmdcommon.Repository.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)

	if (!cmd.Flags().Changed("enable")) && (!cmd.Flags().Changed("disable")) {
		fmt.Println(app.State)
		os.Exit(0)
	}

	if app.State == model.ApplicationState_Inoperable {
		fmt.Fprintf(os.Stderr, "Error: Cannot execute operation. Application %s is on %s state\n", app.Name, app.State)
		os.Exit(1)
	}

	dirty := false
	if cmd.Flags().Changed("enable") && app.State == model.ApplicationState_Disabled {
		app.State = model.ApplicationState_Enabled
		dirty = true
	} else if cmd.Flags().Changed("disable") && app.State == model.ApplicationState_Enabled {
		app.State = model.ApplicationState_Disabled
		dirty = true
	}

	if !dirty {
		fmt.Printf("Application %s status was already %s\n", app.Name, app.State)
		os.Exit(0)
	}

	err = cmdcommon.Repository.UpdateApplicationState(ctx, app)
	cobra.CheckErr(err)

	fmt.Printf("Application %s status updated to %s\n", app.Name, app.State)
}
