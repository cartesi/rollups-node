// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorumfactory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var (
	quorumFactoryAddressParam  string
	quorumValidatorAddressArgs []string
)

var quorumCmd = &cobra.Command{
	Use:     "quorum",
	Short:   "Deploy a new quorum contract",
	Example: quorumExamples,
	Args:    cobra.NoArgs,
	Run:     runDeployQuorum,
	Long: `
Supported Environment Variables:
  CARTESI_CONTRACTS_QUORUM_FACTORY_ADDRESS    Quorum Factory Address`,
}

const quorumExamples = `
# deploy a new quorum contract
 - cli deploy quorum --validator 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA

# deploy a new quorum contract with multiple validators and a custom factory address
 - cli deploy quorum --validator 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA \
     --validator 0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB \
     --quorum-factory 0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC`

func init() {
	quorumCmd.Flags().StringVarP(&quorumFactoryAddressParam, "quorum-factory", "F", "",
		"Quorum Factory Address. If not defined, it will be retrieved from configuration.")
	quorumCmd.Flags().StringArrayVarP(&quorumValidatorAddressArgs, "validator", "v", nil,
		"Quorum validator address. Repeat this flag for multiple validators.")

	origHelpFunc := quorumCmd.HelpFunc()
	quorumCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("epoch-length").Hidden = false
		command.Flags().Lookup("salt").Hidden = false
		command.Flags().Lookup("json").Hidden = false
		command.Flags().Lookup("verbose").Hidden = false
		origHelpFunc(command, strings)
	})
	cli.AddTransactionFlags(quorumCmd)
}

func runDeployQuorum(cmd *cobra.Command, _ []string) {
	var err error

	ctx := cmd.Context()

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	cobra.CheckErr(err)
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	cobra.CheckErr(err)

	txOpts, err := cli.GetTransactOpts(ctx, chainID)
	cobra.CheckErr(err)

	deployment, err := buildQuorumDeployment(cmd)
	cobra.CheckErr(err)

	if verboseParam {
		fmt.Fprint(os.Stderr, deployment)
		fmt.Fprintln(os.Stderr, "\twallet address:       ", txOpts.From)
	}

	if verboseParam {
		fmt.Fprint(os.Stderr, "checking factory address...")
	}

	factoryAddress := deployment.FactoryAddress
	data, err := client.CodeAt(ctx, factoryAddress, nil)
	cobra.CheckErr(err)

	if len(data) == 0 {
		cobra.CheckErr(fmt.Errorf("no code at the factory address: %v", factoryAddress))
	}
	if verboseParam {
		fmt.Fprint(os.Stderr, "success\n")
	}

	if verboseParam || !asJSONParam {
		fmt.Fprintf(os.Stderr, "deploying quorum...")
	}
	var tx *types.Transaction
	var receipt *types.Receipt
	deployment.Address, err = deployment.DeployWithTransaction(ctx, client, txOpts,
		func(
			ctx context.Context, opts *bind.TransactOpts, build func(*bind.TransactOpts) (*types.Transaction, error),
		) (*types.Receipt, error) {
			var err error
			tx, receipt, err = cli.Transact(ctx, cmd, client, opts, build)
			return receipt, err
		})
	cobra.CheckErr(cli.DecorateRevert(err, iquorumfactory.IQuorumFactoryMetaData))
	if receipt == nil {
		cobra.CheckErr(writeDeploymentBroadcast(cmd, tx, deployment.Address))
		return
	}

	if verboseParam || !asJSONParam {
		fmt.Fprintf(os.Stderr, "success\n")
	}

	if asJSONParam {
		report := struct {
			*ethutil.QuorumDeployment
			Transaction cli.TransactionResult `json:"transaction"`
		}{QuorumDeployment: deployment, Transaction: cli.NewTransactionResult(tx, receipt)}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		cobra.CheckErr(encoder.Encode(report))
	} else {
		cobra.CheckErr(cli.WriteTransactionResult(cmd, tx, receipt))
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Consensus address: %s\nEpoch length: %d\nClaim staging period: %d\n",
			deployment.Address, deployment.EpochLength, deployment.ClaimStagingPeriod)
		cobra.CheckErr(err)
	}
}

func buildQuorumDeployment(cmd *cobra.Command) (*ethutil.QuorumDeployment, error) {
	var err error
	var quorumFactoryAddress common.Address

	if !cmd.Flags().Changed("quorum-factory") {
		quorumFactoryAddress, err = config.GetContractsQuorumFactoryAddress()
	} else {
		quorumFactoryAddress, err = parseHexAddress(quorumFactoryAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter quorum-factory: %w", err)
	}

	validators, err := parseValidatorAddresses(quorumValidatorAddressArgs)
	if err != nil {
		return nil, fmt.Errorf("error on parameter validator: %w", err)
	}

	salt, err := ethutil.ParseSalt(saltParam)
	if err != nil {
		return nil, fmt.Errorf("error on parameter salt: %w", err)
	}

	return &ethutil.QuorumDeployment{
		FactoryAddress:     quorumFactoryAddress,
		Validators:         validators,
		EpochLength:        epochLengthParam,
		ClaimStagingPeriod: claimStagingPeriodParam,
		Salt:               salt,
		Verbose:            verboseParam,
	}, nil
}

func parseValidatorAddresses(values []string) ([]common.Address, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one --validator address is required")
	}

	validators := make([]common.Address, 0, len(values))
	seen := map[common.Address]struct{}{}
	for _, value := range values {
		if !common.IsHexAddress(value) {
			return nil, fmt.Errorf("failed to parse hex address: %s", value)
		}
		validator := common.HexToAddress(value)
		if validator == (common.Address{}) {
			return nil, fmt.Errorf("zero address validator is not allowed")
		}
		if _, ok := seen[validator]; ok {
			return nil, fmt.Errorf("duplicate validator address: %s", validator.Hex())
		}
		seen[validator] = struct{}{}
		validators = append(validators, validator)
	}
	return validators, nil
}
