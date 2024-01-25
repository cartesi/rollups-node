// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validate

import (
	"os"

	"github.com/Khan/genqlient/graphql"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/readerclient"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "validate",
	Short:   "Validates a notice",
	Example: examples,
	Run:     run,
}

const examples = `# Validates notice 5 from input 6:
cartesi-rollups-cli validate --notice-index 5 --input-index 6`

var (
	noticeIndex     int
	inputIndex      int
	graphqlEndpoint string
	ethEndpoint     string
	addressBookFile string
)

func init() {
	Cmd.Flags().IntVar(&noticeIndex, "notice-index", 0,
		"index of the notice")

	cobra.CheckErr(Cmd.MarkFlagRequired("notice-index"))

	Cmd.Flags().IntVar(&inputIndex, "input-index", 0,
		"index of the input")

	cobra.CheckErr(Cmd.MarkFlagRequired("input-index"))

	Cmd.Flags().StringVar(&graphqlEndpoint, "graphql-endpoint", "http://localhost:10000/graphql",
		"address used to connect to graphql")

	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")

	Cmd.Flags().StringVar(&addressBookFile, "address-book", "",
		"if set, load the address book from the given file; else, use test addresses")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	graphqlClient := graphql.NewClient(graphqlEndpoint, nil)

	resp, err := readerclient.GetNotice(ctx, graphqlClient, noticeIndex, inputIndex)
	cobra.CheckErr(err)

	if resp.Proof == nil {
		config.InfoLogger.Printf("The notice has no associated proof yet.\n")
		os.Exit(0)
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint)
	cobra.CheckErr(err)
	config.InfoLogger.Printf("connected to %v\n", ethEndpoint)

	var book *addresses.Book
	if addressBookFile != "" {
		book, err = addresses.GetBookFromFile(addressBookFile)
		cobra.CheckErr(err)
	} else {
		book = addresses.GetTestBook()
	}

	proof := readerclient.ConvertToContractProof(resp.Proof)

	config.InfoLogger.Printf("validating notice %d from input %d with address %x\n",
		noticeIndex,
		inputIndex,
		book.CartesiDApp,
	)
	err = ethutil.ValidateNotice(ctx, client, book, resp.Payload, proof)
	cobra.CheckErr(err)

	config.InfoLogger.Printf("The notice is valid!\n")
}
