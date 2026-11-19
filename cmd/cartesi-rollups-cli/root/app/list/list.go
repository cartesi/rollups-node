// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package list //nolint:revive // The package implements the CLI's "list" subcommand.

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonutil"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

var Cmd = &cobra.Command{
	Use:     "list",
	Short:   "Lists all applications",
	Example: examples,
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# List all registered applications:
cartesi-rollups-cli app list`

func init() {

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := cli.OpenRepository(ctx, dsn)
	cobra.CheckErr(err)
	defer repo.Close()

	applications, _, err := repo.ListApplications(ctx, repository.ApplicationFilter{}, repository.Pagination{}, false)
	cobra.CheckErr(err)

	if applications == nil {
		applications = []*model.Application{}
	}

	result, err := json.Marshal(applications, jsonutil.Options, jsontext.WithIndentPrefix(""), jsontext.WithIndent("    "))
	cobra.CheckErr(err)

	fmt.Println(string(result))
}
