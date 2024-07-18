// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package upgrade

import (
	"fmt"
	"log/slog"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Create or Upgrade the Database Schema",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {

	schemaManager, err := repository.NewSchemaManager(common.PostgresEndpoint)
	cobra.CheckErr(err)
	defer schemaManager.Close()

	err = schemaManager.Upgrade()
	cobra.CheckErr(err)

	version, err := schemaManager.GetVersion()
	cobra.CheckErr(err)

	if repository.EXPECTED_VERSION != version {
		slog.Warn("Current version is different to expected one")
	}

	fmt.Printf("Database Schema successfully Updated. Current version is %d\n", version)

}
