// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var Cmd = &cobra.Command{
	Use:     "remove [app-name-or-address]",
	Aliases: []string{"rm"},
	Short:   "Soft-delete an application (use --force for hard delete)",
	Example: examples,
	Args:    cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return validateRemoveOptions()
	},
	Run: run,
	Long: `
Soft-deletes an application by setting deleted_at. The application is first disabled,
triggering the drain protocol so services can finish in-flight work.

Use --force to hard-delete immediately. This skips the drain protocol and may destroy
in-flight state (pending claims, active tournaments, running machine advances).
Use --wait to block until all required drain acknowledgments are present.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Soft-delete application:
cartesi-rollups-cli app remove echo-dapp

# Soft-delete without confirmation:
cartesi-rollups-cli app remove echo-dapp --yes

# Soft-delete and wait for drain completion:
cartesi-rollups-cli app remove echo-dapp --wait --timeout 5m --poll-interval 1s

# Hard-delete application immediately (unsafe):
cartesi-rollups-cli app remove echo-dapp --force --yes`

var (
	yesFlag             bool
	forceFlag           bool
	waitFlag            bool
	acknowledgeBondLoss bool
	waitTimeout         time.Duration
	waitPollInterval    time.Duration
)

func init() {
	Cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")
	Cmd.Flags().BoolVar(&forceFlag, "force", false,
		"Hard delete immediately (skips drain protocol, may destroy in-flight state)")
	Cmd.Flags().BoolVar(&waitFlag, "wait", false,
		"Wait until all required drain acknowledgments are present")
	Cmd.Flags().DurationVar(&waitTimeout, "timeout", 5*time.Minute,
		"Maximum time to wait for drain acknowledgments when --wait is set")
	Cmd.Flags().DurationVar(&waitPollInterval, "poll-interval", time.Second,
		"Polling interval for drain acknowledgment checks when --wait is set")
	Cmd.Flags().BoolVar(&acknowledgeBondLoss, "acknowledge-bond-loss", false,
		"Acknowledge that removing a PRT app with active tournaments forfeits bonded ETH")

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

	hasActiveTournaments := false
	if app.IsDaveConsensus() {
		hasActive, tournErr := repo.HasActiveTournaments(ctx, app.ID)
		cobra.CheckErr(tournErr)
		hasActiveTournaments = hasActive
		if requiresBondLossAcknowledgement(app, forceFlag, hasActiveTournaments) && !acknowledgeBondLoss {
			fmt.Fprintf(os.Stderr,
				"Error: application %s has active PRT tournaments with bonded ETH.\n"+
					"Removing will forfeit bonded funds.\n"+
					"Use --acknowledge-bond-loss to proceed.\n", app.Name)
			os.Exit(1)
		}
	}

	if forceFlag {
		// Warn about pending drain acks and bond loss.
		if hasActiveTournaments {
			fmt.Fprintf(os.Stderr,
				"WARNING: application %s has active PRT tournaments with bonded ETH.\n"+
					"Hard-deleting will forfeit bonded funds.\n", app.Name)
		}
		required, reqErr := repository.DrainServicesForConsensus(app.ConsensusType)
		cobra.CheckErr(reqErr)
		pending, ackErr := repo.GetPendingAcks(ctx, app.ID, required)
		if ackErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not check drain status: %v\n", ackErr)
		} else if len(pending) > 0 {
			fmt.Fprintf(os.Stderr,
				"WARNING: services [%s] have not acknowledged drain.\n"+
					"Hard-deleting may destroy in-flight state "+
					"(pending claims, active tournaments, running advances).\n",
				strings.Join(pending, ", "))
			if !yesFlag {
				confirmed, promptErr := cli.ConfirmPrompt("Proceed with hard delete?")
				if promptErr != nil || !confirmed {
					fmt.Println("Operation cancelled")
					return
				}
			}
		}

		// Soft-delete first (sets enabled=false, deleted_at), then hard-delete.
		// HardDeleteApplication requires deleted_at IS NOT NULL as a safety guard.
		if app.DeletedAt == nil {
			err = repo.SoftDeleteApplication(ctx, app.ID)
			cobra.CheckErr(err)
		}
		err = repo.HardDeleteApplication(ctx, app.ID)
		cobra.CheckErr(err)
		fmt.Printf("Application %s hard-deleted\n", app.Name)
	} else {
		// Soft delete
		if app.DeletedAt != nil {
			fmt.Printf("Application %s is already soft-deleted\n", app.Name)
			if waitFlag {
				cobra.CheckErr(waitForDrainAcks(
					ctx, repo, app, waitTimeout, waitPollInterval, cmd.OutOrStdout()))
				fmt.Printf(
					"Application %s drain complete (use 'app purge' to permanently remove)\n",
					app.Name,
				)
				return
			}
			fmt.Printf("Use 'app purge' to permanently remove it\n")
			return
		}
		err = repo.SoftDeleteApplication(ctx, app.ID)
		cobra.CheckErr(err)
		fmt.Printf("Application %s soft-deleted (use 'app purge' to permanently remove)\n", app.Name)
		if waitFlag {
			cobra.CheckErr(waitForDrainAcks(
				ctx, repo, app, waitTimeout, waitPollInterval, cmd.OutOrStdout()))
			fmt.Printf(
				"Application %s drain complete (use 'app purge' to permanently remove)\n",
				app.Name,
			)
		}
	}
}

func requiresBondLossAcknowledgement(
	app *model.Application,
	forceDelete bool,
	hasActiveTournaments bool,
) bool {
	if app == nil || !app.IsDaveConsensus() || !hasActiveTournaments {
		return false
	}
	return forceDelete || app.DeletedAt == nil
}

type ackRepository interface {
	GetPendingAcks(
		ctx context.Context,
		appID int64,
		requiredServices []string,
	) ([]string, error)
}

func validateRemoveOptions() error {
	if forceFlag && waitFlag {
		return errors.New("--wait cannot be used with --force")
	}
	if waitTimeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}
	if waitPollInterval <= 0 {
		return errors.New("--poll-interval must be greater than 0")
	}
	return nil
}

func waitForDrainAcks(
	ctx context.Context,
	repo ackRepository,
	app *model.Application,
	timeout time.Duration,
	pollInterval time.Duration,
	out io.Writer,
) error {
	required, err := repository.DrainServicesForConsensus(app.ConsensusType)
	if err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var announced bool
	var lastPending []string

	for {
		pending, err := repo.GetPendingAcks(waitCtx, app.ID, required)
		if err != nil {
			return fmt.Errorf("check pending acks: %w", err)
		}
		if len(pending) == 0 {
			return nil
		}
		lastPending = pending
		if !announced {
			fmt.Fprintf(out,
				"Waiting for drain acknowledgments from [%s] for application %s...\n",
				strings.Join(pending, ", "), app.Name)
			announced = true
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf(
					"timed out waiting for drain acknowledgments from [%s]",
					strings.Join(lastPending, ", "),
				)
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}
