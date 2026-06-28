// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package provedriveroot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

var Cmd = &cobra.Command{
	Use:     "prove-drive-root [app-name-or-address]",
	Short:   "Anchor the accounts-drive Merkle root on a foreclosed application",
	Example: examples,
	Args:    cobra.ExactArgs(1),
	Run:     run,
	Long: `
Calls IApplication.proveAccountsDriveMerkleRoot(accountsDriveMerkleRoot, proof).
This must be done ONCE per foreclosed application before any user can call
withdraw(). The signer is just the gas-payer; the call is permissionless.

The [app-name-or-address] argument accepts EITHER an application name
(looked up in the local rollups-node database) OR an Ethereum address (used
directly without any DB access — useful on remote/reader hosts).

The proof data is consumed verbatim from --proof-file (JSON). Suggested shape:

  {
    "accounts_drive_merkle_root": "0x... 32 bytes ...",
    "proof": ["0x...", "0x...", ...]
  }

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection (only when an app name is passed)
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_AUTH_MNEMONIC, CARTESI_AUTH_PRIVATE_KEY, CARTESI_AUTH_AWS_KMS_KEY_ID  signer
  CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX            derived account index (mnemonic auth)`,
}

const examples = `# Anchor the accounts-drive Merkle root from a JSON proof file:
cartesi-rollups-cli prove-drive-root echo-dapp --proof-file ./drive-root-proof.json

# Skip the confirmation prompt:
cartesi-rollups-cli prove-drive-root echo-dapp --proof-file ./drive-root-proof.json --yes`

type proveDriveRootJSON struct {
	AccountsDriveMerkleRoot string   `json:"accounts_drive_merkle_root"`
	Proof                   []string `json:"proof"`
}

var (
	proofFileParam   string
	skipConfirmation bool
	asJSONParam      bool
)

func init() {
	Cmd.Flags().StringVar(&proofFileParam, "proof-file", "",
		"Path to the JSON proof file emitted by the accounts-drive proof generation tool")
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

	root, proof, err := loadProof(proofFileParam)
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

	if !skipConfirmation {
		fmt.Printf("Preparing to prove the accounts-drive Merkle root for application %v\n"+
			"  signer:     %v\n"+
			"  root:       0x%x\n"+
			"  proof size: %d siblings\n",
			appAddr, txOpts.From, root, len(proof))
		confirmed, promptErr := cli.ConfirmPrompt("Do you want to continue?")
		cobra.CheckErr(promptErr)
		if !confirmed {
			fmt.Println("Transaction cancelled")
			os.Exit(0)
		}
	}

	tx, err := appContract.ProveAccountsDriveMerkleRoot(txOpts, root, proof)
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
		fmt.Printf("prove-drive-root tx-hash: %v\n", txHash)
	}
}

func loadProof(path string) ([32]byte, [][32]byte, error) {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return [32]byte{}, nil, fmt.Errorf("read proof file %s: %w", path, err)
	}
	var aux proveDriveRootJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return [32]byte{}, nil, fmt.Errorf("parse proof file %s: %w", path, err)
	}

	rootBytes, err := hexutil.Decode(aux.AccountsDriveMerkleRoot)
	if err != nil {
		return [32]byte{}, nil, fmt.Errorf("invalid accounts_drive_merkle_root: %w", err)
	}
	if len(rootBytes) != 32 { //nolint:mnd
		return [32]byte{}, nil, fmt.Errorf(
			"accounts_drive_merkle_root must be 32 bytes, got %d", len(rootBytes))
	}
	var root [32]byte
	copy(root[:], rootBytes)

	proof := make([][32]byte, len(aux.Proof))
	for i, s := range aux.Proof {
		b, err := hexutil.Decode(s)
		if err != nil {
			return [32]byte{}, nil, fmt.Errorf("invalid proof[%d]: %w", i, err)
		}
		if len(b) != 32 { //nolint:mnd
			return [32]byte{}, nil, fmt.Errorf("proof[%d] must be 32 bytes, got %d", i, len(b))
		}
		copy(proof[i][:], b)
	}
	return root, proof, nil
}
