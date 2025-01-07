// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package send

import (
	"fmt"
	"io"
	"os"
	"strings"

	cmdcommon "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "send",
	Short:   "Send a rollups input to the Ethereum node",
	Example: examples,
	Run:     run,
}

const examples = `# Send the string "hi":
cartesi-rollups-cli send -n echo-dapp --payload "hi"

# Send the string "hi" encoded as hex:
cartesi-rollups-cli send -n echo-dapp --payload 0x6869 --hex

# Read from stdin:
echo "hi" | cartesi-rollups-cli send -n echo-dapp`

var (
	name            string
	address         string
	ethEndpoint     string
	mnemonic        string
	account         uint32
	inputBoxAddress string
	cmdPayload      string
	isHex           bool
)

func init() {
	Cmd.Flags().StringVar(&ethEndpoint, "eth-endpoint", "http://localhost:8545",
		"ethereum node JSON-RPC endpoint")

	Cmd.Flags().StringVar(&mnemonic, "mnemonic", ethutil.FoundryMnemonic,
		"mnemonic used to sign the transaction")

	Cmd.Flags().Uint32Var(&account, "account", 0,
		"account index used to sign the transaction (default: 0)")

	Cmd.Flags().StringVarP(&name, "name", "n", "",
		"Application name")

	Cmd.Flags().StringVarP(&address, "address", "a", "", "Application contract address")

	Cmd.Flags().StringVar(&cmdPayload, "payload", "",
		"input payload hex-encoded starting with 0x")

	Cmd.Flags().BoolVarP(&isHex, "hex", "x", false,
		"Force interpretation of --payload as hex.")

	Cmd.Flags().StringVar(&inputBoxAddress, "inputbox-address", "",
		"Input Box contract address")

	Cmd.Flags().StringVarP(
		&cmdcommon.PostgresEndpoint,
		"postgres-endpoint",
		"p",
		"postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable",
		"Postgres endpoint",
	)

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if name == "" && address == "" {
			return fmt.Errorf("either 'name' or 'address' must be specified")
		}
		if name != "" && address != "" {
			return fmt.Errorf("only one of 'name' or 'address' can be specified")
		}
		return cmdcommon.PersistentPreRun(cmd, args)
	}
}

func resolvePayload(cmd *cobra.Command) ([]byte, error) {
	if !cmd.Flags().Changed("payload") {
		stdinBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		return stdinBytes, nil
	}

	if isHex {
		return decodeHex(cmdPayload)
	}

	return []byte(cmdPayload), nil
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
	if cmdcommon.Repository == nil {
		panic("Repository was not initialized")
	}

	var nameOrAddress string
	if cmd.Flags().Changed("name") {
		nameOrAddress = name
	} else if cmd.Flags().Changed("address") {
		nameOrAddress = address
	}

	app, err := cmdcommon.Repository.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)

	payload, err := resolvePayload(cmd)
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint)
	cobra.CheckErr(err)

	signer, err := ethutil.NewMnemonicSigner(ctx, client, mnemonic, account)
	cobra.CheckErr(err)

	if !cmd.Flags().Changed("inputbox-address") {
		nconfig, err := repository.LoadNodeConfig[model.NodeConfigValue](ctx, cmdcommon.Repository, model.BaseConfigKey)
		cobra.CheckErr(err)
		inputBoxAddress = nconfig.Value.InputBoxAddress
	}

	appAddr := app.IApplicationAddress
	ibAddr := common.HexToAddress(inputBoxAddress)

	inputIndex, blockNumber, err := ethutil.AddInput(ctx, client, ibAddr, appAddr, signer, payload)
	cobra.CheckErr(err)

	fmt.Printf("Input sent to app at %s. Index: %d BlockNumber: %d\n", appAddr, inputIndex, blockNumber)
}
