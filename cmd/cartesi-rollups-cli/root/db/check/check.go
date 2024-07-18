// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package check

import (
	"fmt"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "check-version",
	Short: "Validate the Database Schema version",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {

	schemaManager, err := repository.NewSchemaManager(common.PostgresEndpoint)
	cobra.CheckErr(err)
	defer schemaManager.Close()

	version, err := schemaManager.ValidateSchemaVersion()
	cobra.CheckErr(err)

	fmt.Printf("Database Schema is at the correct version: %d\n", version)
}
