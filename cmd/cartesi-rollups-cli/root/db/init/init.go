// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package initialize

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the Database Schema",
	Run:   run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

var upgradeFlag bool

func init() {
	Cmd.Flags().BoolVar(&upgradeFlag, "upgrade", false, "Upgrade an existing database schema if one exists")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(_ *cobra.Command, _ []string) {
	var s *schema.Schema
	var err error

	dsnURL, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	for i := range 5 {
		s, err = schema.New(dsnURL.Raw())
		if err == nil {
			break
		}
		if i == 4 { //nolint:mnd
			fmt.Fprintf(os.Stderr, "Failed to connect to database. (%s)\n", dsnURL)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Connection to database failed. Trying again... (%s)\n", dsnURL)
		// wait before retrying
		time.Sleep(5 * time.Second) //nolint:mnd
	}
	defer s.Close()

	// Check if a schema already exists
	version, _, err := s.Version()
	if err != nil {
		if errors.Is(err, schema.ErrNoValidDatabaseSchema) {
			// No schema exists, so create it
			err = s.Upgrade()
			cobra.CheckErr(err)

			version, err = s.ValidateVersion()
			cobra.CheckErr(err)

			fmt.Printf("Database Schema was successfully initialized. Current version: %d\n", version)
			return
		}
		cobra.CheckErr(err)
	}
	if version == schema.ExpectedVersion {
		fmt.Printf("Database Schema is already at the correct version: %d\n", version)
		return
	}

	// Schema exists, check if we should upgrade
	if !upgradeFlag {
		fmt.Printf("Database Schema already exists (version: %d). Use --upgrade flag to upgrade it.\n", version)
		return
	}

	// Upgrade the existing schema
	err = s.Upgrade()
	cobra.CheckErr(err)

	version, err = s.ValidateVersion()
	cobra.CheckErr(err)

	fmt.Printf("Database Schema was successfully upgraded. Current version: %d\n", version)
}
