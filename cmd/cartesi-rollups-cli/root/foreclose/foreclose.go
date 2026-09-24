// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package foreclose

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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
	RunE:    run,
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
	cli.AddTransactionFlags(Cmd)

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		command.Flags().Lookup("blockchain-http-endpoint").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	if err != nil {
		return err
	}

	appAddr, err := util.ResolveApplicationAddress(ctx, nameOrAddress)
	if err != nil {
		return err
	}

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return err
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	if err != nil {
		return err
	}
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return err
	}

	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	if err != nil {
		return err
	}

	appContract, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return err
	}

	// Surface the guardian / signer mismatch early as a hint, instead of letting
	// the on-chain revert produce an opaque "NotGuardian" error.
	guardian, err := appContract.GetGuardian(&bind.CallOpts{Context: ctx})
	if err != nil {
		return err
	}
	if guardian != txOpts.From {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: signer %s does not match the application guardian %s — foreclose() will revert with NotGuardian\n",
			txOpts.From, guardian)
		if err != nil {
			return err
		}
	}

	if !skipConfirmation {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "Preparing to foreclose application %v with signer %v\n",
			appAddr, txOpts.From)
		if err != nil {
			return err
		}

		confirmed, promptErr := cli.ConfirmPromptTo(cmd.ErrOrStderr(), "Do you want to continue?")
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			_, err := fmt.Fprintln(cmd.ErrOrStderr(), "Transaction cancelled")
			return err
		}
	}

	tx, receipt, err := cli.Transact(ctx, cmd, client, txOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return appContract.Foreclose(opts)
	})
	if err != nil {
		return cli.DecorateRevert(err, iapplication.IApplicationMetaData)
	}

	if asJSONParam {
		result := struct {
			cli.TransactionResult
			ApplicationAddr common.Address `json:"application_address"`
		}{TransactionResult: cli.NewTransactionResult(tx, receipt), ApplicationAddr: appAddr}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	return cli.WriteTransactionResult(cmd, tx, receipt)
}
