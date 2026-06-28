// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package foreclose

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

var Cmd = &cobra.Command{
	Use:     "foreclose [app-name-or-address]",
	Short:   "Foreclose an application (guardian-only)",
	Example: examples,
	Args:    cobra.ExactArgs(1),
	Run:     run,
	Long: `
Calls IApplication.foreclose() on the application contract. The transaction
must be signed by the guardian wallet configured at deploy time, otherwise it
reverts with NotGuardian. The signer is the wallet configured via
CARTESI_AUTH_*; override CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX to pick a
different derived account when the guardian differs from the node's default
signer.

The [app-name-or-address] argument accepts EITHER an application name
(looked up in the local rollups-node database) OR an Ethereum address (used
directly without any DB access — useful on remote/reader hosts).

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection (only when an app name is passed)
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_AUTH_MNEMONIC, CARTESI_AUTH_PRIVATE_KEY, CARTESI_AUTH_AWS_KMS_KEY_ID  signer
  CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX            derived account index (mnemonic auth)`,
}

const examples = `# Foreclose by application name (guardian signs from CARTESI_AUTH_*):
cartesi-rollups-cli foreclose echo-dapp

# Foreclose by application address with the second derived mnemonic account as guardian:
CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX=1 cartesi-rollups-cli foreclose 0x7Ba726B1bc58b1fca5BD28fE3A752D57228891cC

# Skip the confirmation prompt:
cartesi-rollups-cli foreclose echo-dapp --yes`

var (
	skipConfirmation bool
	asJSONParam      bool
)

func init() {
	Cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip confirmation prompt")
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

	appAddr, err := util.ResolveApplicationAddress(ctx, nameOrAddress)
	cobra.CheckErr(err)

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	cobra.CheckErr(err)

	chainId, err := client.ChainID(ctx)
	cobra.CheckErr(err)

	txOpts, err := cli.GetTransactOpts(ctx, chainId)
	cobra.CheckErr(err)

	appContract, err := iapplication.NewIApplication(appAddr, client)
	cobra.CheckErr(err)

	// Surface the guardian / signer mismatch early as a hint, instead of letting
	// the on-chain revert produce an opaque "NotGuardian" error.
	guardian, err := appContract.GetGuardian(&bind.CallOpts{Context: ctx})
	cobra.CheckErr(err)
	if guardian != txOpts.From {
		fmt.Fprintf(os.Stderr,
			"warning: signer %s does not match the application guardian %s — foreclose() will revert with NotGuardian\n",
			txOpts.From, guardian)
	}

	if !skipConfirmation {
		fmt.Printf("Preparing to foreclose application %v with signer %v\n",
			appAddr, txOpts.From)

		confirmed, promptErr := cli.ConfirmPrompt("Do you want to continue?")
		cobra.CheckErr(promptErr)
		if !confirmed {
			fmt.Println("Transaction cancelled")
			os.Exit(0)
		}
	}

	tx, err := appContract.Foreclose(txOpts)
	// go-ethereum's binding returns (signedTx, sendErr) when signing
	// succeeded but the broadcast/response read failed — the tx may already
	// be in the mempool. Surface the hash on stderr so the operator can find
	// it even when CheckErr below aborts.
	if tx != nil {
		fmt.Fprintf(os.Stderr, "broadcast attempt sent — tx hash %s\n", tx.Hash().Hex())
	}
	cobra.CheckErr(cli.DecorateRevert(err, iapplication.IApplicationMetaData))
	txHash := tx.Hash()

	if asJSONParam {
		result := struct {
			TransactionHash string         `json:"transaction_hash"`
			ApplicationAddr common.Address `json:"application_address"`
		}{TransactionHash: txHash.Hex(), ApplicationAddr: appAddr}
		jsonBytes, err := json.MarshalIndent(&result, "", "  ")
		cobra.CheckErr(err)
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Foreclose tx-hash: %v\n", txHash)
	}
}
