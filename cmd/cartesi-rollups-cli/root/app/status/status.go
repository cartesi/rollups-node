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
	Short:   "Display application status or set the enabled flag",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), //nolint:mnd
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
		repo.Close()
		os.Exit(1) //nolint:gocritic // The repository is closed explicitly before exiting.
	}

	// If no new status is provided, display the current status, operator
	// enabled flag, and reason.
	// Foreclose / drive-prove markers (zero == not observed) are surfaced
	// so operators and integration tests can detect post-foreclosure
	// progress without going through the JSON-RPC API.
	if len(args) == 1 {
		fmt.Println(app.Status)
		fmt.Printf("Enabled: %t\n", app.Enabled)
		if app.Reason != nil && *app.Reason != "" {
			fmt.Printf("Reason: %s\n", *app.Reason)
		}
		if app.ForecloseBlock != 0 {
			fmt.Printf("Foreclose block: 0x%x\n", app.ForecloseBlock)
			if app.ForecloseTransaction != nil {
				fmt.Printf("Foreclose transaction: %s\n", app.ForecloseTransaction.Hex())
			}
		}
		if app.AccountsDriveProvedBlock != 0 {
			fmt.Printf("Accounts drive proved block: 0x%x\n", app.AccountsDriveProvedBlock)
			if app.AccountsDriveMerkleRoot != nil {
				fmt.Printf("Accounts drive merkle root: %s\n", app.AccountsDriveMerkleRoot.Hex())
			}
		}
		repo.Close()
		os.Exit(0)
	}

	// Handle status change
	newStatus := strings.ToLower(args[1])

	var targetEnabled bool
	switch newStatus {
	case "enabled", "enable":
		targetEnabled = true
	case "disabled", "disable":
		targetEnabled = false
	default:
		fmt.Fprintf(os.Stderr, "Error: Invalid status %q. Valid values are 'enabled' or 'disabled'\n", newStatus)
		repo.Close()
		os.Exit(1)
	}

	if app.Enabled == targetEnabled && (app.Status != model.ApplicationStatus_Failed || !targetEnabled) {
		fmt.Printf("Application %s enabled flag is already %t\n", app.Name, app.Enabled)
		repo.Close()
		os.Exit(0)
	}

	// Re-enabling a FAILED application clears the failure status and requires
	// confirmation because processing may restart from the last snapshot.
	if app.Status == model.ApplicationStatus_Failed && targetEnabled && !yesFlag {
		fmt.Printf("Application %q has FAILED status.\n", app.Name)
		if app.Reason != nil {
			fmt.Printf("Reason: %s\n", *app.Reason)
		}
		fmt.Println("Re-enabling will attempt to restart processing from the last snapshot.")
		confirmed, err := cli.ConfirmPrompt("Proceed?")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			repo.Close()
			os.Exit(1)
		}
		if !confirmed {
			fmt.Println("Aborted.")
			repo.Close()
			os.Exit(0)
		}
	}

	// Show failure reason when changing status away from FAILED.
	if app.Status == model.ApplicationStatus_Failed && app.Reason != nil && *app.Reason != "" {
		fmt.Printf("Previous failure reason: %s\n", *app.Reason)
	}

	clearFailureStatus := targetEnabled && app.Status == model.ApplicationStatus_Failed
	if clearFailureStatus {
		err = repo.EnableApplicationAndClearFailed(ctx, app.ID)
	} else {
		err = repo.UpdateApplicationEnabled(ctx, app.ID, targetEnabled)
	}
	cobra.CheckErr(err)

	fmt.Printf("Application %s enabled flag updated to %t\n", app.Name, targetEnabled)
}
