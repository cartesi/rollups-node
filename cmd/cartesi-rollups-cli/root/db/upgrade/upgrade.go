// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package upgrade

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Create or Upgrade the Database Schema",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	var s *schema.Schema
	var err error

	uri, err := url.Parse(common.PostgresEndpoint)
	if err == nil {
		uri.User = nil
	} else {
		fmt.Fprintln(os.Stderr, "Failed to parse PostgresEndpoint.")
		os.Exit(1)
	}
	for i := 0; i < 5; i++ {
		s, err = schema.New(common.PostgresEndpoint)
		if err == nil {
			break
		}
		fmt.Fprintf(os.Stderr, "Connection to database failed. Trying again... (%s)\n", uri.String())
		if i == 4 {
			cobra.CheckErr(err)
		}
		time.Sleep(5 * time.Second) // wait before retrying
	}
	defer s.Close()

	err = s.Upgrade()
	cobra.CheckErr(err)

	version, err := s.ValidateVersion()
	cobra.CheckErr(err)

	fmt.Printf("Database Schema was successfully upgraded. Current version: %d\n", version)
}
