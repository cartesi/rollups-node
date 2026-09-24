// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deposit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20errors"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20metadata"
	"github.com/cartesi/rollups-node/pkg/contracts/ierc20portal"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "deposit",
	Short: "Deposit assets into an application through a portal",
}

var erc20Cmd = &cobra.Command{
	Use:     "erc20 [app-name-or-address]",
	Short:   "Deposit ERC-20 tokens through the Erc20Portal",
	Example: erc20Examples,
	Args:    cobra.ExactArgs(1),
	RunE:    runERC20,
	PreRunE: validateERC20Flags,
	Long: `
Calls Erc20Portal.depositErc20Tokens(token, app, amount, execData).

The command does not approve token spending unless --approve is supplied.
Without --approve, the signer must already have enough allowance for the
portal.

--approve cannot be combined with --no-wait. The approval must succeed
before the deposit transaction is prepared. Submit approval separately
if the deposit must return without waiting for mining.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection (only when an app name is passed)
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_AUTH_MNEMONIC, CARTESI_AUTH_PRIVATE_KEY, CARTESI_AUTH_AWS_KMS_KEY_ID  signer
  CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX            derived account index (mnemonic auth)`,
}

const erc20Examples = `# Deposit 100 units of a token through the Erc20Portal:
cartesi-rollups-cli deposit erc20 echo-dapp \
  --portal 0x3332DE61a8BB9aC84893b2f552Fe81C9a6dC5419 \
  --token 0x7a051EDffC0884cd88d4a377F4C87BE074CF6c81 \
  --amount 100

# Approve the portal first, then deposit:
cartesi-rollups-cli deposit erc20 echo-dapp --portal 0x... --token 0x... --amount 100 --approve --yes`

var (
	portalParam      string
	tokenParam       string
	amountParam      string
	execDataParam    string
	approveParam     bool
	skipConfirmation bool
	asJSONParam      bool
)

func init() {
	Cmd.AddCommand(erc20Cmd)

	erc20Cmd.Flags().StringVar(&portalParam, "portal", "", "Erc20Portal contract address")
	erc20Cmd.Flags().StringVar(&tokenParam, "token", "", "ERC-20 token contract address")
	erc20Cmd.Flags().StringVar(&amountParam, "amount", "", "Token amount to deposit (decimal or 0x-prefixed)")
	erc20Cmd.Flags().StringVar(&execDataParam, "exec-data", "0x", "Extra execution-layer data")
	erc20Cmd.Flags().BoolVar(&approveParam, "approve", false, "Approve the portal for --amount before depositing")
	erc20Cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip confirmation prompt")
	erc20Cmd.Flags().BoolVar(&asJSONParam, "json", false, "Print result as JSON")
	cli.AddTransactionFlags(erc20Cmd)
	cobra.CheckErr(erc20Cmd.MarkFlagRequired("portal"))
	cobra.CheckErr(erc20Cmd.MarkFlagRequired("token"))
	cobra.CheckErr(erc20Cmd.MarkFlagRequired("amount"))

	origHelpFunc := erc20Cmd.HelpFunc()
	erc20Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		command.Flags().Lookup("blockchain-http-endpoint").Hidden = false
		origHelpFunc(command, strings)
	})
}

func validateERC20Flags(cmd *cobra.Command, _ []string) error {
	approve, err := cmd.Flags().GetBool("approve")
	if err != nil {
		return err
	}
	noWait, err := cmd.Flags().GetBool("no-wait")
	if err != nil {
		return err
	}
	if approve && noWait {
		return fmt.Errorf("--approve cannot be combined with --no-wait: the deposit requires a successful approval")
	}
	return nil
}

