// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send

import (
	"log"

	"github.com/spf13/cobra"
)

var (
	ethEndpoint string
	mnemonic    string
	account     uint32
	payload     string
)

var Cmd = &cobra.Command{
	Use:   "send",
	Short: "Send a rollups input to the Ethereum node",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	log.Printf("sending input")
}

func init() {
	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")

	Cmd.Flags().StringVar(&mnemonic, "mnemonic",
		"test test test test test test test test test test test junk",
		"mnemonic used to sign the transaction")

	Cmd.Flags().Uint32Var(&account, "account", 0,
		"account index used to sign the transaction")

	Cmd.Flags().StringVar(&payload, "payload", "",
		"input payload hex-encoded starting with 0x")
	Cmd.MarkFlagRequired("payload")
}
