// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use: "remove [app-name-or-address]", Aliases: []string{"rm"}, Args: cobra.ExactArgs(1),
	Short: "Remove registered applications", RunE: run,
	Long: `Remove a disabled application and its stored data.
Use --allow-abandonment to acknowledge that removal stops protection and alerts
and abandons outstanding work. Transactions already sent or in progress may still
execute. --yes skips only the prompt. The command does not inspect the blockchain
for remaining obligations. The database connection uses CARTESI_DATABASE_CONNECTION.`,
	Example: `cartesi-rollups-cli app remove echo-dapp --allow-abandonment
cartesi-rollups-cli app remove echo-dapp --allow-abandonment --yes --timeout 10m`,
}

const defaultRemovalTimeout = 10 * time.Minute

var yesFlag, allowAbandonment bool
var removalTimeout time.Duration

func init() {
	Cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip the confirmation prompt")
	Cmd.Flags().BoolVar(&allowAbandonment, "allow-abandonment", false, "Acknowledge abandoned work, protection and alerts")
	Cmd.Flags().DurationVar(&removalTimeout, "timeout", defaultRemovalTimeout, "Timeout for database access and deletion")
	origHelp := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelp(command, args)
	})
}

func checkAcknowledgment() error {
	if !allowAbandonment {
		return fmt.Errorf("use --allow-abandonment to acknowledge abandoned work, protection and alerts")
	}
	return nil
}

func run(cmd *cobra.Command, args []string) error {
	if removalTimeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if err := checkAcknowledgment(); err != nil {
		return err
	}
	name, err := config.ToApplicationNameOrAddressFromString(args[0])
	if err != nil {
		return err
	}
	dsn, err := config.GetDatabaseConnection()
	if err != nil {
		return err
	}
	// Bound startup and lookup independently of operator time at the prompt.
	ctx, cancel := context.WithTimeout(cmd.Context(), removalTimeout)
	repo, err := cli.OpenRepository(ctx, dsn)
	if err != nil {
		cancel()
		return err
	}
	defer repo.Close()
	app, err := repo.GetApplication(ctx, name)
	cancel()
	if err != nil {
		return err
	}
	if app == nil {
		return repository.ErrNotFound
	}
	if app.Enabled {
		return fmt.Errorf("application %s must be disabled before removal", app.Name)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Removal cannot be undone. The node stops protection and alerts for this application "+
		"and abandons outstanding work. Transactions already sent or in progress may still execute.")
	if !yesFlag {
		confirmed, promptErr := cli.ConfirmPrompt(fmt.Sprintf("Remove application %s (%s)?", app.Name, app.IApplicationAddress))
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Operation cancelled")
			return nil
		}
	}
	slog.Info("Application removal requested with abandonment acknowledged", "application", app.Name,
		"address", app.IApplicationAddress)
	deleteCtx, deleteDone := context.WithTimeout(cmd.Context(), removalTimeout)
	defer deleteDone()
	if err = repo.DeleteApplication(deleteCtx, app.ID); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Application %s (%s) successfully removed\n", app.Name, app.IApplicationAddress)
	return nil
}
