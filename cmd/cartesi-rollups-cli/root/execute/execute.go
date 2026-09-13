// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

var Cmd = newCommand()

type options struct {
	proofFile        string
	skipConfirmation bool
}

func newCommand() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:     "execute [app-name-or-address] [output-index]",
		Short:   "Execute a proven application output",
		Example: examples,
		Args:    cobra.ExactArgs(2), //nolint:mnd // Application and output index.
		RunE:    opts.run,
		PreRunE: func(command *cobra.Command, _ []string) error {
			if command.Flags().Changed("proof-file") && opts.proofFile == "" {
				return fmt.Errorf("--proof-file cannot be empty")
			}
			return nil
		},
		Long: `Execute the output at the application-wide output index.

Without --proof-file, read the output and its proof from the local database.
With --proof-file, read a JSON object with raw_data and output_hashes_siblings.
raw_data must contain the complete 0x-prefixed output bytes, not decoded payload.
output_hashes_siblings must contain the ordered 0x-prefixed 32-byte proof hashes.
The output index remains the second positional argument; do not put it in the file.

An application address plus --proof-file needs no database or node JSON-RPC API.
An application name still needs the database to resolve its address.

Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Required for local output or application-name lookup
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Ethereum HTTP endpoint`,
	}
	cmd.Flags().StringVar(&opts.proofFile, "proof-file", "", "JSON file with raw_data and output_hashes_siblings")
	cmd.Flags().BoolVarP(&opts.skipConfirmation, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().Bool("json", false, "Print result as JSON")
	cli.AddTransactionFlags(cmd)

	originalHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(command *cobra.Command, args []string) {
		for _, name := range []string{"verbose", "database-connection", "blockchain-http-endpoint"} {
			if flag := command.Flags().Lookup(name); flag != nil {
				flag.Hidden = false
			}
		}
		originalHelp(command, args)
	})
	return cmd
}

const examples = `# Execute output 5 with its proof from the local database:
cartesi-rollups-cli execute echo-dapp 5

# Export a proof from the node JSON-RPC API:
cartesi-rollups-cli read outputs echo-dapp 5 --jsonrpc | jq '.data | {raw_data, output_hashes_siblings}' > output-proof.json

# Execute the exported output without a database or node JSON-RPC connection:
cartesi-rollups-cli execute 0x1234567890123456789012345678901234567890 5 --proof-file output-proof.json --yes`

type executionInput struct {
	applicationAddress common.Address
	output             []byte
	proof              iapplication.OutputValidityProof
}

func (o *options) run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	if err != nil {
		return err
	}
	outputIndex, err := config.ToUint64FromDecimalOrHexString(args[1])
	if err != nil {
		return err
	}
	input, err := resolveExecution(ctx, nameOrAddress, outputIndex, o.proofFile)
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
	if !o.skipConfirmation {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "Preparing to execute application %v (%v) output index %v with account %v\n",
			nameOrAddress, input.applicationAddress, outputIndex, txOpts.From)
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
	appContract, err := iapplication.NewIApplication(input.applicationAddress, client)
	if err != nil {
		return err
	}
	tx, receipt, err := cli.Transact(ctx, cmd, client, txOpts, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return appContract.ExecuteOutput(opts, input.output, input.proof)
	})
	if err != nil {
		return cli.DecorateRevert(err, iapplication.IApplicationMetaData)
	}
	if receipt != nil {
		executed := false
		for _, log := range receipt.Logs {
			if log == nil || log.Address != input.applicationAddress {
				continue
			}
			event, err := appContract.ParseOutputExecuted(*log)
			if err == nil && event.OutputIndex == outputIndex && bytes.Equal(event.Output, input.output) {
				executed = true
				break
			}
		}
		if !executed {
			return fmt.Errorf("transaction %s mined, but its receipt has no matching OutputExecuted event", tx.Hash())
		}
	}
	return cli.WriteTransactionResult(cmd, tx, receipt)
}

func resolveExecution(ctx context.Context, nameOrAddress string, outputIndex uint64, proofFile string) (executionInput, error) {
	input := executionInput{proof: iapplication.OutputValidityProof{OutputIndex: outputIndex}}
	if proofFile != "" {
		output, siblings, err := loadOutputProof(proofFile)
		if err != nil {
			return executionInput{}, err
		}
		address, err := util.ResolveApplicationAddress(ctx, nameOrAddress)
		if err != nil {
			return executionInput{}, err
		}
		input.applicationAddress = address
		input.output = output
		input.proof.OutputHashesSiblings = siblings
		return input, nil
	}
	dsn, err := config.GetDatabaseConnection()
	if err != nil {
		return executionInput{}, err
	}
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	if err != nil {
		return executionInput{}, err
	}
	defer repo.Close()
	output, err := repo.GetOutput(ctx, nameOrAddress, outputIndex)
	if err != nil {
		return executionInput{}, err
	}
	if output == nil {
		return executionInput{}, fmt.Errorf("output with index %d was not found in the database", outputIndex)
	}
	if len(output.OutputHashesSiblings) == 0 {
		return executionInput{}, fmt.Errorf("output with index %d has no associated proof yet", outputIndex)
	}
	app, err := repo.GetApplication(ctx, nameOrAddress)
	if err != nil {
		return executionInput{}, err
	}
	if app == nil {
		return executionInput{}, fmt.Errorf("application %q not found", nameOrAddress)
	}
	input.applicationAddress = app.IApplicationAddress
	input.output = output.RawData
	input.proof.OutputHashesSiblings = make([][32]byte, len(output.OutputHashesSiblings))
	for i, hash := range output.OutputHashesSiblings {
		input.proof.OutputHashesSiblings[i] = hash
	}
	return input, nil
}

func loadOutputProof(path string) ([]byte, [][32]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read output proof file: %w", err)
	}
	var data struct {
		RawData              string   `json:"raw_data"`
		OutputHashesSiblings []string `json:"output_hashes_siblings"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return nil, nil, fmt.Errorf("parse output proof file: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, nil, fmt.Errorf("parse output proof file trailing data: %w", err)
		}
		return nil, nil, fmt.Errorf("output proof file must contain exactly one JSON object")
	}
	output, err := hexutil.Decode(data.RawData)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid raw_data: %w", err)
	}
	if len(output) == 0 {
		return nil, nil, fmt.Errorf("raw_data cannot be empty")
	}
	if len(data.OutputHashesSiblings) == 0 {
		return nil, nil, fmt.Errorf("output_hashes_siblings cannot be empty")
	}
	siblings := make([][32]byte, len(data.OutputHashesSiblings))
	for i, encoded := range data.OutputHashesSiblings {
		decoded, err := hexutil.Decode(encoded)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid output_hashes_siblings[%d]: %w", i, err)
		}
		if len(decoded) != common.HashLength {
			return nil, nil, fmt.Errorf("output_hashes_siblings[%d] must be %d bytes, got %d", i, common.HashLength, len(decoded))
		}
		copy(siblings[i][:], decoded)
	}
	return output, siblings, nil
}
