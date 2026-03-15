// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package purge

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var gracePeriod string

var Cmd = &cobra.Command{
	Use:   "purge",
	Short: "Permanently remove soft-deleted applications past the grace period",
	Example: `# Purge applications soft-deleted more than 1 hour ago (default):
cartesi-rollups-cli app purge

# Purge with custom grace period:
cartesi-rollups-cli app purge --grace-period 24h`,
	Run: run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

func init() {
	Cmd.Flags().StringVar(&gracePeriod, "grace-period", "1h",
		"Minimum time since soft delete before hard delete")

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
