// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package check

import (
	"fmt"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/repository/postgres/schema"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "check-version",
	Short: "Validate the Database Schema version",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	schema, err := schema.New(common.PostgresEndpoint)
	cobra.CheckErr(err)
	defer schema.Close()

	version, err := schema.ValidateVersion()
	cobra.CheckErr(err)

	fmt.Printf("Database Schema is at the correct version: %d", version)
}
