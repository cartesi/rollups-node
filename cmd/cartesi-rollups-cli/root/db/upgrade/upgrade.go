// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package upgrade

import (
	"log/slog"
	"time"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository/schema"
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

	for i := 0; i < 5; i++ {
		s, err = schema.New(common.PostgresEndpoint)
		if err == nil {
			break
		}
		slog.Warn("Connection to database failed. Trying again.", "PostgresEndpoint", common.PostgresEndpoint)
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

	slog.Info("Database Schema successfully Updated.", "version", version)
}
