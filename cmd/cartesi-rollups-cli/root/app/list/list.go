// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package list

import (
	"log/slog"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "list",
	Short:   "Lists all applications",
	Example: examples,
	Run:     run,
}

const examples = `# List all registered applications:
cartesi-rollups-cli app list`

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if common.Database == nil {
		panic("Database was not initialized")
	}

	applications, err := common.Database.GetAllApplications(ctx)
	cobra.CheckErr(err)
	for index, app := range applications {
		slog.Info("Application", "index", index, "app", app)
	}
}
