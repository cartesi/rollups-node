// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execute

import (
	"log/slog"
	"os"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "execute",
	Short:   "Executes a voucher",
	Example: examples,
	Run:     run,
}

const examples = `# Executes voucher 5 from input 6:
cartesi-rollups-cli execute --voucher-index 5 --input-index 6`

var (
	voucherIndex    int
	inputIndex      int
	graphqlEndpoint string
	ethEndpoint     string
	mnemonic        string
	account         uint32
	addressBookFile string
)

func init() {
	Cmd.Flags().IntVar(&voucherIndex, "voucher-index", 0,
		"index of the voucher")

	cobra.CheckErr(Cmd.MarkFlagRequired("voucher-index"))

	Cmd.Flags().IntVar(&inputIndex, "input-index", 0,
		"index of the input")

	cobra.CheckErr(Cmd.MarkFlagRequired("input-index"))

	Cmd.Flags().StringVar(&graphqlEndpoint, "graphql-endpoint", "http://localhost:10000/graphql",
		"address used to connect to graphql")

	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")

	Cmd.Flags().StringVar(&mnemonic, "mnemonic", ethutil.FoundryMnemonic,
		"mnemonic used to sign the transaction")

	Cmd.Flags().Uint32Var(&account, "account", 0,
		"account index used to sign the transaction (default: 0)")

	Cmd.Flags().StringVar(&addressBookFile, "address-book", "",
		"if set, load the address book from the given file; else, use test addresses")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	graphqlClient := graphql.NewClient(graphqlEndpoint, nil)

	resp, err := readerclient.GetVoucher(ctx, graphqlClient, voucherIndex, inputIndex)
	cobra.CheckErr(err)

	if resp.Proof == nil {
		slog.Warn("The voucher has no associated proof yet")
		os.Exit(0)
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint)
	cobra.CheckErr(err)
	slog.Info("Connected", "eth-endpoint", ethEndpoint)

	signer, err := ethutil.NewMnemonicSigner(ctx, client, mnemonic, account)
	cobra.CheckErr(err)

	var book *addresses.Book
	if addressBookFile != "" {
		book, err = addresses.GetBookFromFile(addressBookFile)
		cobra.CheckErr(err)
	} else {
		book = addresses.GetTestBook()
	}

	proof := readerclient.ConvertToContractProof(resp.Proof)

	slog.Info("Executing voucher",
		"voucher-index", voucherIndex,
		"input-index", inputIndex,
		"application-address", book.Application,
	)
	txHash, err := ethutil.ExecuteOutput(
		ctx,
		client,
		book,
		signer,
		resp.Payload,
		proof,
	)
	cobra.CheckErr(err)

	slog.Info("Voucher executed", "tx-hash", txHash)
}
