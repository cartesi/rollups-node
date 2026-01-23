// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package epochs

import (
	"encoding/json"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/jsonrpc"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "epochs [application-name-or-address] [epoch-index]",
	Short:   "Reads epochs",
	Example: examples,
	Args:    cobra.RangeArgs(1, 2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

const examples = `# Read all epochs:
cartesi-rollups-cli read epochs echo-dapp

# Read specific epoch by index:
cartesi-rollups-cli read epochs echo-dapp 2

# Read epochs filtered by status:
cartesi-rollups-cli read epochs echo-dapp --status OPEN

# Read epochs with pagination:
cartesi-rollups-cli read epochs echo-dapp --limit 10 --offset 20`

func init() {
	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	})
}

var (
	statusFilter string
	limit        uint64
	offset       uint64
)

func init() {
	Cmd.Flags().StringVar(&statusFilter, "status", "",
		"Filter epochs by status (OPEN, CLOSED, INPUTS_PROCESSED, CLAIM_COMPUTED, CLAIM_SUBMITTED, CLAIM_ACCEPTED, CLAIM_REJECTED)")
	Cmd.Flags().Uint64Var(&limit, "limit", 50, // nolint: mnd
		"Maximum number of epochs to return")
	Cmd.Flags().Uint64Var(&offset, "offset", 0,
		"Starting point for the list of epochs")

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
	if len(args) == 1 {
		// Create filter based on flags
		filter := repository.EpochFilter{}
		if statusFilter != "" {
			var status model.EpochStatus
			if err := status.Scan(statusFilter); err != nil {
				cobra.CheckErr(fmt.Errorf("invalid status filter: %w", err))
			}
			filter.Status = []model.EpochStatus{status}
		}

		// Limit is validated in PreRunE

		epochs, total, err := repo.ListEpochs(ctx, nameOrAddress, filter, repository.Pagination{
			Limit:  limit,
			Offset: offset,
		}, false)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data       []*model.Epoch `json:"data"`
			Pagination struct {
				TotalCount uint64 `json:"total_count"`
				Limit      uint64 `json:"limit"`
				Offset     uint64 `json:"offset"`
			} `json:"pagination"`
		}{
			Data: epochs,
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
	} else {
		epochIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
		if err != nil {
			cobra.CheckErr(fmt.Errorf("invalid value for epoch-index: %w", err))
		}
		epoch, err := repo.GetEpoch(ctx, nameOrAddress, epochIndex)
		cobra.CheckErr(err)

		// Format response to match JSON-RPC API
		response := struct {
			Data *model.Epoch `json:"data"`
		}{
			Data: epoch,
		}

		result, err = json.MarshalIndent(response, "", "    ")
		cobra.CheckErr(err)
	}

	fmt.Println(string(result))
}
