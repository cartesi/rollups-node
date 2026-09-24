// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package refund submits refunds for deposits in a foreclosed application.
package refund

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

var Cmd = newCommand()

type options struct {
	inputFile        string
	skipConfirmation bool
	asJSON           bool
}

func newCommand() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:   "refund <app-name-or-address> <input-index>",
		Short: "Refund an unfinalized deposit from a foreclosed application",
		Args:  cobra.ExactArgs(2), //nolint:mnd // Application and input index.
		RunE:  opts.run,
		Long: `Calls IApplication.issueRefund(inputIndex, input). The guardian must have
already foreclosed the application, and consensus must not have finalized
the deposit input. Local ACCEPTED or REJECTED status does not determine
whether the input is finalized.

Any funded signer can pay the gas. The canonical refund builder sends the
funds to the original depositor recorded by the portal. The gas payer does
not have to be the depositor or guardian and cannot choose a new recipient.
No machine, output, accounts-drive, or account proof is required.

The application argument accepts a name (resolved through the local database)
or an Ethereum address (no database access). The input index is application-wide,
not epoch-relative, and accepts decimal or 0x-prefixed hexadecimal uint256 values.

--input-file must contain the complete original InputAdded.input bytes as
0x-prefixed hexadecimal text, with optional surrounding whitespace. Use the
node's data.raw_data field, not decoded_data.payload, the portal payload,
deposit transaction calldata, or JSON.

The command waits for a successful mined receipt by default. With --no-wait,
it returns after broadcast without confirmation of payment. In that case,
check the receipt, RefundIssued event, and wasRefundForInputIssued on chain.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                 Required only for an application name
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT            Ethereum HTTP endpoint
  CARTESI_AUTH_KIND                           Signer authentication method
  CARTESI_AUTH_MNEMONIC, CARTESI_AUTH_PRIVATE_KEY, CARTESI_AUTH_AWS_KMS_KEY_ID
  CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX         Derived mnemonic account index`,
		Example: `# Export the complete input through the node JSON-RPC API:
cartesi-rollups-cli read inputs echo-dapp 7 --jsonrpc | jq -er '.data.raw_data' > deposit-input.hex

# Refund after the guardian has foreclosed the application:
cartesi-rollups-cli refund echo-dapp 7 --input-file deposit-input.hex --yes --json`,
	}
	cmd.Flags().StringVar(&opts.inputFile, "input-file", "", "File containing the complete 0x-prefixed InputAdded.input hex")
	cobra.CheckErr(cmd.MarkFlagRequired("input-file"))
	cmd.Flags().BoolVarP(&opts.skipConfirmation, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&opts.asJSON, "json", false, "Print the transaction hash, application address, and input index as JSON")
	cli.AddTransactionFlags(cmd)
	origHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		for _, name := range []string{"database-connection", "blockchain-http-endpoint", "gas-limit"} {
			if flag := command.Flags().Lookup(name); flag != nil {
				flag.Hidden = false
			}
		}
		origHelp(command, args)
	})
	return cmd
}

func (o *options) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	if err != nil {
		return err
	}
	inputIndex, err := parseInputIndex(args[1])
	if err != nil {
		return err
	}
	input, err := loadInput(o.inputFile)
	if err != nil {
		return err
	}
	appAddr, err := util.ResolveApplicationAddress(ctx, nameOrAddress)
	if err != nil {
		return err
	}
	endpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return err
	}
	client, err := ethclient.DialContext(ctx, endpoint.Raw())
	if err != nil {
		return fmt.Errorf("connect to Ethereum: %w", err)
	}
	defer client.Close()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("read chain ID: %w", err)
	}
	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	if err != nil {
		return fmt.Errorf("prepare refund signer: %w", err)
	}
	appContract, err := iapplication.NewIApplication(appAddr, client)
	if err != nil {
		return fmt.Errorf("bind application: %w", err)
	}
	if !o.skipConfirmation {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "Preparing to refund input %s from application %s\n"+
			"  gas payer: %s\n"+
			"The canonical refund builder pays the original depositor; the gas payer cannot change the recipient.\n",
			inputIndex, appAddr, txOpts.From)
		if err != nil {
			return err
		}
		confirmed, err := cli.ConfirmPromptTo(cmd.ErrOrStderr(), "Do you want to continue?")
		if err != nil {
			return err
		}
		if !confirmed {
			_, err := fmt.Fprintln(cmd.ErrOrStderr(), "Transaction cancelled")
			return err
		}
	}

	tx, receipt, err := cli.Transact(ctx, cmd, client, txOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return appContract.IssueRefund(opts, inputIndex, input)
	})
	if err != nil {
		return fmt.Errorf("refund: %w", cli.DecorateRevert(err, iapplication.IApplicationMetaData))
	}
	if receipt != nil && !hasMatchingRefundEvent(appContract, receipt, appAddr, inputIndex, input) {
		return fmt.Errorf("transaction %s mined, but its receipt has no matching RefundIssued event for input %s", tx.Hash(), inputIndex)
	}
	if o.asJSON {
		result := struct {
			cli.TransactionResult
			ApplicationAddr common.Address `json:"application_address"`
			InputIndex      string         `json:"input_index"`
		}{TransactionResult: cli.NewTransactionResult(tx, receipt), ApplicationAddr: appAddr, InputIndex: inputIndex.String()}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("write refund result: %w", err)
		}
	} else {
		return cli.WriteTransactionResult(cmd, tx, receipt)
	}
	return nil
}

func hasMatchingRefundEvent(
	application *iapplication.IApplication,
	receipt *types.Receipt,
	address common.Address,
	inputIndex *big.Int,
	input []byte,
) bool {
	for _, log := range receipt.Logs {
		if log == nil || log.Address != address {
			continue
		}
		event, err := application.ParseRefundIssued(*log)
		if err == nil && event.InputIndex.Cmp(inputIndex) == 0 && bytes.Equal(event.Input, input) {
			return true
		}
	}
	return false
}

func parseInputIndex(raw string) (*big.Int, error) {
	const uint256Bits = 256
	base, digits := 10, raw
	if strings.HasPrefix(raw, "0x") {
		base, digits = 16, raw[2:]
	}
	if digits == "" || strings.HasPrefix(digits, "+") || strings.HasPrefix(digits, "-") {
		return nil, fmt.Errorf("invalid input index: expected a decimal or 0x-prefixed uint256")
	}
	index, ok := new(big.Int).SetString(digits, base)
	if !ok || index.Sign() < 0 || index.BitLen() > uint256Bits {
		return nil, fmt.Errorf("invalid input index: expected a decimal or 0x-prefixed uint256")
	}
	return index, nil
}

func loadInput(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input file: %w", err)
	}
	encoded := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(encoded, "0x") {
		return nil, fmt.Errorf("invalid input file: expected 0x-prefixed hexadecimal InputAdded.input bytes")
	}
	input, err := hexutil.Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid input file: %w", err)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("invalid input file: input bytes cannot be empty")
	}
	return input, nil
}
