// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send

import (
	"log/slog"
	"os"

	"github.com/cartesi/rollups-node/pkg/addresses"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "send",
	Short:   "Send a rollups input to the Ethereum node",
	Example: examples,
	Run:     run,
}

const examples = `# Send the string "hi" encoded as hex:
cartesi-rollups-cli send --payload 0x$(printf "hi" | xxd -p)`

var (
	ethEndpoint     string
	mnemonic        string
	account         uint32
	hexPayload      string
	addressBookFile string
	verbose         bool
)

func init() {
	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")

	Cmd.Flags().StringVar(&mnemonic, "mnemonic", ethutil.FoundryMnemonic,
		"mnemonic used to sign the transaction")

	Cmd.Flags().Uint32Var(&account, "account", 0,
		"account index used to sign the transaction (default: 0)")

	Cmd.Flags().StringVar(&hexPayload, "payload", "",
		"input payload hex-encoded starting with 0x")

	cobra.CheckErr(Cmd.MarkFlagRequired("payload"))

	Cmd.Flags().StringVar(&addressBookFile, "address-book", "",
		"if set, load the address book from the given file; else, use test addresses")

	Cmd.Flags().BoolVar(&verbose, "verbose", false,
		"If set, prints all debug logs")
}

func run(cmd *cobra.Command, args []string) {

	if verbose {
		opts := &tint.Options{
			Level: slog.LevelDebug,
		}
		handler := tint.NewHandler(os.Stdout, opts)
		logger := slog.New(handler)
		slog.SetDefault(logger)
	}

	payload, err := hexutil.Decode(hexPayload)
	cobra.CheckErr(err)

	ctx := cmd.Context()
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

	slog.Info("Sending input", "application-address", book.Application)
	inputIndex, err := ethutil.AddInput(ctx, client, book, signer, payload)
	cobra.CheckErr(err)

	slog.Info("Input added", "input-index", inputIndex)

	if verbose {
		inputAddedEvent, err := ethutil.GetInputFromInputBox(client, book, inputIndex)
		cobra.CheckErr(err)
		slog.Debug("Input added", "event", inputAddedEvent)
	}

}
