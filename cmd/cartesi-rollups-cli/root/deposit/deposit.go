// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deposit

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
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
	Short:   "Deposit ERC-20 tokens through the ERC20Portal",
	Example: erc20Examples,
	Args:    cobra.ExactArgs(1),
	Run:     runERC20,
	Long: `
Calls ERC20Portal.depositERC20Tokens(token, app, amount, execData).

The command does not approve token spending unless --approve is supplied.
Without --approve, the signer must already have enough allowance for the
portal.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection (only when an app name is passed)
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_AUTH_MNEMONIC, CARTESI_AUTH_PRIVATE_KEY, CARTESI_AUTH_AWS_KMS_KEY_ID  signer
  CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX            derived account index (mnemonic auth)`,
}

const erc20Examples = `# Deposit 100 units of a token through the ERC20Portal:
cartesi-rollups-cli deposit erc20 echo-dapp \
  --portal 0x22E57511C30CcE6CDaa742E13CE3b774fDC663b1 \
  --token 0x88A2120B7068E78692C8fd12E751d610B6377E4d \
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

	erc20Cmd.Flags().StringVar(&portalParam, "portal", "", "ERC20Portal contract address")
	erc20Cmd.Flags().StringVar(&tokenParam, "token", "", "ERC-20 token contract address")
	erc20Cmd.Flags().StringVar(&amountParam, "amount", "", "Token amount to deposit (decimal or 0x-prefixed)")
	erc20Cmd.Flags().StringVar(&execDataParam, "exec-data", "0x", "Extra execution-layer data")
	erc20Cmd.Flags().BoolVar(&approveParam, "approve", false, "Approve the portal for --amount before depositing")
	erc20Cmd.Flags().BoolVarP(&skipConfirmation, "yes", "y", false, "Skip confirmation prompt")
	erc20Cmd.Flags().BoolVar(&asJSONParam, "json", false, "Print result as JSON")
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

func runERC20(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	appAddr, err := util.ResolveApplicationAddress(ctx, args[0])
	cobra.CheckErr(err)
	portalAddr, err := parseAddress("portal", portalParam)
	cobra.CheckErr(err)
	tokenAddr, err := parseAddress("token", tokenParam)
	cobra.CheckErr(err)
	amount, err := parseAmount(amountParam)
	cobra.CheckErr(err)
	execData, err := hexutil.Decode(execDataParam)
	cobra.CheckErr(err)

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)
	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	cobra.CheckErr(err)
	chainID, err := client.ChainID(ctx)
	cobra.CheckErr(err)
	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	cobra.CheckErr(err)

	if !skipConfirmation {
		fmt.Printf("Preparing ERC-20 deposit\n"+
			"  signer:      %s\n"+
			"  application: %s\n"+
			"  portal:      %s\n"+
			"  token:       %s\n"+
			"  amount:      %s\n"+
			"  approve:     %t\n",
			txOpts.From, appAddr, portalAddr, tokenAddr, amount.String(), approveParam)
		confirmed, promptErr := cli.ConfirmPrompt("Do you want to continue?")
		cobra.CheckErr(promptErr)
		if !confirmed {
			fmt.Println("Transaction cancelled")
			os.Exit(0)
		}
	}

	var approveHash *common.Hash
	if approveParam {
		token, err := ierc20metadata.NewIERC20Metadata(tokenAddr, client)
		cobra.CheckErr(err)
		approveOpts, err := cli.GetTransactOpts(ctx, chainID)
		cobra.CheckErr(err)
		tx, err := token.Approve(approveOpts, portalAddr, amount)
		cobra.CheckErr(cli.DecorateRevert(err,
			ierc20metadata.IERC20MetadataMetaData,
			ierc20errors.IERC20ErrorsMetaData,
		))
		receipt, err := bind.WaitMined(ctx, client, tx)
		cobra.CheckErr(err)
		cobra.CheckErr(checkReceiptStatus(receipt, "approve"))
		hash := receipt.TxHash
		approveHash = &hash
	}

	portal, err := ierc20portal.NewIERC20Portal(portalAddr, client)
	cobra.CheckErr(err)
	depositOpts, err := cli.GetTransactOpts(ctx, chainID)
	cobra.CheckErr(err)
	tx, err := portal.DepositERC20Tokens(depositOpts, tokenAddr, appAddr, amount, execData)
	// The revert can come from three layers: the portal itself
	// (ERC20TransferFailed), the token's transferFrom (ERC-6093 errors such
	// as ERC20InsufficientBalance/Allowance), or the forwarded
	// InputBox.addInput (InputTooLarge and the application foreclosure-probe
	// family).
	cobra.CheckErr(cli.DecorateRevert(err,
		ierc20portal.IERC20PortalMetaData,
		ierc20errors.IERC20ErrorsMetaData,
		iinputbox.IInputBoxMetaData,
		iapplication.IApplicationMetaData,
	))
	receipt, err := bind.WaitMined(ctx, client, tx)
	cobra.CheckErr(err)
	cobra.CheckErr(checkReceiptStatus(receipt, "depositERC20Tokens"))

	if asJSONParam {
		result := struct {
			ApplicationAddress common.Address `json:"application_address"`
			PortalAddress      common.Address `json:"portal_address"`
			TokenAddress       common.Address `json:"token_address"`
			Amount             string         `json:"amount"`
			ApproveTxHash      *common.Hash   `json:"approve_transaction_hash,omitempty"`
			TransactionHash    common.Hash    `json:"transaction_hash"`
			BlockNumber        string         `json:"block_number"`
		}{
			ApplicationAddress: appAddr,
			PortalAddress:      portalAddr,
			TokenAddress:       tokenAddr,
			Amount:             amount.String(),
			ApproveTxHash:      approveHash,
			TransactionHash:    receipt.TxHash,
			BlockNumber:        fmt.Sprintf("0x%x", receipt.BlockNumber.Uint64()),
		}
		jsonBytes, err := json.MarshalIndent(&result, "", "  ")
		cobra.CheckErr(err)
		fmt.Println(string(jsonBytes))
	} else {
		if approveHash != nil {
			fmt.Printf("approve tx-hash: %s\n", approveHash.Hex())
		}
		fmt.Printf("deposit tx-hash: %s blockNumber: %d\n", receipt.TxHash, receipt.BlockNumber.Uint64())
	}
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
	return amount, nil
}

func checkReceiptStatus(receipt *types.Receipt, action string) error {
	if receipt == nil {
		return fmt.Errorf("%s transaction has no receipt", action)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("%s transaction failed: %s", action, receipt.TxHash)
	}
	return nil
}
