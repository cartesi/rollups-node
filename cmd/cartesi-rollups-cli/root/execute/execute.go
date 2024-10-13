// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execute

import (
	"fmt"
	"os"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "execute",
	Short:   "Executes a voucher",
	Example: examples,
	Run:     run,
	PreRun:  cmdcommon.Setup,
}

const examples = `# Executes voucher 5 from input 6:
cartesi-rollups-cli execute --voucher-index 5 --input-index 6`

var (
	outputIndex uint64
	ethEndpoint string
	mnemonic    string
	account     uint32
)

func init() {
	Cmd.Flags().StringVarP(
		&cmdcommon.ApplicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("address"))

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

}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommon.Database == nil {
		panic("Database was not initialized")
	}

	application := common.HexToAddress(cmdcommon.ApplicationAddress)

	output, err := cmdcommon.Database.GetOutput(ctx, application, outputIndex)
	cobra.CheckErr(err)

	if len(output.OutputHashesSiblings) == 0 {
		fmt.Fprint(os.Stderr, "The voucher has no associated proof yet")
		os.Exit(0)
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint)
	cobra.CheckErr(err)

	signer, err := ethutil.NewMnemonicSigner(ctx, client, mnemonic, account)
	cobra.CheckErr(err)

	fmt.Printf("Executing voucher app: %v output_index: %v\n", application, outputIndex)
	txHash, err := ethutil.ExecuteOutput(
		ctx,
		client,
		application,
		signer,
		outputIndex,
		output.RawData,
		output.OutputHashesSiblings,
	)
	cobra.CheckErr(err)

	fmt.Printf("Voucher executed tx-hash: %v\n", txHash)
}
