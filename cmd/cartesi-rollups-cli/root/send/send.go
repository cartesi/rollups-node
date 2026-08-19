// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "send [app-name-or-address] [payload]",
	Short:   "Sends a rollups input transaction to the ethereum provider",
	Example: examples,
	Args:    cobra.MinimumNArgs(1),
	Run:     run,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_CONTRACTS_INPUT_BOX_ADDRESS            Input Box contract address`,
}

const examples = `# Send the string "hi":
cartesi-rollups-cli send echo-dapp "hi"

# Send the string "hi" encoded as hex:
cartesi-rollups-cli send echo-dapp 0x6869 --hex

# Read from stdin:
echo "hi" | cartesi-rollups-cli send echo-dapp

# Skip confirmation prompt:
cartesi-rollups-cli send echo-dapp "hi" --yes`

var (
	isHex            bool
	skipConfirmation bool
	asJSONParam      bool
	asyncMode        bool
)

func init() {
	Cmd.Flags().BoolVarP(&isHex, "hex", "x", false, "Force interpretation of payload as hex.")
	Cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip confirmation prompt")
	Cmd.Flags().BoolVar(&asJSONParam, "json", false, "Print result as JSON")
	Cmd.Flags().BoolVar(&asyncMode, "async", false,
		"Send the transaction without waiting for confirmation. Prints the tx hash and returns immediately.")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		command.Flags().Lookup("blockchain-http-endpoint").Hidden = false
		command.Flags().Lookup("inputbox").Hidden = false
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

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)

	iboxAddr, err := config.GetContractsInputBoxAddress()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		repo.Close()
		os.Exit(1) //nolint:gocritic // The repository is closed explicitly before exiting.
	}

	// Check if stdin is being used for payload and --yes flag is not set
	if len(args) == 1 && !skipConfirmation && !cli.IsTerminal(os.Stdin) {
		cobra.CheckErr(fmt.Errorf("reading payload from stdin. Use --yes flag to skip confirmation when piping data"))
	}

	payload, err := resolvePayload(args)
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	cobra.CheckErr(err)

	chainID, err := client.ChainID(ctx)
	cobra.CheckErr(err)

	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	cobra.CheckErr(err)

	txOptsFactory := ethutil.NewStaticTransactOptsFactory(txOpts)

	// Ask for confirmation unless --yes flag is set
	if !skipConfirmation {
		fmt.Printf("Preparing to send input to application %v (%v) with account %v\n",
			app.Name, app.IApplicationAddress, txOpts.From)

		confirmed, promptErr := cli.ConfirmPrompt("Do you want to proceed?")
		if promptErr != nil || !confirmed {
			fmt.Println("Operation cancelled")
			return
		}
	}

	if asyncMode {
		txHash, err := ethutil.AddInputAsync(ctx, client, txOptsFactory, iboxAddr, app.IApplicationAddress, payload)
		cobra.CheckErr(cli.DecorateRevert(err, iinputbox.IInputBoxMetaData, iapplication.IApplicationMetaData))
		if asJSONParam {
			result := cli.SendResult{
				ApplicationAddress: app.IApplicationAddress.Hex(),
				TransactionHash:    txHash.Hex(),
			}
			jsonBytes, err := json.MarshalIndent(&result, "", "  ")
			cobra.CheckErr(err)
			fmt.Println(string(jsonBytes))
		} else {
			fmt.Println(txHash.Hex())
		}
		return
	}

	inputIndex, blockNumber, txHash, err := ethutil.AddInput(ctx, client, txOptsFactory, iboxAddr, app.IApplicationAddress, payload)
	cobra.CheckErr(cli.DecorateRevert(err, iinputbox.IInputBoxMetaData, iapplication.IApplicationMetaData))

	if asJSONParam {
		result := cli.SendResult{
			ApplicationAddress: app.IApplicationAddress.Hex(),
			TransactionHash:    txHash.Hex(),
			InputIndex:         fmt.Sprintf("0x%x", inputIndex),
			BlockNumber:        fmt.Sprintf("0x%x", blockNumber),
		}
		jsonBytes, err := json.MarshalIndent(&result, "", "  ")
		cobra.CheckErr(err)
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Input sent to app at %s. Index: %d BlockNumber: %d\n",
			app.IApplicationAddress, inputIndex, blockNumber)
	}
}