func runERC20(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	appAddr, err := util.ResolveApplicationAddress(ctx, args[0])
	if err != nil {
		return err
	}
	portalAddr, err := parseAddress("portal", portalParam)
	if err != nil {
		return err
	}
	tokenAddr, err := parseAddress("token", tokenParam)
	if err != nil {
		return err
	}
	amount, err := parseAmount(amountParam)
	if err != nil {
		return err
	}
	execData, err := hexutil.Decode(execDataParam)
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
	txOptsFactory, err := auth.GetTransactOptsFactory(ctx, chainID)
	if err != nil {
		return err
	}

	if !skipConfirmation {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "Preparing ERC-20 deposit\n"+
			"  signer:      %s\n"+
			"  application: %s\n"+
			"  portal:      %s\n"+
			"  token:       %s\n"+
			"  amount:      %s\n"+
			"  approve:     %t\n",
			txOptsFactory.From(), appAddr, portalAddr, tokenAddr, amount.String(), approveParam)
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

	var approveHash *common.Hash
	if approveParam {
		token, err := ierc20metadata.NewIERC20Metadata(tokenAddr, client)
		if err != nil {
			return err
		}
		approveOpts, err := cli.GetTransactOptsFromFactory(ctx, txOptsFactory)
		if err != nil {
			return err
		}
		tx, receipt, err := cli.Transact(ctx, cmd, client, approveOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return token.Approve(opts, portalAddr, amount)
		})
		if err != nil {
			return cli.DecorateRevert(err, ierc20metadata.IERC20MetadataMetaData, ierc20errors.IERC20ErrorsMetaData)
		}
		approved := false
		for _, log := range receipt.Logs {
			if log == nil || log.Address != tokenAddr || len(log.Data) != common.HashLength {
				continue
			}
			event, err := token.ParseApproval(*log)
			if err == nil && event.Owner == approveOpts.From && event.Spender == portalAddr && event.Value.Cmp(amount) == 0 {
				approved = true
				break
			}
		}
		if !approved {
			return fmt.Errorf("transaction %s mined, but its receipt has no matching Approval event", tx.Hash())
		}
		hash := tx.Hash()
		approveHash = &hash
	}

	portal, err := ierc20portal.NewIErc20Portal(portalAddr, client)
	if err != nil {
		return err
	}
	depositOpts, err := cli.GetTransactOptsFromFactory(ctx, txOptsFactory)
	if err != nil {
		return err
	}
	tx, receipt, err := cli.Transact(ctx, cmd, client, depositOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return portal.DepositErc20Tokens(opts, tokenAddr, appAddr, amount, execData)
	})
	// The revert can come from three layers: the portal itself
	// (Erc20TransferFailed or a balance-delta error), the token's transferFrom
	// (ERC-6093 errors such as ERC20InsufficientBalance/Allowance), or the forwarded
	// InputBox.addInput (InputTooLarge and the application foreclosure-probe
	// family).
	if err != nil {
		return cli.DecorateRevert(err,
			ierc20portal.IErc20PortalMetaData,
			ierc20errors.IERC20ErrorsMetaData,
			iinputbox.IInputBoxMetaData,
			iapplication.IApplicationMetaData,
		)
	}
	if receipt != nil {
		// Erc20Portal uses abi.encodePacked(token, sender, amount, execData).
		payload := bytes.Join([][]byte{tokenAddr.Bytes(), depositOpts.From.Bytes(), common.LeftPadBytes(amount.Bytes(), common.HashLength),
			execData}, nil)
		if err := verifyDepositReceipt(ctx, client, receipt, appAddr, portalAddr, payload); err != nil {
			return fmt.Errorf("transaction %s mined, but its deposit could not be confirmed: %w", tx.Hash(), err)
		}
	}

	if asJSONParam {
		result := struct {
			cli.TransactionResult
			ApplicationAddress common.Address `json:"application_address"`
			PortalAddress      common.Address `json:"portal_address"`
			TokenAddress       common.Address `json:"token_address"`
			Amount             string         `json:"amount"`
			ApproveTxHash      *common.Hash   `json:"approve_transaction_hash,omitempty"`
		}{
			TransactionResult:  cli.NewTransactionResult(tx, receipt),
			ApplicationAddress: appAddr,
			PortalAddress:      portalAddr,
			TokenAddress:       tokenAddr,
			Amount:             amount.String(),
			ApproveTxHash:      approveHash,
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if approveHash != nil {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "approve tx-hash: %s\n", approveHash.Hex()); err != nil {
			return err
		}
	}
	return cli.WriteTransactionResult(cmd, tx, receipt)
}

func parseAddress(name string, value string) (common.Address, error) {
	if !common.IsHexAddress(value) {
		return common.Address{}, fmt.Errorf("invalid %s address %q", name, value)
	}
	return common.HexToAddress(value), nil
}

func parseAmount(value string) (*big.Int, error) {
	var amount *big.Int
	var err error
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		amount, err = hexutil.DecodeBig(value)
		if err != nil {
			return nil, err
		}
	} else {
		var ok bool
		amount, ok = new(big.Int).SetString(value, 10) //nolint:mnd // User-facing amounts are decimal.
		if !ok {
			return nil, fmt.Errorf("invalid amount %q", value)
		}
	}
	if amount.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	const uint256Bits = 256
	if amount.BitLen() > uint256Bits {
		return nil, fmt.Errorf("amount must fit in uint256")
	}
	return amount, nil
}
