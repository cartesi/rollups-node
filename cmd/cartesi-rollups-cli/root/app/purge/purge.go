// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package purge

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var (
	gracePeriod string
	forceFlag   bool
)

var Cmd = &cobra.Command{
	Use:   "purge",
	Short: "Permanently remove soft-deleted applications past the grace period",
	Example: `# Purge applications soft-deleted more than 1 hour ago (default):
cartesi-rollups-cli app purge

# Purge with custom grace period:
cartesi-rollups-cli app purge --grace-period 24h

# Purge even if some services have not acknowledged (unsafe):
cartesi-rollups-cli app purge --force`,
	Run: run,
	Long: `
Permanently removes applications that were soft-deleted longer ago than the grace period.
Before deleting, verifies that all required services (advancer, claimer, prt) have
acknowledged they are no longer processing the application. Use --force to skip this check.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

func init() {
	Cmd.Flags().StringVar(&gracePeriod, "grace-period", "1h",
		"Minimum time since soft delete before hard delete")
	Cmd.Flags().BoolVar(&forceFlag, "force", false,
		"Skip drain ack check (unsafe: may destroy in-flight state)")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	grace, err := time.ParseDuration(gracePeriod)
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	cutoff := time.Now().Add(-grace)

	// List all apps (empty filter returns everything including soft-deleted)
	apps, _, err := repo.ListApplications(ctx, repository.ApplicationFilter{}, repository.Pagination{}, false)
	cobra.CheckErr(err)

	purged := 0
	for _, app := range apps {
		if app.DeletedAt == nil {
			continue
		}
		if app.DeletedAt.After(cutoff) {
			fmt.Printf("Skipping %s (deleted %s ago, grace period is %s)\n",
				app.Name, time.Since(*app.DeletedAt).Round(time.Second), grace)
			continue
		}

		// Check drain acks before hard-deleting.
		if !forceFlag {
			pending, ackErr := repo.GetPendingAcks(ctx, app.ID, repository.DrainRequiredServices)
			if ackErr != nil {
				fmt.Fprintf(os.Stderr, "Error checking acks for %s: %v\n", app.Name, ackErr)
				continue
			}
			if len(pending) > 0 {
				fmt.Fprintf(os.Stderr,
					"Skipping %s: services still draining [%s] (use --force to override)\n",
					app.Name, strings.Join(pending, ", "))
				continue
			}
		}

		err = repo.HardDeleteApplication(ctx, app.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error purging %s: %v\n", app.Name, err)
			continue
		}
		fmt.Printf("Purged %s (deleted %s ago)\n",
			app.Name, time.Since(*app.DeletedAt).Round(time.Second))
		purged++
	}

	if purged == 0 {
		fmt.Println("No applications to purge")
	} else {
		fmt.Printf("Purged %d application(s)\n", purged)
	}
}
