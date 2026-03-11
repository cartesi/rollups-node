// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validate

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "validate [app-name-or-address] [output-index]",
	Short:   "Validates a notice",
	Example: examples,
	Args:    cobra.ExactArgs(2), // nolint: mnd
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint`,
}

const examples = `# Validates output with index 5:
cartesi-rollups-cli validate echo-dapp 5

# Validates output with index 3 using application address:
cartesi-rollups-cli validate 0x1234567890123456789012345678901234567890 3`

var asJSONParam bool

func init() {
	Cmd.Flags().BoolVar(&asJSONParam, "json", false, "Print result as JSON")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		command.Flags().Lookup("blockchain-http-endpoint").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	outputIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	output, err := repo.GetOutput(ctx, nameOrAddress, outputIndex)
	cobra.CheckErr(err)

	if output == nil {
		fmt.Fprintf(os.Stderr, "The output with index %d was not found in the database\n", outputIndex)
		os.Exit(1)
	}

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)

	if len(output.OutputHashesSiblings) == 0 {
		fmt.Fprintf(os.Stderr, "The output with index %d has no associated proof yet\n", outputIndex)
		os.Exit(0)
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	cobra.CheckErr(err)

	if !asJSONParam {
		fmt.Printf("Validating output app: %v (%v) output_index: %v\n",
			app.Name, app.IApplicationAddress, outputIndex)
	}
	err = ethutil.ValidateOutput(
		ctx,
		client,
		app.IApplicationAddress,
		outputIndex,
		output.RawData,
		output.OutputHashesSiblings,
	)
	cobra.CheckErr(err)

	if asJSONParam {
		result := cli.ValidateResult{Valid: true}
		jsonBytes, err := json.MarshalIndent(&result, "", "  ")
		cobra.CheckErr(err)
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Println("Output validated!")
	}
}
