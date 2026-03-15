// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package status

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var yesFlag bool

var Cmd = &cobra.Command{
	Use:     "status [app-name-or-address] [new-status]",
	Short:   "Display or set application status (enabled or disabled)",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Get application status:
cartesi-rollups-cli app status echo-dapp

# Set application status:
cartesi-rollups-cli app status echo-dapp enabled
cartesi-rollups-cli app status echo-dapp disabled

# Re-enable a FAILED application without confirmation prompt:
cartesi-rollups-cli app status echo-dapp enabled --yes`

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

	// If no new status is provided, display the current status and reason
	if len(args) == 1 {
		fmt.Printf("enabled=%v health=%s\n", app.Enabled, app.Health)
		if app.Reason != nil && *app.Reason != "" {
			fmt.Printf("Reason: %s\n", *app.Reason)
		}
		if app.DeletedAt != nil {
			fmt.Printf("deleted_at=%s\n", app.DeletedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		os.Exit(0)
	}

	// Handle status change
	newStatus := strings.ToLower(args[1])

	if app.Health == model.ApplicationHealth_Inoperable {
		fmt.Fprintf(os.Stderr,
			"Error: Cannot change state of application %s. It is INOPERABLE (irrecoverable).\n",
			app.Name)
		if app.Reason != nil {
			fmt.Fprintf(os.Stderr, "Reason: %s\n", *app.Reason)
		}
		fmt.Fprintf(os.Stderr, "Use 'app remove' to remove this application.\n")
		os.Exit(1)
	}

	switch newStatus {
	case "enabled", "enable":
		if app.Enabled {
			fmt.Printf("Application %s is already enabled\n", app.Name)
			os.Exit(0)
		}

		// Changing state of a FAILED application requires confirmation
		if app.Health == model.ApplicationHealth_Failed && !yesFlag {
			fmt.Printf("Application %q is in FAILED state.\n", app.Name)
			if app.Reason != nil {
				fmt.Printf("Reason: %s\n", *app.Reason)
			}
			fmt.Println("Re-enabling will attempt to restart processing from the last snapshot.")
			confirmed, promptErr := cli.ConfirmPrompt("Proceed?")
			if promptErr != nil {
				fmt.Fprintf(os.Stderr, "Error reading input: %v\n", promptErr)
				os.Exit(1)
			}
			if !confirmed {
				fmt.Println("Aborted.")
				os.Exit(0)
			}
		}

		// Show failure reason when changing state away from FAILED
		if app.Health == model.ApplicationHealth_Failed && app.Reason != nil && *app.Reason != "" {
			fmt.Printf("Previous failure reason: %s\n", *app.Reason)
		}

		err = repo.SetApplicationEnabled(ctx, app.ID, true)
		cobra.CheckErr(err)
		if app.Health == model.ApplicationHealth_Stopped ||
			app.Health == model.ApplicationHealth_Failed {
			err = repo.MarkApplicationRunning(ctx, app.ID)
			cobra.CheckErr(err)
		}
		fmt.Printf("Application %s enabled\n", app.Name)

	case "disabled", "disable":
		if !app.Enabled {
			fmt.Printf("Application %s is already disabled\n", app.Name)
			os.Exit(0)
		}

		// Changing state of a FAILED application requires confirmation
		if app.Health == model.ApplicationHealth_Failed && !yesFlag {
			fmt.Printf("Application %q is in FAILED state.\n", app.Name)
			if app.Reason != nil {
				fmt.Printf("Reason: %s\n", *app.Reason)
			}
			confirmed, promptErr := cli.ConfirmPrompt("Proceed?")
			if promptErr != nil {
				fmt.Fprintf(os.Stderr, "Error reading input: %v\n", promptErr)
				os.Exit(1)
			}
			if !confirmed {
				fmt.Println("Aborted.")
				os.Exit(0)
			}
		}

		err = repo.SetApplicationEnabled(ctx, app.ID, false)
		cobra.CheckErr(err)
		fmt.Printf("Application %s disabled\n", app.Name)

	default:
		fmt.Fprintf(os.Stderr,
			"Error: Invalid status %q. Valid values are 'enabled' or 'disabled'\n", newStatus)
		os.Exit(1)
	}
}
