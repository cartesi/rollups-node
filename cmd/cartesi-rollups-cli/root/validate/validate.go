// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validate

import (
	"fmt"
	"os"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "validate",
	Short:   "Validates a notice",
	Example: examples,
	Run:     run,
}

const examples = `# Validates output with index 5:
cartesi-rollups-cli validate -n echo-dapp --output-index 5`

var (
	name        string
	address     string
	outputIndex uint64
	ethEndpoint string
)

func init() {
	Cmd.Flags().StringVarP(
		&name,
		"name",
		"n",
		"",
		"Application name",
	)

	Cmd.Flags().StringVarP(
		&address,
		"address",
		"a",
		"",
		"Application contract address",
	)

	Cmd.Flags().StringVarP(
		&cmdcommon.PostgresEndpoint,
		"postgres-endpoint",
		"p",
		"postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable",
		"Postgres endpoint",
	)

	Cmd.Flags().Uint64Var(&outputIndex, "output-index", 0,
		"index of the output")
	cobra.CheckErr(Cmd.MarkFlagRequired("output-index"))

	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if name == "" && address == "" {
			return fmt.Errorf("either 'name' or 'address' must be specified")
		}
		if name != "" && address != "" {
			return fmt.Errorf("only one of 'name' or 'address' can be specified")
		}
		return cmdcommon.PersistentPreRun(cmd, args)
	}

}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	if cmdcommon.Repository == nil {
		panic("Repository was not initialized")
	}

	var nameOrAddress string
	if cmd.Flags().Changed("name") {
		nameOrAddress = name
	} else if cmd.Flags().Changed("address") {
		nameOrAddress = address
	}

	output, err := cmdcommon.Repository.GetOutput(ctx, nameOrAddress, outputIndex)
	cobra.CheckErr(err)

	if output == nil {
		fmt.Fprintf(os.Stderr, "The output with index %d was not found in the database\n", outputIndex)
		os.Exit(1)
	}

	app, err := cmdcommon.Repository.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)

	if len(output.OutputHashesSiblings) == 0 {
		fmt.Fprintf(os.Stderr, "The output with index %d has no associated proof yet\n", outputIndex)
		os.Exit(0)
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint)
	cobra.CheckErr(err)

	fmt.Printf("Validating output app: %v (%v) output_index: %v\n", app.Name, app.IApplicationAddress, outputIndex)
	err = ethutil.ValidateOutput(
		ctx,
		client,
		app.IApplicationAddress,
		outputIndex,
		output.RawData,
		output.OutputHashesSiblings,
	)
	cobra.CheckErr(err)

	fmt.Println("Output validated!")
}
