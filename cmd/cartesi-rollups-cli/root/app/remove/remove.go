// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var Cmd = &cobra.Command{
	Use:     "remove [app-name-or-address]",
	Aliases: []string{"rm"},
	Short:   "Remove registered applications",
	Example: examples,
	Args:    cobra.ExactArgs(1),
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Remove application:
cartesi-rollups-cli app remove echo-dapp

# Remove application without confirmation:
cartesi-rollups-cli app remove echo-dapp --yes`

var yesFlag bool

func init() {
	Cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	if app.State == model.ApplicationState_Enabled {
		fmt.Fprintf(os.Stderr, "Error: Application %s is ENABLED. Must disable it first\n", app.Name)
		os.Exit(1)
	}

	if !yesFlag {
		confirmed, promptErr := cli.ConfirmPrompt(
			fmt.Sprintf("Are you sure you want to remove application %s (%s)?",
				app.Name, app.IApplicationAddress.String()))
		if promptErr != nil || !confirmed {
			fmt.Println("Operation cancelled")
			return
		}
	}

	err = repo.DeleteApplication(ctx, app.ID)
	cobra.CheckErr(err)

	fmt.Printf("Application %s (%s) successfully removed\n", app.Name, app.IApplicationAddress.String())
}
