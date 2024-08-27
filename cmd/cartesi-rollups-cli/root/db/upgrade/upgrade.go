// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package upgrade

import (
	"log/slog"

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
	schema, err := schema.New(common.PostgresEndpoint)
	cobra.CheckErr(err)
	defer schema.Close()

	err = schema.Upgrade()
	cobra.CheckErr(err)

	version, err := schema.ValidateVersion()
	cobra.CheckErr(err)

	slog.Info("Database Schema successfully Updated.", "version", version)
}
