// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execute

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

var Cmd = &cobra.Command{
	Use:     "execute",
	Short:   "Executes a voucher",
	Example: examples,
	Run:     run,
}

const examples = `# Executes voucher/output with index 5:
cartesi-rollups-cli execute -n echo-dapp --output-index 5`

var (
	name        string
	address     string
	outputIndex uint64
	ethEndpoint string
	mnemonic    string
	account     uint32
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

	Cmd.Flags().StringVar(&mnemonic, "mnemonic", ethutil.FoundryMnemonic,
		"mnemonic used to sign the transaction")

	Cmd.Flags().Uint32Var(&account, "account", 0,
		"account index used to sign the transaction (default: 0)")

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
		panic("Database was not initialized")
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
		fmt.Fprintf(os.Stderr, "The voucher/output with index %d was not found in the database\n", outputIndex)
		os.Exit(1)
	}

	app, err := cmdcommon.Repository.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)

	if len(output.OutputHashesSiblings) == 0 {
		fmt.Fprintf(os.Stderr, "The voucher/output with index %d has no associated proof yet\n", outputIndex)
		os.Exit(1)
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint)
	cobra.CheckErr(err)

	signer, err := ethutil.NewMnemonicSigner(ctx, client, mnemonic, account)
	cobra.CheckErr(err)

	fmt.Printf("Executing voucher app: %v (%v) output_index: %v with account: %v\n", app.Name, app.IApplicationAddress, outputIndex, signer.Account())
	txHash, err := ethutil.ExecuteOutput(
		ctx,
		client,
		app.IApplicationAddress,
		signer,
		outputIndex,
		output.RawData,
		output.OutputHashesSiblings,
	)
	cobra.CheckErr(err)

	fmt.Printf("Voucher executed tx-hash: %v\n", txHash)
}
