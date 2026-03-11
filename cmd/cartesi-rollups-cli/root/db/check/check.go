// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package check

import (
	"fmt"
	"os"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "check-version",
	Short: "Validate the Database Schema version",
	Run:   run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

func init() {
	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, args []string) {
	dsnURL, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	var s *schema.Schema
	for i := range 5 {
		s, err = schema.New(dsnURL.Raw())
		if err == nil {
			break
		}
		if i == 4 { // nolint: mnd
			fmt.Fprintf(os.Stderr, "Failed to connect to database. (%s)\n", dsnURL)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Connection to database failed. Trying again... (%s)\n", dsnURL)
		// wait before retrying
		time.Sleep(5 * time.Second) // nolint: mnd
	}
	defer s.Close()

	version, err := s.ValidateVersion()
	cobra.CheckErr(err)

	fmt.Printf("Database Schema is at the correct version: %d\n", version)
}
