// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package withdraw

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

var Cmd = &cobra.Command{
	Use:     "withdraw [app-name-or-address]",
	Short:   "Withdraw the funds of a single account from a foreclosed application",
	Example: examples,
	Args:    cobra.ExactArgs(1),
	Run:     run,
	Long: `
Calls IApplication.withdraw(account, AccountValidityProof). The signer is just
the gas-payer; the recipient of the funds is encoded inside the 'account'
bytes per the application's WithdrawalOutputBuilder convention. The same
wallet that pays gas does NOT need to match (or own) the account being
withdrawn — they can be different.

The [app-name-or-address] argument accepts EITHER an application name
(looked up in the local rollups-node database) OR an Ethereum address (used
directly without any DB access — useful on remote/reader hosts).

The proof data is consumed verbatim from --proof-file (JSON). Suggested shape:

  {
    "account": "0x... bytes ...",
    "account_index": "0x...",
    "account_root_siblings": ["0x...", "0x...", ...]
  }

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection (only when an app name is passed)
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_AUTH_MNEMONIC, CARTESI_AUTH_PRIVATE_KEY, CARTESI_AUTH_AWS_KMS_KEY_ID  signer
  CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX            derived account index (mnemonic auth)`,
}

const examples = `# Withdraw one account from a foreclosed application:
cartesi-rollups-cli withdraw echo-dapp --proof-file ./account-proof.json

# Skip the confirmation prompt:
cartesi-rollups-cli withdraw echo-dapp --proof-file ./account-proof.json --yes`

type withdrawProofJSON struct {
	Account             string   `json:"account"`
	AccountIndex        string   `json:"account_index"`
	AccountRootSiblings []string `json:"account_root_siblings"`
}

var (
	proofFileParam   string
	skipConfirmation bool
	asJSONParam      bool
)

func init() {
	Cmd.Flags().StringVar(&proofFileParam, "proof-file", "",
		"Path to the JSON account proof file emitted by the proof generation tool")
	cobra.CheckErr(Cmd.MarkFlagRequired("proof-file"))
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

	account, proof, err := loadProof(proofFileParam)
	cobra.CheckErr(err)

	appAddr, err := util.ResolveApplicationAddress(ctx, nameOrAddress)
	cobra.CheckErr(err)

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)
	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	cobra.CheckErr(err)

	chainID, err := client.ChainID(ctx)
	cobra.CheckErr(err)
	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	cobra.CheckErr(err)

	appContract, err := iapplication.NewIApplication(appAddr, client)
	cobra.CheckErr(err)

	// Identify the WithdrawalOutputBuilder and try to surface a decoded
	// recipient + amount. A hand-edit that flips a few characters in
	// `account` would otherwise produce a self-consistent proof against
	// the wrong recipient and the withdraw would silently succeed.
	builderAddr, err := appContract.GetWithdrawalOutputBuilder(&bind.CallOpts{Context: ctx})
	cobra.CheckErr(err)
	accountDesc, matched, err := ethutil.DescribeWithdrawalAccount(ctx, client, builderAddr, account)
	cobra.CheckErr(err)

	if !matched {
		// Unknown builder family. Print the raw bytes so the operator can
		// verify character-for-character, and force interactive
		// confirmation even when --yes is set.
		fmt.Fprintf(os.Stderr,
			"WARNING: builder %s is not a recognized WithdrawalOutputBuilder family.\n"+
				"         The recipient cannot be auto-decoded. Verify the bytes below\n"+
				"         match your intended account before confirming; --yes is ignored.\n%s",
			builderAddr, hex.Dump(account))
	}

	if !skipConfirmation || !matched {
		fmt.Printf("Preparing to withdraw an account from application %v\n"+
			"  gas-payer:           %v  (does NOT have to be the funds recipient)\n"+
			"  withdrawal builder:  %v\n"+
			"  account size:        %d bytes\n"+
			"  account index:       %d\n"+
			"  proof siblings:      %d\n",
			appAddr, txOpts.From, builderAddr,
			len(account), proof.AccountIndex, len(proof.AccountRootSiblings))
		if matched {
			fmt.Println(accountDesc)
		}
		confirmed, promptErr := cli.ConfirmPrompt("Do you want to continue?")
		cobra.CheckErr(promptErr)
		if !confirmed {
			fmt.Println("Transaction cancelled")
			os.Exit(0)
		}
	}

	tx, err := appContract.Withdraw(txOpts, account, proof)
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
		fmt.Printf("withdraw tx-hash: %v\n", txHash)
	}
}

func loadProof(path string) ([]byte, iapplication.AccountValidityProof, error) {
	zero := iapplication.AccountValidityProof{}
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, zero, fmt.Errorf("read proof file %s: %w", path, err)
	}
	var aux withdrawProofJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return nil, zero, fmt.Errorf("parse proof file %s: %w", path, err)
	}

	account, err := hexutil.Decode(aux.Account)
	if err != nil {
		return nil, zero, fmt.Errorf("invalid account: %w", err)
	}

	idx, err := hexutil.DecodeUint64(aux.AccountIndex)
	if err != nil {
		return nil, zero, fmt.Errorf("invalid account_index: %w", err)
	}

	siblings := make([][32]byte, len(aux.AccountRootSiblings))
	for i, s := range aux.AccountRootSiblings {
		b, err := hexutil.Decode(s)
		if err != nil {
			return nil, zero, fmt.Errorf("invalid account_root_siblings[%d]: %w", i, err)
		}
		if len(b) != 32 { //nolint:mnd
			return nil, zero, fmt.Errorf(
				"account_root_siblings[%d] must be 32 bytes, got %d", i, len(b))
		}
		copy(siblings[i][:], b)
	}
	return account, iapplication.AccountValidityProof{
		AccountIndex:        idx,
		AccountRootSiblings: siblings,
	}, nil
}
