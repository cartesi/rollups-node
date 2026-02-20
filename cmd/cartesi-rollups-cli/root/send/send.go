// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/internal/repository/factory"
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
)

func init() {
	Cmd.Flags().BoolVarP(&isHex, "hex", "x", false, "Force interpretation of payload as hex.")
	Cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip confirmation prompt")

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

// isStdinPiped returns true if stdin is being piped (not a terminal)
func isStdinPiped() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) == 0
}

// promptForConfirmation asks the user for confirmation
func promptForConfirmation() bool {
	var response string
	fmt.Print("Do you want to proceed? [y/N]: ")
	_, err := fmt.Scanln(&response)
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
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

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.String())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	// Check if stdin is being used for payload and --yes flag is not set
	if len(args) == 1 && !skipConfirmation && isStdinPiped() {
		cobra.CheckErr(fmt.Errorf("reading payload from stdin. Use --yes flag to skip confirmation when piping data"))
	}

	payload, err := resolvePayload(args)
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint.String())
	cobra.CheckErr(err)

	chainId, err := client.ChainID(ctx)
	cobra.CheckErr(err)

	txOpts, err := auth.GetTransactOpts(ctx, chainId)
	cobra.CheckErr(err)

	// Ask for confirmation unless --yes flag is set
	if !skipConfirmation {
		fmt.Printf("Preparing to send input to application %v (%v) with account %v\n",
			app.Name, app.IApplicationAddress, txOpts.From)

		if !promptForConfirmation() {
			fmt.Println("Operation cancelled")
			return
		}
	}

	inputIndex, blockNumber, err := ethutil.AddInput(ctx, client, txOpts, iboxAddr, app.IApplicationAddress, payload)
	cobra.CheckErr(err)

	fmt.Printf("Input sent to app at %s. Index: %d BlockNumber: %d\n", app.IApplicationAddress, inputIndex, blockNumber)
}
