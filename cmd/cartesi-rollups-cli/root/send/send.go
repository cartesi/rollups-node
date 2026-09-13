// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "send [app-name-or-address] [payload]",
	Short:   "Sends a rollups input transaction to the ethereum provider",
	Example: examples,
	Args:    cobra.MinimumNArgs(1),
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		if cmd.Flags().Changed("inputbox") {
			return fmt.Errorf("--inputbox is not supported by send; the application selects its InputBox")
		}
		return nil
	},
	RunE: run,
	Long: `
Send to the InputBox selected by the Application contract.
An application address does not require a database. An application name does.
The global InputBox setting does not select the input destination.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint`,
}

const examples = `# Send the string "hi":
cartesi-rollups-cli send echo-dapp "hi"

# Send the string "hi" encoded as hex:
cartesi-rollups-cli send echo-dapp 0x6869 --hex

# Read from stdin:
echo "hi" | cartesi-rollups-cli send echo-dapp --yes

# Skip confirmation prompt:
cartesi-rollups-cli send echo-dapp "hi" --yes`

var (
	isHex            bool
	skipConfirmation bool
	asJSONParam      bool
)

func init() {
	Cmd.Flags().BoolVarP(&isHex, "hex", "x", false, "Force interpretation of payload as hex.")
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

func resolvePayload(args []string) ([]byte, error) {
	// If we have exactly one argument (just the app name/address), read from stdin
	if len(args) == 1 {
		stdinBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		if isHex {
			return decodeHex(string(stdinBytes))
		}
		return stdinBytes, nil
	}
	// Otherwise, use the second argument as payload
	if isHex {
		return decodeHex(args[1])
	}
	return []byte(args[1]), nil
}

func decodeHex(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		s = "0x" + s
	}

	b, err := hexutil.Decode(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex payload %q: %w", s, err)
	}
	return b, nil
}

func run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	if err != nil {
		return err
	}

	appAddress, err := util.ResolveApplicationAddress(ctx, nameOrAddress)
	if err != nil {
		return err
	}

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return err
	}

	// Check if stdin is being used for payload and --yes flag is not set
	if len(args) == 1 && !skipConfirmation && !cli.IsTerminal(os.Stdin) {
		return fmt.Errorf("reading payload from stdin: use --yes to skip confirmation when piping data")
	}

	payload, err := resolvePayload(args)
	if err != nil {
		return err
	}

	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	if err != nil {
		return err
	}
	defer client.Close()
	application, err := iapplication.NewIApplication(appAddress, client)
	if err != nil {
		return err
	}
	inputBoxAddress, err := application.GetInputBox(&bind.CallOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("read application InputBox: %w", cli.DecorateRevert(err, iapplication.IApplicationMetaData))
	}
	if inputBoxAddress == (common.Address{}) {
		return fmt.Errorf("application %s returned the zero InputBox address", appAddress)
	}

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return err
	}

	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	if err != nil {
		return err
	}

	// Ask for confirmation unless --yes flag is set
	if !skipConfirmation {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "Preparing to send input to application %v with account %v\n",
			appAddress, txOpts.From)
		if err != nil {
			return err
		}

		confirmed, promptErr := cli.ConfirmPromptTo(cmd.ErrOrStderr(), "Do you want to proceed?")
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			_, err := fmt.Fprintln(cmd.ErrOrStderr(), "Operation cancelled")
			return err
		}
	}

	inputBox, err := iinputbox.NewIInputBox(inputBoxAddress, client)
	if err != nil {
		return err
	}
	tx, receipt, err := cli.Transact(ctx, cmd, client, txOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return inputBox.AddInput(opts, appAddress, payload)
	})
	if err != nil {
		return cli.DecorateRevert(err, iinputbox.IInputBoxMetaData, iapplication.IApplicationMetaData)
	}

	result := cli.SendResult{
		TransactionResult:  cli.NewTransactionResult(tx, receipt),
		ApplicationAddress: appAddress.Hex(),
	}
	var inputIndex *big.Int
	if receipt != nil {
		for _, log := range receipt.Logs {
			if log.Address != inputBoxAddress {
				continue
			}
			event, err := inputBox.ParseInputAdded(*log)
			if err == nil && event.AppContract == appAddress {
				inputIndex = event.Index
				result.InputIndex = hexutil.EncodeBig(event.Index)
				break
			}
		}
		if result.InputIndex == "" {
			return fmt.Errorf("transaction %s mined, but its receipt has no matching InputAdded event", tx.Hash())
		}
	}
	if asJSONParam {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if receipt != nil {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Input sent to app at %s. Index: %s BlockNumber: %s Tx-hash: %s\n",
			appAddress, inputIndex.String(), receipt.BlockNumber.String(), result.TransactionHash)
		return err
	}
	return cli.WriteTransactionResult(cmd, tx, receipt)
}
