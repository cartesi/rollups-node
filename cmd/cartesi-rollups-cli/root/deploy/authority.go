// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var (
	authorityFactoryAddressParam string
	authorityOwnerAddressParam   string
)

var authorityCmd = &cobra.Command{
	Use:     "authority",
	Short:   "Deploy a new authority contract",
	Example: authorityExamples,
	Run:     runDeployAuthority,
	Long: `
Supported Environment Variables:
  CARTESI_CONTRACTS_AUTHORITY_FACTORY_ADDRESS    Authority Factory Address`,
}

const authorityExamples = `
# deploy a new authority contract
 - cli deploy authority

# deploy a new authority contract with a custom owner and factory address
 - cli deploy authority --authority-owner 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA --authority-factory 0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB`

func init() {
	authorityCmd.Flags().StringVarP(&authorityFactoryAddressParam, "authority-factory", "F", "",
		"Authority Factory Address. If defined, epoch-length value will be used to create a new consensus")
	authorityCmd.Flags().StringVarP(&authorityOwnerAddressParam, "authority-owner", "O", "",
		"Authority Owner. If not defined, it will be derived from the auth method.")

	origHelpFunc := authorityCmd.HelpFunc()
	authorityCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("epoch-length").Hidden = false
		command.Flags().Lookup("salt").Hidden = false
		command.Flags().Lookup("json").Hidden = false
		command.Flags().Lookup("verbose").Hidden = false
		origHelpFunc(command, strings)
	})
}

func runDeployAuthority(cmd *cobra.Command, args []string) {
	var err error

	ctx := cmd.Context()

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint.String())
	cobra.CheckErr(err)

	chainId, err := client.ChainID(ctx)
	cobra.CheckErr(err)

	txOpts, err := auth.GetTransactOpts(ctx, chainId)
	cobra.CheckErr(err)

	deployment, err := buildAuthorityDeployment(cmd, txOpts)
	cobra.CheckErr(err)

	if verboseParam {
		fmt.Fprint(os.Stderr, deployment)
		fmt.Fprintln(os.Stderr, "\twallet address:       ", txOpts.From)

	}

	// factory check
	if verboseParam {
		fmt.Fprint(os.Stderr, "checking factory address...")
	}

	factoryAddress := deployment.FactoryAddress
	data, err := client.CodeAt(ctx, factoryAddress, nil)
	cobra.CheckErr(err)

	if len(data) == 0 {
		cobra.CheckErr(fmt.Errorf("No code at the factory address: %v", factoryAddress))
	}
	if verboseParam {
		fmt.Fprint(os.Stderr, "success\n")
	}

	// deploy
	if verboseParam || !asJSONParam {
		fmt.Fprintf(os.Stderr, "deploying authority...")
	}
	deployment.Address, err = deployment.Deploy(ctx, client, txOpts)
	cobra.CheckErr(err)

	// report
	if verboseParam || !asJSONParam {
		fmt.Fprintf(os.Stderr, "success\n")
		fmt.Fprintln(os.Stderr, "\tconsensus address:    ", deployment.Address)
		fmt.Fprintln(os.Stderr, "\tepoch length:         ", deployment.EpochLength)
	}

	if asJSONParam {
		report, err := json.MarshalIndent(&deployment, "", "  ")
		cobra.CheckErr(err) // deployed, but fail to print

		fmt.Println(string(report))
	}
}

func buildAuthorityDeployment(cmd *cobra.Command, txOpts *bind.TransactOpts) (*ethutil.AuthorityDeployment, error) {
	var err error
	var authorityFactoryAddress common.Address
	var authorityOwnerAddress common.Address

	if !cmd.Flags().Changed("authority-factory") {
		authorityFactoryAddress, err = config.GetContractsAuthorityFactoryAddress()
	} else {
		authorityFactoryAddress, err = parseHexAddress(authorityFactoryAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter authority-factory: %w", err)
	}

	if !cmd.Flags().Changed("authority-owner") {
		authorityOwnerAddress = txOpts.From
	} else {
		authorityOwnerAddress, err = parseHexAddress(authorityOwnerAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter authority-owner: %w", err)
	}

	salt, err := ethutil.ParseSalt(saltParam)
	if err != nil {
		return nil, fmt.Errorf("error on parameter salt: %w", err)
	}

	return &ethutil.AuthorityDeployment{
		FactoryAddress: authorityFactoryAddress,
		OwnerAddress:   authorityOwnerAddress,
		EpochLength:    epochLengthParam,
		Salt:           salt,
		Verbose:        verboseParam,
	}, nil
}
