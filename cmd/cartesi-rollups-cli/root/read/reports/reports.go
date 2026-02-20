// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package reports

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
)

var Cmd = &cobra.Command{
	Use:     "reports [application-name-or-address] [report-index]",
	Short:   "Reads reports",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all reports:
cartesi-rollups-cli read reports echo-dapp

# Read specific report by index:
cartesi-rollups-cli read reports echo-dapp 42

# Read reports filtered by epoch index:
cartesi-rollups-cli read reports echo-dapp --epoch-index 0x3

# Read reports with pagination:
cartesi-rollups-cli read reports echo-dapp --limit 10 --offset 5`

var (
	epochIndex uint64
	inputIndex uint64
	limit      uint64
	offset     uint64
)

func init() {
	Cmd.Flags().Uint64Var(&epochIndex, "epoch-index", 0,
		"Filter reports by epoch index (hex encoded)")
	Cmd.Flags().Uint64Var(&inputIndex, "input-index", 0,
		"Filter reports by input index (hex encoded)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of reports to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of reports")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if limit > jsonrpc.LIST_ITEM_LIMIT {
			return fmt.Errorf("limit cannot exceed %d", jsonrpc.LIST_ITEM_LIMIT)
		} else if limit == 0 {
			limit = jsonrpc.LIST_ITEM_LIMIT
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.String())
	cobra.CheckErr(err)
	defer repo.Close()

	var result []byte
	if len(args) == 2 { // nolint: mnd
		// Get a specific report by index
		reportIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid report index value: %w", err))
		}

		report, err := repo.GetReport(ctx, nameOrAddress, reportIndex)
		cobra.CheckErr(err)
		if report == nil {
			cobra.CheckErr(errors.New("not found"))
		}

		// Format response to match JSON-RPC API
		response := struct {
			Data *model.Report `json:"data"`
		}{
			Data: report,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	} else {
		// Create filter based on flags
		filter := repository.ReportFilter{}

		// Add epoch index filter if provided
		if cmd.Flags().Changed("epoch-index") {
			filter.EpochIndex = &epochIndex
		}

		// Add input index filter if provided
		if cmd.Flags().Changed("input-index") {
			filter.InputIndex = &inputIndex
		}

		// Limit is validated in PreRunE

		// List reports with filters
		reports, total, err := repo.ListReports(ctx, nameOrAddress, filter, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*model.Report `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: reports,
			Pagination: struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			}{
				TotalCount: total,
				Limit:      limit,
				Offset:     offset,
			},
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
