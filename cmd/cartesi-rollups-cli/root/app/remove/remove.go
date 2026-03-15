// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var Cmd = &cobra.Command{
	Use:     "remove [app-name-or-address]",
	Aliases: []string{"rm"},
	Short:   "Soft-delete an application (use --force for hard delete)",
	Example: examples,
	Args:    cobra.ExactArgs(1),
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Soft-delete application:
cartesi-rollups-cli app remove echo-dapp

# Soft-delete without confirmation:
cartesi-rollups-cli app remove echo-dapp --yes

# Hard-delete application immediately:
cartesi-rollups-cli app remove echo-dapp --force --yes`

var (
	yesFlag   bool
	forceFlag bool
)

func init() {
	Cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")
	Cmd.Flags().BoolVar(&forceFlag, "force", false, "Hard delete immediately (skip soft delete)")

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

	if !yesFlag {
		action := "soft-delete"
		if forceFlag {
			action = "permanently delete"
		}
		confirmed, promptErr := cli.ConfirmPrompt(
			fmt.Sprintf("Are you sure you want to %s application %s (%s)?",
				action, app.Name, app.IApplicationAddress.String()))
		if promptErr != nil || !confirmed {
			fmt.Println("Operation cancelled")
			return
		}
	}

	// Disable if still enabled
	if app.Enabled {
		err = repo.SetApplicationEnabled(ctx, app.ID, false)
		cobra.CheckErr(err)
	}

	if forceFlag {
		// Hard delete immediately
		err = repo.HardDeleteApplication(ctx, app.ID)
		cobra.CheckErr(err)
		fmt.Printf("Application %s hard-deleted\n", app.Name)
	} else {
		// Soft delete
		err = repo.SoftDeleteApplication(ctx, app.ID)
		cobra.CheckErr(err)
		fmt.Printf("Application %s soft-deleted (use 'app purge' to permanently remove)\n", app.Name)
	}
}
